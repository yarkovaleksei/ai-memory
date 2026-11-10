package codex

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/yarkovaleksei/ai-memory/internal/model"
)

func scan(t *testing.T, includeV1 bool) map[string][]model.Item {
	t.Helper()
	home, err := filepath.Abs(filepath.Join("..", "..", "..", "fixtures", "home", ".codex"))
	if err != nil {
		t.Fatal(err)
	}
	a := Adapter{Home: home, IncludeV1: includeV1}
	srcs, err := a.Discover(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	out := map[string][]model.Item{}
	for _, s := range srcs {
		items, err := a.Scan(context.Background(), s)
		if err != nil {
			t.Fatalf("Scan %s: %v", s.ID, err)
		}
		out[s.Label] = items
	}
	return out
}

func find(items []model.Item, kind model.ItemKind, title string) *model.Item {
	for i := range items {
		if items[i].Kind == kind && items[i].Title == title {
			return &items[i]
		}
	}
	return nil
}

func TestDiscoverV1OnlyOnRequest(t *testing.T) {
	if got := scan(t, false); len(got) != 1 || got["Codex v2"] == nil {
		t.Errorf("по умолчанию только v2, получено %v", keys(got))
	}
	if got := scan(t, true); len(got) != 2 || got["Codex v1"] == nil {
		t.Errorf("с IncludeV1 нужны v2 и v1, получено %v", keys(got))
	}
}

func TestScanV2(t *testing.T) {
	items := scan(t, false)["Codex v2"]
	demo := filepath.FromSlash("/home/me/projects/demo")

	tests := []struct {
		kind    model.ItemKind
		title   string
		write   model.WriteMode
		global  bool
		project string
	}{
		{model.KindCodexSection, "User Profile", model.WriteViaNote, true, ""},
		{model.KindCodexSection, "User preferences", model.WriteViaNote, true, ""},
		{model.KindCodexSection, "Demo — релизный процесс", model.WriteViaNote, false, demo},
		{model.KindCodexRollout, "demo release flow", model.WriteViaNote, false, demo},
		{model.KindCodexRollout, "scratch experiment", model.WriteViaNote, false, filepath.FromSlash("/tmp/scratch-42")},
		{model.KindCodexAdHocNote, "2026-01-03 — пакеты фронтенда", model.WriteDirect, true, ""},
	}
	for _, tt := range tests {
		t.Run(tt.title, func(t *testing.T) {
			it := find(items, tt.kind, tt.title)
			if it == nil {
				t.Fatalf("нет записи %s %q", tt.kind, tt.title)
			}
			if it.Write != tt.write || it.Project.Global != tt.global || it.Project.Path != tt.project {
				t.Errorf("write=%s project=%+v", it.Write, it.Project)
			}
		})
	}

	if find(items, model.KindCodexSection, "What's in Memory") != nil {
		t.Error("«What's in Memory» должен быть разбит на группы, а не идти одной записью")
	}
	scratch := find(items, model.KindCodexRollout, "scratch experiment")
	if !scratch.Project.Temp {
		t.Error("rollout из /tmp должен относиться к временному проекту")
	}
	release := find(items, model.KindCodexRollout, "demo release flow")
	if release.Frontmatter["git_branch"] != "main" || release.Body == "" {
		t.Errorf("заголовок rollout-сводки разобран неверно: %+v", release.Frontmatter)
	}
	group := find(items, model.KindCodexSection, "Demo — релизный процесс")
	if len(group.Links) != 1 || group.Links[0].TargetID != release.ID {
		t.Errorf("ссылка группы на rollout-сводку не разрешилась: %+v", group.Links)
	}
}

func TestScanV1IsReadOnly(t *testing.T) {
	items := scan(t, true)["Codex v1"]
	if len(items) == 0 {
		t.Fatal("v1 пуст")
	}
	for _, it := range items {
		if it.Write != model.WriteNone {
			t.Errorf("%s %q: write = %s, в v1 всё только для чтения", it.Kind, it.Title, it.Write)
		}
	}
	task := find(items, model.KindCodexSection, "Demo — старая задача")
	if task == nil || task.Project.Path != filepath.FromSlash("/home/me/projects/demo") {
		t.Errorf("группа задач v1: %+v", task)
	}
	if find(items, model.KindCodexRaw, "raw_memories.md") == nil {
		t.Error("нет raw_memories.md")
	}
}

func TestSplitHeaderWithoutHeader(t *testing.T) {
	h, body := splitHeader("Просто текст без заголовка\n\nВторой абзац")
	if len(h) != 0 || body != "Просто текст без заголовка\n\nВторой абзац" {
		t.Errorf("h=%v body=%q", h, body)
	}
}

func keys(m map[string][]model.Item) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	return out
}
