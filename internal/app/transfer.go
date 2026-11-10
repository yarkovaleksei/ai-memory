package app

import (
	"context"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/yarkovaleksei/ai-memory/internal/adapter/claude"
	"github.com/yarkovaleksei/ai-memory/internal/adapter/codex"
	"github.com/yarkovaleksei/ai-memory/internal/model"
	"github.com/yarkovaleksei/ai-memory/internal/writer"
)

// Перенос между агентами, проектами и профилями — обычный план: создание
// записи в цели и, при переносе, удаление факта Claude или заметка «забыть»
// для записи Codex. Всё одной операцией, которая откатывается одной операцией
// (ADR-014). Связь «источник — копия» хранится в метаданных операции в
// репозитории бэкапов: история — единственный источник правды о переносах, и
// откат переноса сам убирает связь.

const transferPrefix = "transfer."

// Параметры операции переноса, по которым строятся связи.
const (
	paramFrom      = "from"      // ID исходной записи
	paramFromTitle = "fromTitle" // её заголовок на момент переноса
	paramFromAgent = "fromAgent" // её агент: источник может пропасть из индекса
	paramTarget    = "target"    // путь созданного файла
)

func transferCode(move bool, to model.Agent) string {
	mode := "copy"
	if move {
		mode = "move"
	}
	return transferPrefix + mode + "-to-" + string(to)
}

// TransferToClaude — факт Claude из записи любого агента: в другой проект или
// профиль Claude или из Codex. move — убрать исходную запись: факт Claude
// удаляется, для записи Codex пишется заметка «забыть» с текстом forgetTitle и
// forgetBody (их даёт словарь интерфейса).
func (s *PlansService) TransferToClaude(ctx context.Context, itemID, sourceID, projectID string, in claude.FactInput,
	move bool, forgetTitle, forgetBody string) (writer.Plan, error) {
	it, err := s.e.item(ctx, itemID)
	if err != nil {
		return writer.Plan{}, err
	}
	memDir, err := s.memoryDir(ctx, sourceID, projectID)
	if err != nil {
		return writer.Plan{}, err
	}
	create, err := claude.PlanCreate(memDir, in)
	if err != nil {
		return writer.Plan{}, s.e.error(err)
	}
	plans := []writer.Plan{create}
	if move {
		var remove writer.Plan
		switch {
		case it.Kind == model.KindClaudeFact && it.Write == model.WriteDirect:
			if samePath(filepath.Dir(it.File), memDir) {
				return writer.Plan{}, fail("transfer.same-place")
			}
			remove, err = claude.PlanDeleteMany([]claude.FactRef{{File: it.File, Hash: it.Hash}})
		case it.Agent == model.AgentCodex && it.Write != model.WriteNone:
			g, _ := s.e.codexAdapter().Active(ctx)
			remove, err = codex.PlanForget(s.e.rt.Config().CodexHome, g, forgetTitle, forgetBody)
		default:
			return writer.Plan{}, fail("plan.readonly", "title", it.Title)
		}
		if err != nil {
			return writer.Plan{}, s.e.error(err)
		}
		plans = append(plans, remove)
	}
	op := writer.Op{Code: transferCode(move, model.AgentClaude), Params: map[string]string{
		"name":         in.Name,
		paramFrom:      it.ID,
		paramFromTitle: it.Title,
		paramFromAgent: string(it.Agent),
		paramTarget:    create.Changes[0].Path,
	}}
	return s.e.keep(writer.Combine(op, plans...))
}

