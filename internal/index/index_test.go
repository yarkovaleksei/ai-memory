package index

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/yarkovaleksei/ai-memory/internal/model"
)

func testIndex(t *testing.T) *Index {
	t.Helper()
	ix, err := Open(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ix.Close() })

	at := func(d int) time.Time { return time.Date(2026, 1, d, 10, 0, 0, 0, time.UTC) }
	items := []model.Item{
		{ID: "a", Agent: model.AgentClaude, Kind: model.KindClaudeFact, Type: "feedback", ProjectID: "p1",
			Title: "Базовые ветки", Body: "Нацеливай PR на ветку main", ModTime: at(1)},
		{ID: "b", Agent: model.AgentCodex, Kind: model.KindCodexRollout, ProjectID: "p1",
			Title: "Ёлка", Body: "запись про ёжика", ModTime: at(3),
			Warnings: []model.Warning{{Code: "x"}}},
		{ID: "c", Agent: model.AgentClaude, Kind: model.KindClaudeFact, Type: "project", ProjectID: "tmp",
			Title: "Черновик", Body: "временная ветка", ModTime: at(2), Project: model.ProjectRef{Temp: true}},
		{ID: "d", Agent: model.AgentClaude, Kind: model.KindInstructions, ProjectID: "p1",
			Title: "CLAUDE.md", Body: "Правила проекта", ModTime: at(4)},
	}
	projects := []model.Project{{ID: "p1", Label: "demo"}, {ID: "tmp", Label: "s", Temp: true}}
	sources := []model.Source{{ID: "claude:x"}, {ID: "codex:y"}}
	if err := ix.Replace(context.Background(), sources, projects, items); err != nil {
		t.Fatal(err)
	}
	return ix
}

func ids(p Page) []string {
	var out []string
	for _, h := range p.Hits {
		out = append(out, h.ID)
	}
	return out
}

func TestSearch(t *testing.T) {
	ix := testIndex(t)
	tests := []struct {
		name  string
		q     Query
		want  []string
		total int
	}{
		{"без фильтров скрывает временные, новые сверху", Query{}, []string{"d", "b", "a"}, 3},
		{"с временными", Query{IncludeTemp: true}, []string{"d", "b", "c", "a"}, 4},
		{"вся память без инструкций и навыков", Query{Kind: KindMemory}, []string{"b", "a"}, 2},
		{"только инструкции", Query{Kind: string(model.KindInstructions)}, []string{"d"}, 1},
		{"явно выбранный временный проект", Query{ProjectID: "tmp"}, []string{"c"}, 1},
		{"несколько проектов, временный — явно", Query{ProjectIDs: []string{"p1", "tmp"}, Kind: KindMemory}, []string{"b", "c", "a"}, 3},
		{"несколько проектов и один из них", Query{ProjectIDs: []string{"p1", "tmp"}, ProjectID: "tmp"}, []string{"c"}, 1},
		{"неизвестный проект в списке", Query{ProjectIDs: []string{"nope"}}, nil, 0},
		{"агент", Query{Agent: "codex"}, []string{"b"}, 1},
		{"тип", Query{Type: "feedback"}, []string{"a"}, 1},
		{"только с предупреждениями", Query{WarningsOnly: true}, []string{"b"}, 1},
		{"поиск по началу слова", Query{Text: "ветк", IncludeTemp: true}, nil, 2},
		{"ё ищется как е и наоборот", Query{Text: "елка"}, []string{"b"}, 1},
		{"ё в запросе", Query{Text: "ёжик"}, []string{"b"}, 1},
		{"слова через AND", Query{Text: "ветк main"}, []string{"a"}, 1},
		{"кавычки и звёздочки из ввода не ломают запрос", Query{Text: `"ветк*`, IncludeTemp: true}, nil, 2},
		{"OR — обычное слово, а не оператор", Query{Text: "ветк OR"}, nil, 0},
		{"сортировка по названию по алфавиту, ё как е", Query{Sort: "title", IncludeTemp: true}, []string{"d", "a", "b", "c"}, 4},
		{"страница", Query{Sort: "title", IncludeTemp: true, Offset: 1, Limit: 1}, []string{"a"}, 4},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			page, err := ix.Search(context.Background(), tt.q)
			if err != nil {
				t.Fatal(err)
			}
			if page.Total != tt.total {
				t.Errorf("total = %d, want %d", page.Total, tt.total)
			}
			if tt.want != nil && !slices.Equal(ids(page), tt.want) {
				t.Errorf("ids = %v, want %v", ids(page), tt.want)
			}
			for _, h := range page.Hits {
				if h.Body != "" {
					t.Error("в списке не должно быть тела записи")
				}
			}
		})
	}
}

func TestSearchSnippet(t *testing.T) {
	ix := testIndex(t)
	page, err := ix.Search(context.Background(), Query{Text: "main"})
	if err != nil || len(page.Hits) != 1 {
		t.Fatalf("%v %v", page, err)
	}
	if !strings.Contains(page.Hits[0].Snippet, MarkStart+"main"+MarkEnd) {
		t.Errorf("snippet = %q", page.Hits[0].Snippet)
	}
}

func TestGetAndLists(t *testing.T) {
	ix := testIndex(t)
	ctx := context.Background()

	it, err := ix.Get(ctx, "a")
	if err != nil || it.Body != "Нацеливай PR на ветку main" {
		t.Fatalf("Get: %+v %v", it, err)
	}
	if _, err := ix.Get(ctx, "nope"); !errors.Is(err, ErrNotFound) {
		t.Errorf("ожидалась ErrNotFound, получено %v", err)
	}
	many, _ := ix.GetMany(ctx, []string{"c", "nope", "a"})
	if len(many) != 2 || many[0].ID != "c" || many[1].ID != "a" {
		t.Errorf("GetMany: %+v", many)
	}
	projects, _ := ix.Projects(ctx)
	sources, _ := ix.Sources(ctx)
	if len(projects) != 2 || projects[0].ID != "p1" || len(sources) != 2 {
		t.Errorf("projects %+v sources %+v", projects, sources)
	}
}

func TestReplaceIsFullAndReopenKeepsData(t *testing.T) {
	path := filepath.Join(t.TempDir(), "index.db")
	ix, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	_ = ix.Replace(ctx, nil, nil, []model.Item{{ID: "old", Title: "старое"}})
	_ = ix.Replace(ctx, nil, nil, []model.Item{{ID: "new", Title: "новое"}})
	ix.Close()

	ix, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer ix.Close()
	page, _ := ix.Search(ctx, Query{IncludeTemp: true})
	if !slices.Equal(ids(page), []string{"new"}) {
		t.Errorf("после повторной пересборки и открытия: %v", ids(page))
	}
	if p, _ := ix.Search(ctx, Query{Text: "старое"}); p.Total != 0 {
		t.Error("FTS хранит записи прошлой пересборки")
	}
}
