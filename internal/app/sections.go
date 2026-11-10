package app

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/yarkovaleksei/ai-memory/internal/adapter"
	"github.com/yarkovaleksei/ai-memory/internal/adapter/agentfiles"
	"github.com/yarkovaleksei/ai-memory/internal/index"
	"github.com/yarkovaleksei/ai-memory/internal/model"
	"github.com/yarkovaleksei/ai-memory/internal/writer"
)

// Перенос правила между файлами инструкций (ADR-021): раздел по заголовку
// вставляется в другой файл того же проекта — обычно из CLAUDE.md в AGENTS.md
// и обратно. Цель выбирается из того, что посчитал Go: существующий файл
// инструкций по ID записи или новый файл по ключу «new:<хранилище>». Путь
// нового файла интерфейс не передаёт.

const newTargetPrefix = "new:"

// SectionsDraft — разделы файла инструкций и хэш, при котором они посчитаны.
type SectionsDraft struct {
	Hash     string               `json:"hash"`
	Sections []agentfiles.Section `json:"sections"`
}

// SectionTarget — куда можно вставить раздел.
type SectionTarget struct {
	Key    string      `json:"key"` // ID записи или «new:<ID хранилища>»
	Agent  model.Agent `json:"agent"`
	Title  string      `json:"title"`
	File   string      `json:"file"` // только для показа
	Role   string      `json:"role"`
	Exists bool        `json:"exists"` // false — файл будет создан
	Repo   string      `json:"repo"`
	SectionsDraft
}

// InstructionSections режет файл инструкций на разделы. Читает диск, а не
// индекс: номера разделов должны совпасть с тем, что увидит план.
func (s *PlansService) InstructionSections(ctx context.Context, itemID string) (SectionsDraft, error) {
	it, err := s.e.item(ctx, itemID)
	if err != nil {
		return SectionsDraft{}, err
	}
	if it.Kind != model.KindInstructions {
		return SectionsDraft{}, fail("section.not-instructions", "title", it.Title)
	}
	return readSections(it.File)
}

func readSections(file string) (SectionsDraft, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		return SectionsDraft{}, fail("write.unreadable", "path", file, "error", err.Error())
	}
	return SectionsDraft{Hash: model.ContentHash(data), Sections: agentfiles.Sections(string(data))}, nil
}

// SectionTargets — файлы инструкций того же проекта (или глобальные), в
// которые можно вставить раздел, и файлы, которые можно для этого создать:
// AGENTS.md, если у проекта нет инструкций Codex, CLAUDE.md — если нет
// инструкций Claude. Сначала — файлы другого агента.
func (s *PlansService) SectionTargets(ctx context.Context, itemID string) ([]SectionTarget, error) {
	it, err := s.e.item(ctx, itemID)
	if err != nil {
		return nil, err
	}
	if it.Kind != model.KindInstructions {
		return nil, fail("section.not-instructions", "title", it.Title)
	}
	page, err := s.e.lib.Search(ctx, index.Query{ProjectID: it.ProjectID, Kind: string(model.KindInstructions), IncludeTemp: true})
	if err != nil {
		return nil, s.e.error(err)
	}
	out := []SectionTarget{}
	for _, h := range page.Hits {
		if h.ID == it.ID || h.Write != model.WriteDirect {
			continue
		}
		d, err := readSections(h.File)
		if err != nil {
			continue // файл пропал после индексации — его не предлагаем
		}
		out = append(out, SectionTarget{Key: h.ID, Agent: h.Agent, Title: h.Title, File: h.File, Role: h.Role,
			Exists: true, Repo: h.Repo, SectionsDraft: d})
	}
	news, err := s.newTargets(ctx, it)
	if err != nil {
		return nil, err
	}
	for _, n := range news {
		out = append(out, n.SectionTarget)
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if (a.Agent != it.Agent) != (b.Agent != it.Agent) {
			return a.Agent != it.Agent
		}
		if (a.Role == agentfiles.RoleMain) != (b.Role == agentfiles.RoleMain) {
			return a.Role == agentfiles.RoleMain
		}
		return a.Title < b.Title
	})
	return out, nil
}

