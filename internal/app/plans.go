package app

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/yarkovaleksei/ai-memory/internal/adapter/agentfiles"
	"github.com/yarkovaleksei/ai-memory/internal/adapter/claude"
	"github.com/yarkovaleksei/ai-memory/internal/adapter/codex"
	"github.com/yarkovaleksei/ai-memory/internal/backup"
	"github.com/yarkovaleksei/ai-memory/internal/index"
	"github.com/yarkovaleksei/ai-memory/internal/library"
	"github.com/yarkovaleksei/ai-memory/internal/model"
	"github.com/yarkovaleksei/ai-memory/internal/writer"
)

// Запись из интерфейса идёт в два шага: метод PlansService строит план (ничего
// не пишет) и запоминает его по ID, интерфейс показывает diff, и только Apply
// с этим ID пишет. Из фронтенда приходят ID записей, хэши и тексты — путей
// Go не берёт ни в одном методе (AGENTS.md).

// maxPlans — сколько неподтверждённых планов держим: брошенный диалог не
// должен копить память.
const maxPlans = 32

// editor — общее состояние PlansService и HistoryService: планы откатов
// применяются тем же Apply.
type editor struct {
	lib      *library.Library
	rt       *Runtime
	repo     *backup.Repo
	disabled string // код причины, по которой запись выключена; пусто — включена
	log      *slog.Logger
	emit     func(name string, data any)

	mu    sync.Mutex
	plans map[string]writer.Plan
	order []string

	applyMu sync.Mutex // планы применяются по одному
}

// NewEditing открывает репозиторий бэкапов и создаёт сервисы записи. Без git
// окно работает, но только на чтение: писать без снимка нельзя (ADR-012).
func NewEditing(ctx context.Context, lib *library.Library, rt *Runtime, log *slog.Logger) (*PlansService, *HistoryService) {
	if log == nil {
		log = slog.Default()
	}
	e := &editor{lib: lib, rt: rt, log: log, plans: map[string]writer.Plan{}, emit: emitEvent}
	repo, err := backup.Open(ctx, filepath.FromSlash(rt.Config().DataDir))
	switch {
	case errors.Is(err, backup.ErrNoGit):
		e.disabled = backup.ErrNoGit.Error()
	case err != nil:
		log.Error("репозиторий бэкапов не открыт", "error", err)
		e.disabled = "backup.failed"
	default:
		e.repo = repo
	}
	return &PlansService{e: e}, &HistoryService{e: e}
}

// --- ошибки ---

// editError переводит отказ ядра в код для интерфейса.
func (e *editor) error(err error) error {
	if err == nil {
		return nil
	}
	var we *writer.Error
	if errors.As(err, &we) {
		return &Error{Code: we.Code, Params: we.Params}
	}
	var ae *Error
	if errors.As(err, &ae) {
		return ae
	}
	if errors.Is(err, index.ErrNotFound) {
		return &Error{Code: index.ErrNotFound.Error()}
	}
	e.log.Error("ошибка записи", "error", err)
	return &Error{Code: "write.failed", Params: map[string]string{"error": err.Error()}}
}

func fail(code string, kv ...string) error {
	we := writer.Fail(code, kv...)
	return &Error{Code: we.Code, Params: we.Params}
}

// --- окружение записи ---

func (e *editor) codexAdapter() codex.Adapter {
	cfg := e.rt.Config()
	return codex.Adapter{Home: cfg.CodexHome, Generation: codex.Parse(cfg.CodexGeneration)}
}

// writer собирается на каждое применение: активное поколение Codex может
// смениться, пока окно открыто.
func (e *editor) writer(ctx context.Context) (*writer.Writer, error) {
	if e.disabled != "" {
		return nil, fail(e.disabled)
	}
	cfg := e.rt.Config()
	roots := make([]string, 0, len(cfg.ClaudeProfiles)+1)
	for _, p := range cfg.ClaudeProfiles {
		roots = append(roots, filepath.FromSlash(p))
	}
	home := filepath.FromSlash(cfg.CodexHome)
	if home != "" {
		roots = append(roots, home)
	}
	roots = append(roots, e.fileRoots(ctx)...)
	g, _ := e.codexAdapter().Active(ctx)
	return &writer.Writer{
		Roots:  roots,
		Backup: e.repo,
		Guards: []writer.Guard{codex.Guard{Home: home, Generation: g}},
	}, nil
}