// TransferToCodex — заметка Codex из факта Claude. move — удалить факт.
func (s *PlansService) TransferToCodex(ctx context.Context, itemID, title, body string, move bool) (writer.Plan, error) {
	it, err := s.e.item(ctx, itemID)
	if err != nil {
		return writer.Plan{}, err
	}
	if it.Agent != model.AgentClaude {
		return writer.Plan{}, fail("transfer.same-agent")
	}
	g, _ := s.e.codexAdapter().Active(ctx)
	note, err := codex.PlanNewNote(s.e.rt.Config().CodexHome, g, title, body)
	if err != nil {
		return writer.Plan{}, s.e.error(err)
	}
	plans := []writer.Plan{note}
	if move {
		if it.Kind != model.KindClaudeFact || it.Write != model.WriteDirect {
			return writer.Plan{}, fail("plan.readonly", "title", it.Title)
		}
		remove, err := claude.PlanDeleteMany([]claude.FactRef{{File: it.File, Hash: it.Hash}})
		if err != nil {
			return writer.Plan{}, s.e.error(err)
		}
		plans = append(plans, remove)
	}
	op := writer.Op{Code: transferCode(move, model.AgentCodex), Params: map[string]string{
		"title":        strings.TrimSpace(title),
		paramFrom:      it.ID,
		paramFromTitle: it.Title,
		paramFromAgent: string(it.Agent),
		paramTarget:    note.Changes[0].Path,
	}}
	return s.e.keep(writer.Combine(op, plans...))
}

// TransferLink — связь записи с переносом: Direction «to» — эта запись
// источник, другая сторона — копия; «from» — эта запись получена переносом.
type TransferLink struct {
	Direction string      `json:"direction"`
	Move      bool        `json:"move"`
	ItemID    string      `json:"itemId"` // другая сторона; пусто, если её уже нет в индексе
	Title     string      `json:"title"`
	Agent     model.Agent `json:"agent"`
	Time      time.Time   `json:"time"`
	Commit    string      `json:"commit"`
}

// Links — переносы, в которых участвовала запись: показать пару «источник —
// копия» и предупредить о повторном переносе. Отменённые откатом переносы не
// показываются.
func (s *HistoryService) Links(ctx context.Context, itemID string) ([]TransferLink, error) {
	out := []TransferLink{}
	if s.e.repo == nil {
		return out, nil
	}
	it, err := s.e.item(ctx, itemID)
	if err != nil {
		return out, err
	}
	hist, err := (&writer.Writer{Backup: s.e.repo}).History(ctx, historyLimit)
	if err != nil {
		return out, s.e.error(err)
	}
	reverted := map[string]bool{}
	for _, h := range hist {
		if h.Op.Code == "revert" && !reverted[h.Commit] {
			reverted[h.Op.Params["commit"]] = true
		}
	}
	for _, h := range hist {
		p := h.Op.Params
		if !strings.HasPrefix(h.Op.Code, transferPrefix) || reverted[h.Commit] {
			continue
		}
		link := TransferLink{Move: strings.HasPrefix(h.Op.Code, transferPrefix+"move-"), Time: h.Time, Commit: h.Commit}
		switch {
		case p[paramFrom] == it.ID:
			link.Direction = "to"
			link.Agent = model.AgentClaude
			if strings.HasSuffix(h.Op.Code, "-to-codex") {
				link.Agent = model.AgentCodex
			}
			link.Title = nonEmpty(p["name"], p["title"])
			if other, err := s.e.lib.ByFile(ctx, p[paramTarget]); err == nil {
				link.ItemID, link.Title = other.ID, other.Title
			}
		case it.Anchor == "" && samePath(it.File, p[paramTarget]):
			link.Direction = "from"
			link.Title, link.Agent = p[paramFromTitle], model.Agent(p[paramFromAgent])
			if other, err := s.e.lib.Get(ctx, p[paramFrom]); err == nil {
				link.ItemID, link.Title, link.Agent = other.ID, other.Title, other.Agent
			}
		default:
			continue
		}
		out = append(out, link)
	}
	return out, nil
}

// samePath сравнивает пути так, как их сравнивает файловая система: на macOS
// и Windows по умолчанию без учёта регистра.
func samePath(a, b string) bool {
	a, b = filepath.Clean(a), filepath.Clean(b)
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		return strings.EqualFold(a, b)
	}
	return a == b
}