type newTarget struct {
	SectionTarget
	path string
}

// newTargets — файлы инструкций, которых ещё нет: главный файл агента в
// каталоге проекта, в CODEX_HOME или в профиле Claude.
func (s *PlansService) newTargets(ctx context.Context, it model.Item) ([]newTarget, error) {
	var out []newTarget
	add := func(sourceID string, agent model.Agent, dir, name string, others ...string) {
		if dir == "" || !adapter.IsDir(dir) {
			return
		}
		for _, n := range append([]string{name}, others...) {
			if _, err := os.Lstat(filepath.Join(dir, n)); err == nil {
				return
			}
		}
		path := filepath.Join(dir, name)
		out = append(out, newTarget{path: path, SectionTarget: SectionTarget{
			Key: newTargetPrefix + sourceID, Agent: agent, Title: name, File: path, Role: agentfiles.RoleMain,
			SectionsDraft: SectionsDraft{Sections: []agentfiles.Section{}},
		}})
	}
	projects, err := s.e.lib.Projects(ctx)
	if err != nil {
		return nil, s.e.error(err)
	}
	var project model.Project
	for _, p := range projects {
		if p.ID == it.ProjectID {
			project = p
		}
	}
	switch {
	case project.Global:
		srcs, err := s.e.lib.Sources(ctx)
		if err != nil {
			return nil, s.e.error(err)
		}
		for _, src := range srcs {
			if !src.Files || src.Root == "" {
				continue
			}
			switch {
			case src.ID == agentfiles.CodexFilesID(src.Root):
				add(src.ID, model.AgentCodex, src.Root, agentfiles.AgentsMD, agentfiles.AgentsOverMD)
			case src.ID == agentfiles.ClaudeFilesID(src.Root):
				add(src.ID, model.AgentClaude, src.Root, agentfiles.ClaudeMD)
			}
		}
	case project.Path != "":
		add(agentfiles.CodexProjects, model.AgentCodex, project.Path, agentfiles.AgentsMD, agentfiles.AgentsOverMD)
		add(agentfiles.ClaudeProjects, model.AgentClaude, project.Path, agentfiles.ClaudeMD,
			filepath.Join(".claude", agentfiles.ClaudeMD))
	}
	return out, nil
}

// CopySection строит план: раздел section файла itemID (хэш hash) — в цель
// targetKey (хэш targetHash; для нового файла пусто) в конец её раздела after
// или в конец файла (after < 0). move — убрать раздел из исходного файла.
func (s *PlansService) CopySection(ctx context.Context, itemID, hash string, section int,
	targetKey, targetHash string, after int, move bool) (writer.Plan, error) {
	it, err := s.e.item(ctx, itemID)
	if err != nil {
		return writer.Plan{}, err
	}
	if it.Kind != model.KindInstructions {
		return writer.Plan{}, fail("section.not-instructions", "title", it.Title)
	}
	if move && it.Write != model.WriteDirect {
		return writer.Plan{}, fail("plan.readonly", "title", it.Title)
	}
	c := agentfiles.SectionCopy{From: it, FromHash: hash, Section: section, ToHash: targetHash, After: after, Move: move}
	if strings.HasPrefix(targetKey, newTargetPrefix) {
		news, err := s.newTargets(ctx, it)
		if err != nil {
			return writer.Plan{}, err
		}
		for _, n := range news {
			if n.Key == targetKey {
				c.NewFile, c.NewAgent = n.path, n.Agent
			}
		}
		if c.NewFile == "" {
			return writer.Plan{}, fail("section.target-invalid")
		}
	} else {
		to, err := s.e.item(ctx, targetKey)
		if err != nil {
			return writer.Plan{}, err
		}
		if to.ProjectID != it.ProjectID {
			return writer.Plan{}, fail("section.target-invalid")
		}
		c.To = to
	}
	return s.e.keep(agentfiles.PlanSection(c))
}
