package similar

import (
	"context"
	"path/filepath"
	"sort"
	"testing"

	"github.com/yarkovaleksei/ai-memory/internal/config"
	"github.com/yarkovaleksei/ai-memory/internal/library"
	"github.com/yarkovaleksei/ai-memory/internal/model"
)

func fixtureItems(t *testing.T) ([]model.Item, map[string]string) {
	t.Helper()
	home, err := filepath.Abs(filepath.Join("..", "..", "fixtures", "home"))
	if err != nil {
		t.Fatal(err)
	}
	lib, err := library.New(config.Config{
		DataDir:        t.TempDir(),
		ClaudeProfiles: []string{filepath.Join(home, ".claude")},
		CodexHome:      filepath.Join(home, ".codex"),
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = lib.Close() })
	if _, err := lib.Rebuild(context.Background()); err != nil {
		t.Fatal(err)
	}
	items, err := lib.All(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	titles := map[string]string{}
	for _, it := range items {
		titles[it.ID] = it.Title
	}
	return items, titles
}

func titlesOf(g Group, titles map[string]string) []string {
	var out []string
	for _, id := range g.ItemIDs {
		out = append(out, titles[id])
	}
	sort.Strings(out)
	return out
}

func TestFindOnFixtures(t *testing.T) {
	items, titles := fixtureItems(t)
	groups := Find(items, Options{MinScore: DefaultMinScore})
	want := [][]string{
		{"2026-01-03 — пакеты фронтенда", "User preferences", "Пакеты только через yarn"},
		{"Demo — релизный процесс", "General Tips", "Как выпускается релиз"},
	}
	if len(groups) != len(want) {
		for _, g := range groups {
			t.Logf("группа %.3f: %v", g.Score, titlesOf(g, titles))
		}
		t.Fatalf("групп %d, ожидалось %d", len(groups), len(want))
	}
	for i, w := range want {
		got := titlesOf(groups[i], titles)
		if len(got) != len(w) {
			t.Errorf("группа %d: %v, want %v", i, got, w)
			continue
		}
		for j := range w {
			if got[j] != w[j] {
				t.Errorf("группа %d: %v, want %v", i, got, w)
				break
			}
		}
	}
	// Заметка и раздел сводки Codex рядом — это консолидация, а не дубль.
	for _, g := range groups {
		for _, p := range g.Pairs {
			if titles[p.A] == "User preferences" && titles[p.B] == "2026-01-03 — пакеты фронтенда" ||
				titles[p.B] == "User preferences" && titles[p.A] == "2026-01-03 — пакеты фронтенда" {
				t.Error("пара заметка — раздел сводки Codex не должна считаться дублем")
			}
		}
	}
}

func TestFindOptions(t *testing.T) {
	mk := func(id string, agent model.Agent, kind model.ItemKind, project, body string) model.Item {
		return model.Item{ID: id, Agent: agent, Kind: kind, ProjectID: project, File: "/" + id, SourceID: string(agent), Title: id, Body: body}
	}
	items := []model.Item{
		mk("c1", model.AgentClaude, model.KindClaudeFact, "p1", "Релиз выпускается тегом на ветке main"),
		mk("c2", model.AgentClaude, model.KindClaudeFact, "p1", "Релиз выпускается тегом на ветке main проекта"),
		mk("x1", model.AgentCodex, model.KindCodexAdHocNote, "global", "Релиз выпускается тегом на main"),
		mk("other", model.AgentClaude, model.KindClaudeFact, "p2", "Совсем про другое: кошки и собаки"),
		{ID: "idx", Agent: model.AgentClaude, Kind: model.KindClaudeIndex, File: "/idx", Body: "Релиз выпускается тегом на ветке main"},
		{ID: "tmp", Agent: model.AgentClaude, Kind: model.KindClaudeFact, File: "/tmp", Body: "Релиз выпускается тегом на ветке main", Project: model.ProjectRef{Temp: true}},
	}
	tests := []struct {
		name string
		opt  Options
		want int // записей в первой группе
	}{
		{"все", Options{MinScore: 0.2}, 3},
		{"только между агентами", Options{MinScore: 0.2, CrossAgent: true}, 3},
		{"по проекту без совпадений", Options{MinScore: 0.2, ProjectID: "p2"}, 0},
		{"высокий порог", Options{MinScore: 0.99}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			groups := Find(items, tt.opt)
			got := 0
			if len(groups) > 0 {
				got = len(groups[0].ItemIDs)
				for _, id := range groups[0].ItemIDs {
					if id == "idx" || id == "tmp" || id == "other" {
						t.Errorf("в группе лишняя запись %s", id)
					}
				}
			}
			if got != tt.want {
				t.Errorf("записей в группе: %d, want %d (%+v)", got, tt.want, groups)
			}
		})
	}
	if g := Find(items, Options{MinScore: 0.2, CrossAgent: true}); len(g) > 0 {
		for _, p := range g[0].Pairs {
			if (p.A == "c1" || p.A == "c2") && (p.B == "c1" || p.B == "c2") {
				t.Error("пара одного агента при CrossAgent")
			}
		}
	}
}
