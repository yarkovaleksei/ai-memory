package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/yarkovaleksei/ai-memory/internal/adapter/agentfiles"
	"github.com/yarkovaleksei/ai-memory/internal/model"
)

// Приём для проекта demo: CLAUDE.md без импорта, общий раздел уходит в
// AGENTS.md, Claude-специфичный остаётся. Откат возвращает оба файла.
func TestUnifyProject(t *testing.T) {
	e := newEditEnvWith(t, func(home string) {
		relocateHome(t, home)
		// В фикстуре CLAUDE.md уже импортирует AGENTS.md — убираем импорт.
		p := filepath.Join(home, "projects", "demo", "CLAUDE.md")
		data, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		out := []byte("# Проект demo (фикстура)\n\n## Пакеты\n\n- Только yarn.\n" + string(data[len("# Проект demo (фикстура)\n\n@AGENTS.md\n"):]))
		if err := os.WriteFile(p, out, 0o644); err != nil {
			t.Fatal(err)
		}
	})
	ctx := context.Background()
	project := e.findIn(t, model.KindInstructions, "CLAUDE.md", "demo").ProjectID

	st, err := e.plans.UnifyDraft(ctx, project)
	if err != nil {
		t.Fatal(err)
	}
	if st.State != agentfiles.UnifyBoth || st.Import != "@AGENTS.md" || !st.Override || len(st.Blocks) != 3 {
		t.Fatalf("состояние: %+v", st)
	}
	claudeBefore, agentsBefore := readFile(t, st.ClaudeFile), readFile(t, st.AgentsFile)
	p, err := e.plans.Unify(ctx, project, st.ClaudeHash, st.AgentsHash, []int{1})
	if err != nil {
		t.Fatal(err)
	}
	if p.Op.Code != "instructions.unify" || warningCodes(p) != agentfiles.WarnShadowed {
		t.Errorf("операция %s, предупреждения %q", p.Op.Code, warningCodes(p))
	}
	res := e.apply(t)(p, nil)
	if got := readFile(t, st.ClaudeFile); got != "# Проект demo (фикстура)\n\n@AGENTS.md\n\n## Только для Claude Code\n\n- Тесты запускай через `yarn test`.\n" {
		t.Errorf("CLAUDE.md после приёма: %q", got)
	}
	if after, _ := e.plans.UnifyDraft(ctx, project); after.State != agentfiles.UnifyDone {
		t.Errorf("после приёма состояние %s", after.State)
	}
	e.apply(t)(e.history.Revert(ctx, res.Commit, ""))
	if readFile(t, st.ClaudeFile) != claudeBefore || readFile(t, st.AgentsFile) != agentsBefore {
		t.Error("откат не вернул файлы байт в байт")
	}

	if _, err := e.plans.UnifyDraft(ctx, "global"); errCode(err) != "unify.project-required" {
		t.Errorf("глобальная память: %v", err)
	}
}