// fileRoots — корни инструкций и навыков вне профилей и CODEX_HOME (ADR-020):
// ~/.agents, каталоги проектов и цели симлинков навыков. Всё — из
// конфигурации и индекса, а не из интерфейса; откат получает те же корни.
func (e *editor) fileRoots(ctx context.Context) []string {
	var out []string
	if dir := e.rt.Config().AgentsDir; dir != "" {
		out = append(out, filepath.FromSlash(dir))
	}
	if projects, err := e.lib.Projects(ctx); err == nil {
		for _, p := range projects {
			if p.Path != "" {
				out = append(out, p.Path)
			}
		}
	}
	if page, err := e.lib.Search(ctx, index.Query{Kind: string(model.KindSkill), IncludeTemp: true}); err == nil {
		for _, h := range page.Hits {
			if h.Target != "" {
				out = append(out, h.Target)
			}
		}
	}
	return out
}

func (e *editor) item(ctx context.Context, id string) (model.Item, error) {
	it, err := e.lib.Get(ctx, id)
	if err != nil {
		return model.Item{}, e.error(err)
	}
	return it, nil
}

// --- хранилище планов ---

func (e *editor) keep(p writer.Plan, err error) (writer.Plan, error) {
	if err != nil {
		return writer.Plan{}, e.error(err)
	}
	if len(p.Changes) == 0 {
		return writer.Plan{}, fail("write.no-changes")
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.plans[p.ID] = p
	e.order = append(e.order, p.ID)
	for len(e.order) > maxPlans {
		delete(e.plans, e.order[0])
		e.order = e.order[1:]
	}
	return p, nil
}

func (e *editor) take(id string) (writer.Plan, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	p, ok := e.plans[id]
	if ok {
		delete(e.plans, id)
		for i, v := range e.order {
			if v == id {
				e.order = append(e.order[:i], e.order[i+1:]...)
				break
			}
		}
	}
	return p, ok
}

// --- PlansService ---

// PlansService строит планы записи и применяет подтверждённые.
type PlansService struct {
	e *editor
}

// EditStatus — можно ли писать и что известно о Codex.
type EditStatus struct {
	Enabled bool        `json:"enabled"`
	Reason  string      `json:"reason"` // код причины, если запись выключена
	Codex   CodexStatus `json:"codex"`
}

// CodexStatus — куда пишутся заметки и когда Codex их учтёт.
type CodexStatus struct {
	Generation       string              `json:"generation"`
	GenerationReason string              `json:"generationReason"`
	Consolidation    codex.Consolidation `json:"consolidation"`
	CooldownHours    int                 `json:"cooldownHours"`
}

// Status сообщает, включена ли запись, и состояние консолидации Codex.
func (s *PlansService) Status(ctx context.Context) EditStatus {
	g, reason := s.e.codexAdapter().Active(ctx)
	return EditStatus{
		Enabled: s.e.disabled == "",
		Reason:  s.e.disabled,
		Codex: CodexStatus{
			Generation:       string(g),
			GenerationReason: reason,
			Consolidation:    codex.ReadConsolidation(ctx, s.e.rt.Config().CodexHome, g),
			CooldownHours:    int(codex.CooldownAfterSuccess / time.Hour),
		},
	}
}

// Draft — файл записи для редактора. Hash интерфейс возвращает при построении
// плана: если файл с тех пор изменился, план не построится, а не затрёт чужую правку.
type Draft struct {
	Content string            `json:"content"`
	Hash    string            `json:"hash"`
	Fact    *claude.FactInput `json:"fact"` // поля формы факта Claude; nil — правится только исходник
}

// Draft читает с диска запись, которую можно править напрямую.
func (s *PlansService) Draft(ctx context.Context, itemID string) (Draft, error) {
	it, err := s.e.item(ctx, itemID)
	if err != nil {
		return Draft{}, err
	}
	switch {
	case it.Kind == model.KindClaudeFact && it.Write == model.WriteDirect:
		d, err := claude.ReadFact(it.File)
		if err != nil {
			return Draft{}, s.e.error(err)
		}
		out := Draft{Content: d.Content, Hash: d.Hash}
		if d.Parsed {
			out.Fact = &d.Fact
		}
		return out, nil
	case it.Kind == model.KindCodexAdHocNote && it.Write == model.WriteDirect,
		!it.Kind.Memory() && it.Write == model.WriteDirect:
		// Заметка Codex, файл инструкций или SKILL.md — правится исходник целиком.
		data, err := os.ReadFile(it.File)
		if err != nil {
			return Draft{}, fail("write.unreadable", "path", it.File, "error", err.Error())
		}
		return Draft{Content: string(data), Hash: model.ContentHash(data)}, nil
	}
	return Draft{}, fail("plan.readonly", "title", it.Title)
}

// CreateFact — новый факт Claude в проекте и профиле.
func (s *PlansService) CreateFact(ctx context.Context, sourceID, projectID string, in claude.FactInput) (writer.Plan, error) {
	memDir, err := s.memoryDir(ctx, sourceID, projectID)
	if err != nil {
		return writer.Plan{}, err
	}
	return s.e.keep(claude.PlanCreate(memDir, in))
}

// memoryDir находит каталог памяти проекта в профиле: там, где уже лежат его
// факты, иначе по закодированному пути проекта, как это делает Claude Code.
func (s *PlansService) memoryDir(ctx context.Context, sourceID, projectID string) (string, error) {
	srcs, err := s.e.lib.Sources(ctx)
	if err != nil {
		return "", s.e.error(err)
	}
	var src *model.Source
	for i := range srcs {
		if srcs[i].ID == sourceID && srcs[i].Agent == model.AgentClaude {
			src = &srcs[i]
		}
	}
	if src == nil {
		return "", fail("plan.source-invalid")
	}
	for _, w := range src.Warnings {
		if w.Code == "claude.settings.auto-memory-directory" {
			// Claude читает память из другого каталога, а мы его не знаем: запись
			// в каталог по умолчанию Claude не увидит.
			return "", fail("claude.auto-memory-directory", "dir", w.Params["dir"])
		}
	}
	page, err := s.e.lib.Search(ctx, index.Query{ProjectID: projectID, Agent: string(model.AgentClaude), IncludeTemp: true})
	if err != nil {
		return "", s.e.error(err)
	}
	for _, h := range page.Hits {
		if h.SourceID == sourceID && (h.Kind == model.KindClaudeFact || h.Kind == model.KindClaudeIndex) {
			return filepath.Dir(h.File), nil
		}
	}
	projects, err := s.e.lib.Projects(ctx)
	if err != nil {
		return "", s.e.error(err)
	}
	for _, p := range projects {
		if p.ID != projectID {
			continue
		}
		switch {
		case p.Global:
			return "", fail("claude.fact.project-required")
		case p.Path != "":
			return claude.MemoryDir(src.Root, claude.EncodeProjectPath(p.Path)), nil
		case strings.HasPrefix(p.ID, "encoded:"):
			return claude.MemoryDir(src.Root, strings.TrimPrefix(p.ID, "encoded:")), nil
		}
	}
	return "", fail("claude.fact.project-required")
}

// UpdateFact меняет поля факта Claude; baseHash — Draft.Hash.
func (s *PlansService) UpdateFact(ctx context.Context, itemID, baseHash string, in claude.FactInput) (writer.Plan, error) {
	it, err := s.e.item(ctx, itemID)
	if err != nil {
		return writer.Plan{}, err
	}
	if it.Kind != model.KindClaudeFact || it.Write != model.WriteDirect {
		return writer.Plan{}, fail("plan.readonly", "title", it.Title)
	}
	return s.e.keep(claude.PlanUpdate(it.File, nonEmpty(baseHash, it.Hash), in))
}

// UpdateSource заменяет исходник записи целиком: факт Claude (так чинится
// сломанный frontmatter), нашу заметку Codex, файл инструкций или SKILL.md.
func (s *PlansService) UpdateSource(ctx context.Context, itemID, baseHash, content string) (writer.Plan, error) {
	it, err := s.e.item(ctx, itemID)
	if err != nil {
		return writer.Plan{}, err
	}
	hash := nonEmpty(baseHash, it.Hash)
	switch {
	case it.Kind == model.KindClaudeFact && it.Write == model.WriteDirect:
		return s.e.keep(claude.PlanUpdateRaw(it.File, hash, content))
	case it.Kind == model.KindCodexAdHocNote && it.Write == model.WriteDirect:
		return s.e.keep(codex.PlanUpdateNote(it.File, hash, content))
	case !it.Kind.Memory() && it.Write == model.WriteDirect:
		return s.e.keep(agentfiles.PlanUpdate(it, hash, content))
	}
	return writer.Plan{}, fail("plan.readonly", "title", it.Title)
}

// DeleteFacts удаляет выбранные факты Claude одним планом. Записи Codex так не
// удаляются: для них есть заметка «забыть» (ADR-002).
func (s *PlansService) DeleteFacts(ctx context.Context, itemIDs []string) (writer.Plan, error) {
	if len(itemIDs) == 0 {
		return writer.Plan{}, fail("write.empty-plan")
	}
	refs := make([]claude.FactRef, 0, len(itemIDs))
	for _, id := range itemIDs {
		it, err := s.e.item(ctx, id)
		if err != nil {
			return writer.Plan{}, err
		}
		if it.Kind != model.KindClaudeFact || it.Write != model.WriteDirect {
			return writer.Plan{}, fail("plan.delete-unsupported", "title", it.Title)
		}
		refs = append(refs, claude.FactRef{File: it.File, Hash: it.Hash})
	}
	return s.e.keep(claude.PlanDeleteMany(refs))
}

// FixIndex приводит MEMORY.md каталога записи в соответствие с файлами фактов.
func (s *PlansService) FixIndex(ctx context.Context, itemID string) (writer.Plan, error) {
	it, err := s.e.item(ctx, itemID)
	if err != nil {
		return writer.Plan{}, err
	}
	if it.Agent != model.AgentClaude || (it.Kind != model.KindClaudeIndex && it.Kind != model.KindClaudeFact) {
		return writer.Plan{}, fail("plan.readonly", "title", it.Title)
	}
	return s.e.keep(claude.PlanFixIndex(filepath.Dir(it.File)))
}

// CreateNote — новая ad-hoc-заметка Codex в активном поколении: добавить или
// уточнить факт.
func (s *PlansService) CreateNote(ctx context.Context, title, body string) (writer.Plan, error) {
	g, _ := s.e.codexAdapter().Active(ctx)
	return s.e.keep(codex.PlanNewNote(s.e.rt.Config().CodexHome, g, title, body))
}

// ForgetNote — заметка, которая просит Codex исключить факт из памяти.
func (s *PlansService) ForgetNote(ctx context.Context, title, body string) (writer.Plan, error) {
	g, _ := s.e.codexAdapter().Active(ctx)
	return s.e.keep(codex.PlanForget(s.e.rt.Config().CodexHome, g, title, body))
}

// ApplyResult — коммит операции в истории и индекс после записи.
type ApplyResult struct {
	Commit string        `json:"commit"`
	Stats  library.Stats `json:"stats"`
}

// Apply применяет план, который пользователь подтвердил в диалоге с diff.
// План одноразовый: после попытки, удачной или нет, его нужно строить заново —
// так повтор после конфликта всегда покажет свежий diff.
func (s *PlansService) Apply(ctx context.Context, planID string) (ApplyResult, error) {
	p, ok := s.e.take(planID)
	if !ok {
		return ApplyResult{}, fail("plan.not-found")
	}
	w, err := s.e.writer(ctx)
	if err != nil {
		return ApplyResult{}, err
	}
	s.e.applyMu.Lock()
	res, err := w.Apply(ctx, p)
	s.e.applyMu.Unlock()
	if err != nil {
		return ApplyResult{}, s.e.error(err)
	}
	// Наблюдатель тоже пересоберёт индекс, но интерфейсу нужен свежий индекс
	// сразу после ответа, а не через задержку наблюдателя.
	st, err := s.e.lib.Rebuild(ctx)
	if err != nil {
		s.e.log.Error("пересборка после записи", "error", err)
	} else {
		s.e.emit(EventIndexUpdated, st)
	}
	return ApplyResult{Commit: res.Commit, Stats: st}, nil
}

// Discard забывает план, который пользователь не подтвердил.
func (s *PlansService) Discard(planID string) {
	s.e.take(planID)
}

// --- HistoryService ---

// HistoryService — история операций из репозитория бэкапов и откат.
type HistoryService struct {
	e *editor
}

// historyLimit — сколько операций показывает экран истории.
const historyLimit = 500

// List возвращает операции, новые первыми.
func (s *HistoryService) List(ctx context.Context) ([]writer.Entry, error) {
	if s.e.repo == nil {
		return []writer.Entry{}, nil
	}
	w := &writer.Writer{Backup: s.e.repo}
	out, err := w.History(ctx, historyLimit)
	for i := range out {
		out[i].Paths = nonNil(out[i].Paths)
	}
	return nonNil(out), s.e.error(err)
}

// Revert строит план отката операции; применяется он через PlansService.Apply.
// cancelMessage — текст отмены для заметок Codex, которые создала
// откатываемая операция (ADR-014); его даёт словарь интерфейса.
func (s *HistoryService) Revert(ctx context.Context, commit, cancelMessage string) (writer.Plan, error) {
	w, err := s.e.writer(ctx)
	if err != nil {
		return writer.Plan{}, err
	}
	p, err := w.PlanRevert(ctx, commit)
	if err != nil {
		return writer.Plan{}, s.e.error(err)
	}
	// Откат создания заметки Codex не удаляет файл (ADR-002), а переписывает
	// заметку на отмену (ADR-014). Удаление любого другого файла Codex
	// отклоняется сразу, а не после диалога с diff.
	if home := filepath.FromSlash(s.e.rt.Config().CodexHome); home != "" {
		for i, c := range p.Changes {
			if c.AfterExists || !strings.HasPrefix(c.Path, home+string(filepath.Separator)) {
				continue
			}
			if !codex.IsNote(home, codex.GenV2, c.Path) && !codex.IsNote(home, codex.GenV1, c.Path) {
				return writer.Plan{}, fail("codex.note.delete-forbidden", "path", c.Path)
			}
			if strings.TrimSpace(cancelMessage) == "" {
				return writer.Plan{}, fail("codex.note.body-empty")
			}
			p.Changes[i].AfterExists = true
			p.Changes[i].After = codex.CancelNote(c.Before, cancelMessage)
		}
	}
	return s.e.keep(p, nil)
}

func nonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
