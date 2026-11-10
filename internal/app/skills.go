package app

import (
	"context"
	"path/filepath"

	"github.com/yarkovaleksei/ai-memory/internal/adapter/agentfiles"
	"github.com/yarkovaleksei/ai-memory/internal/model"
	"github.com/yarkovaleksei/ai-memory/internal/writer"
)

// Установка навыка в другого агента (ADR-023). Цель выбирается ключом из
// SkillTargets; каталог цели Go считает сам, интерфейс путей не передаёт.

// Ключи целей установки навыка.
const (
	skillToAgents        = "agents"         // ~/.agents/skills — основной путь Codex
	skillToCodexHome     = "codex-home"     // CODEX_HOME/skills — устаревший путь Codex
	skillToProjectAgents = "project-agents" // <проект>/.agents/skills
	skillToProjectClaude = "project-claude" // <проект>/.claude/skills
	skillToProfile       = "profile:"       // + ID хранилища файлов профиля Claude
)

// SkillTarget — куда можно поставить навык.
type SkillTarget struct {
	Key         string      `json:"key"`
	Agent       model.Agent `json:"agent"`
	Kind        string      `json:"kind"` // ключ без ID хранилища: для подписи в интерфейсе
	Label       string      `json:"label"`
	Dir         string      `json:"dir"`         // только для показа
	Exists      bool        `json:"exists"`      // навык с таким именем там уже есть
	Recommended bool        `json:"recommended"` // основной путь агента
}

// SkillTargets — места навыков другого агента того же уровня: глобальные
// для глобального навыка, каталог проекта для проектного.
func (s *PlansService) SkillTargets(ctx context.Context, itemID string) ([]SkillTarget, error) {
	it, err := s.e.item(ctx, itemID)
	if err != nil {
		return nil, err
	}
	return s.skillTargets(ctx, it)
}

func (s *PlansService) skillTargets(ctx context.Context, it model.Item) ([]SkillTarget, error) {
	if it.Kind != model.KindSkill {
		return nil, fail("skill.not-skill", "title", it.Title)
	}
	name := filepath.Base(filepath.Dir(it.File))
	out := []SkillTarget{}
	add := func(key, kind string, agent model.Agent, label, root string, recommended bool) {
		if root == "" {
			return
		}
		dir := filepath.Join(root, name)
		out = append(out, SkillTarget{Key: key, Kind: kind, Agent: agent, Label: label, Dir: dir,
			Exists: agentfiles.Occupied(dir), Recommended: recommended})
	}
	cfg := s.e.rt.Config()
	project := ""
	if it.ProjectID != "global" {
		dir, err := s.projectDir(ctx, it.ProjectID)
		if err != nil {
			return nil, fail("skill.target-invalid")
		}
		project = dir
	}
	switch {
	case it.Agent == model.AgentClaude && project == "":
		if cfg.AgentsDir != "" {
			add(skillToAgents, skillToAgents, model.AgentCodex, "", filepath.Join(filepath.FromSlash(cfg.AgentsDir), "skills"), true)
		}
		if cfg.CodexHome != "" {
			add(skillToCodexHome, skillToCodexHome, model.AgentCodex, "", filepath.Join(filepath.FromSlash(cfg.CodexHome), "skills"), false)
		}
	case it.Agent == model.AgentClaude:
		add(skillToProjectAgents, skillToProjectAgents, model.AgentCodex, "", filepath.Join(project, ".agents", "skills"), true)
	case project == "":
		srcs, err := s.e.lib.Sources(ctx)
		if err != nil {
			return nil, s.e.error(err)
		}
		for _, src := range srcs {
			if src.Files && src.Root != "" && src.ID == agentfiles.ClaudeFilesID(src.Root) {
				add(skillToProfile+src.ID, "profile", model.AgentClaude, src.Label, filepath.Join(src.Root, "skills"), len(out) == 0)
			}
		}
	default:
		add(skillToProjectClaude, skillToProjectClaude, model.AgentClaude, "", filepath.Join(project, ".claude", "skills"), true)
	}
	return out, nil
}

// InstallSkill строит план установки навыка в цель key копией или симлинком.
func (s *PlansService) InstallSkill(ctx context.Context, itemID, key string, link bool) (writer.Plan, error) {
	it, err := s.e.item(ctx, itemID)
	if err != nil {
		return writer.Plan{}, err
	}
	targets, err := s.skillTargets(ctx, it)
	if err != nil {
		return writer.Plan{}, err
	}
	for _, t := range targets {
		if t.Key == key {
			return s.e.keep(agentfiles.PlanInstallSkill(agentfiles.SkillInstall{From: it, Dir: t.Dir, Agent: t.Agent, Link: link}))
		}
	}
	return writer.Plan{}, fail("skill.target-invalid")
}
