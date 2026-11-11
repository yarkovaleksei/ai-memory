package library

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yarkovaleksei/ai-memory/internal/index"
	"github.com/yarkovaleksei/ai-memory/internal/model"
)

// relocate переписывает выдуманный дом /home/me в копии фикстур на путь самой
// копии: тогда проект /home/me/projects/demo из памяти указывает на
// существующий каталог с инструкциями и навыками.
func relocate(t *testing.T, home string) {
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
			to = strings.ReplaceAll(to, `\`, `\\`) // путь внутри JSON-строки
		}
		out := strings.ReplaceAll(string(data), "/home/me/", to)
		if out == string(data) {
			return nil
		}
		return os.WriteFile(p, []byte(out), 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
}

// Инструкции и навыки проекта находятся по пути, восстановленному из памяти,
// и попадают в тот же проект, что и его память.
func TestRebuildReadsProjectFiles(t *testing.T) {
	home := copyFixtures(t)
	relocate(t, home)
	cfg := configFor(home, t.TempDir())
	cfg.AgentsDir = filepath.ToSlash(filepath.Join(home, ".agents"))
	lib := openLibrary(t, cfg)
	ctx := context.Background()
	if _, err := lib.Rebuild(ctx); err != nil {
		t.Fatal(err)
	}

	demo := filepath.Join(home, "projects", "demo")
	projects, err := lib.Projects(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var project model.Project
	for _, p := range projects {
		if p.Path == demo {
			project = p
		}
	}
	if project.ID == "" {
		t.Fatalf("проект %s не найден среди %+v", demo, projects)
	}

	page, err := lib.Search(ctx, index.Query{ProjectID: project.ID})
	if err != nil {
		t.Fatal(err)
	}
	kinds := map[model.ItemKind]int{}
	for _, h := range page.Hits {
		kinds[h.Kind]++
	}
	// CLAUDE.md, CLAUDE.local.md, правило, AGENTS.override.md, AGENTS.md; навыки
	// deploy-preview и changelog — рядом с фактами памяти того же проекта.
	if kinds[model.KindInstructions] != 5 || kinds[model.KindSkill] != 2 || kinds[model.KindClaudeFact] == 0 {
		t.Errorf("виды записей проекта: %v", kinds)
	}

	// Глобальные навыки: свои у профилей и CODEX_HOME, общий ~/.agents,
	// синхронизированный и системный — только чтение.
	skills, err := lib.Search(ctx, index.Query{ProjectID: "global", Kind: string(model.KindSkill)})
	if err != nil {
		t.Fatal(err)
	}
	readOnly := 0
	for _, h := range skills.Hits {
		if h.Write == model.WriteNone {
			readOnly++
		}
	}
	if len(skills.Hits) != 6 || readOnly != 2 {
		t.Errorf("глобальных навыков %d, из них только для чтения %d; want 6 и 2", len(skills.Hits), readOnly)
	}
}

func TestRelevant(t *testing.T) {
	filters := map[string]map[string]bool{
		filepath.FromSlash("/p/.claude"): {"CLAUDE.md": true, "rules": true},
		filepath.FromSlash("/p/memory"):  nil,
	}
	tests := []struct {
		name string
		want bool
	}{
		{"/p/.claude/CLAUDE.md", true},
		{"/p/.claude/history.jsonl", false},
		{"/p/.claude/rules", true},
		{"/p/memory/fact.md", true},
		{"/other/file", true}, // каталог без записи в фильтрах: важно всё
	}
	for _, tt := range tests {
		if got := relevant(filters, filepath.FromSlash(tt.name)); got != tt.want {
			t.Errorf("relevant(%s) = %v, want %v", tt.name, got, tt.want)
		}
	}
}

// Проект без памяти Claude, но с сессиями: его инструкции видны (ADR-025).
func TestRebuildReadsProjectWithoutMemory(t *testing.T) {
	home := copyFixtures(t)
	relocate(t, home)
	cfg := configFor(home, t.TempDir())
	lib := openLibrary(t, cfg)
	ctx := context.Background()
	if _, err := lib.Rebuild(ctx); err != nil {
		t.Fatal(err)
	}
	empty := filepath.Join(home, "projects", "empty")
	projects, err := lib.Projects(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range projects {
		if p.Path != empty {
			continue
		}
		page, err := lib.Search(ctx, index.Query{ProjectID: p.ID})
		if err != nil {
			t.Fatal(err)
		}
		if len(page.Hits) != 1 || page.Hits[0].Kind != model.KindInstructions || page.Hits[0].Title != "CLAUDE.md" {
			t.Errorf("записи проекта без памяти: %+v", page.Hits)
		}
		return
	}
	t.Fatalf("проекта %s нет среди %+v", empty, projects)
}
