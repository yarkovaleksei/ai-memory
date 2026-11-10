package app

import (
	"context"

	"github.com/yarkovaleksei/ai-memory/internal/adapter"
	"github.com/yarkovaleksei/ai-memory/internal/adapter/agentfiles"
	"github.com/yarkovaleksei/ai-memory/internal/writer"
)

// Приём «AGENTS.md — основной, CLAUDE.md импортирует его» для проекта
// (ADR-022). Интерфейс передаёт ID проекта, хэши и номера блоков; каталог
// проекта берётся из индекса.

// UnifyDraft — состояние инструкций проекта для предложения.
func (s *PlansService) UnifyDraft(ctx context.Context, projectID string) (agentfiles.UnifyState, error) {
	dir, err := s.projectDir(ctx, projectID)
	if err != nil {
		return agentfiles.UnifyState{}, err
	}
	st, err := agentfiles.ReadUnify(dir)
	if err != nil {
		return agentfiles.UnifyState{}, s.e.error(err)
	}
	return st, nil
}

// Unify строит план приёма: блоки move из CLAUDE.md — в AGENTS.md, в
// CLAUDE.md — импорт.
func (s *PlansService) Unify(ctx context.Context, projectID, claudeHash, agentsHash string, move []int) (writer.Plan, error) {
	dir, err := s.projectDir(ctx, projectID)
	if err != nil {
		return writer.Plan{}, err
	}
	return s.e.keep(agentfiles.PlanUnify(dir, claudeHash, agentsHash, move))
}

// projectDir — существующий каталог проекта из индекса.
func (s *PlansService) projectDir(ctx context.Context, projectID string) (string, error) {
	projects, err := s.e.lib.Projects(ctx)
	if err != nil {
		return "", s.e.error(err)
	}
	for _, p := range projects {
		if p.ID == projectID && !p.Global && p.Path != "" && adapter.IsDir(p.Path) {
			return p.Path, nil
		}
	}
	return "", fail("unify.project-required")
}
