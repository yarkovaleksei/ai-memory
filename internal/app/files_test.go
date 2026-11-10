package app

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yarkovaleksei/ai-memory/internal/adapter/agentfiles"
	"github.com/yarkovaleksei/ai-memory/internal/model"
	"github.com/yarkovaleksei/ai-memory/internal/writer"
)

// relocateHome заменяет выдуманный дом /home/me путём копии фикстур: проект
// /home/me/projects/demo из памяти указывает на её каталог projects/demo.
func relocateHome(t *testing.T, home string) {
	t.Helper()
	err := filepath.WalkDir(home, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		to := home + string(filepath.Separator)
		if strings.HasSuffix(p, ".jsonl") {
			to = strings.ReplaceAll(to, `\`, `\\`)
		}
		if out := strings.ReplaceAll(string(data), "/home/me/", to); out != string(data) {
			return os.WriteFile(p, []byte(out), 0o644)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func warningCodes(p writer.Plan) string {
	var out []string
	for _, w := range p.Warnings {
		c := w.Code
		if w.Confirm {
			c += "!"
		}
		out = append(out, c)
	}
	return strings.Join(out, ",")
}

// Инструкции и навыки правятся тем же путём, что память: план с diff, запись,
// откат байт в байт. План предупреждает о чужом git-репозитории и об общем
// навыке, а writer пускает в каталоги проектов, ~/.agents и цели симлинков.
func TestEditInstructionsAndSkills(t *testing.T) {
	var outside string
	e := newEditEnvWith(t, func(home string) {
		relocateHome(t, home)
		// Проект в git-репозитории.
		if err := os.MkdirAll(filepath.Join(home, "projects", "demo", ".git"), 0o755); err != nil {
			t.Fatal(err)
		}
		// Навык профиля Claude — симлинк на каталог вне всех корней.
		outside = filepath.Join(filepath.Dir(home), "elsewhere", "shared-skill")
		if err := os.MkdirAll(outside, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(outside, "SKILL.md"), []byte("---\nname: shared-skill\ndescription: Общий.\n---\n\n# Общий\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outside, filepath.Join(home, ".claude", "skills", "shared-skill")); err != nil {
			t.Skipf("симлинки недоступны: %v", err)
		}
	})
	ctx := context.Background()

	projectAgents := ""
	for _, h := range e.searchKind(t, model.KindInstructions) {
		if h.Title == "AGENTS.md" && h.Repo != "" {
			projectAgents = h.ID
		}
	}
	if projectAgents == "" {
		t.Fatal("AGENTS.md проекта в git-репозитории не найден")
	}

	tests := []struct {
		name     string
		id       string
		warnings string
		op       string
	}{
		{"AGENTS.md в CODEX_HOME", e.findIn(t, model.KindInstructions, "AGENTS.md", ".codex").ID, "", "instructions.update"},
		{"AGENTS.md проекта в git", projectAgents, agentfiles.WarnInRepo, "instructions.update"},
		{"навык в ~/.agents", e.find(t, model.KindSkill, "commit-message").ID, "", "skill.update"},
		{"навык CODEX_HOME", e.find(t, model.KindSkill, "lint-fix").ID, "", "skill.update"},
		{"общий навык вне корней", e.find(t, model.KindSkill, "shared-skill").ID, agentfiles.WarnSharedSkill + "!", "skill.update"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			it, err := e.lib.Get(ctx, tt.id)
			if err != nil {
				t.Fatal(err)
			}
			d, err := e.plans.Draft(ctx, it.ID)
			if err != nil {
				t.Fatalf("Draft: %v", err)
			}
			before := readFile(t, it.File)
			if d.Content != before || d.Fact != nil {
				t.Fatalf("Draft: %+v", d)
			}
			p, err := e.plans.UpdateSource(ctx, it.ID, d.Hash, before+"\n- Новое правило.\n")
			if err != nil {
				t.Fatalf("UpdateSource: %v", err)
			}
			if p.Op.Code != tt.op || warningCodes(p) != tt.warnings {
				t.Errorf("операция %s, предупреждения %q; want %s, %q", p.Op.Code, warningCodes(p), tt.op, tt.warnings)
			}
			res := e.apply(t)(p, nil)
			if !strings.HasSuffix(readFile(t, it.File), "- Новое правило.\n") {
				t.Fatal("правка не записана")
			}
			e.apply(t)(e.history.Revert(ctx, res.Commit, ""))
			if readFile(t, it.File) != before {
				t.Error("откат не вернул файл байт в байт")
			}
		})
	}
	if !strings.Contains(readFile(t, filepath.Join(outside, "SKILL.md")), "# Общий") {
		t.Error("каталог общего навыка испорчен")
	}
}

// Навыки и файлы только для чтения план не строит.
func TestEditReadOnlyFiles(t *testing.T) {
	e := newEditEnv(t)
	ctx := context.Background()
	for _, title := range []string{"plan-helper", "report-writer"} {
		it := e.find(t, model.KindSkill, title)
		if _, err := e.plans.Draft(ctx, it.ID); errCode(err) != "plan.readonly" {
			t.Errorf("%s: Draft = %v", title, err)
		}
		if _, err := e.plans.UpdateSource(ctx, it.ID, it.Hash, "x"); errCode(err) != "plan.readonly" {
			t.Errorf("%s: UpdateSource = %v", title, err)
		}
	}
}
