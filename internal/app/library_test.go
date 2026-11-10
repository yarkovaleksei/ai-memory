package app

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"

	"github.com/yarkovaleksei/ai-memory/internal/config"
	"github.com/yarkovaleksei/ai-memory/internal/library"
)

type recorder struct {
	mu     sync.Mutex
	events []string
}

func (r *recorder) emit(name string, _ any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, name)
}

func fixtureService(t *testing.T) (*LibraryService, *recorder) {
	t.Helper()
	home, err := filepath.Abs(filepath.Join("..", "..", "fixtures", "home"))
	if err != nil {
		t.Fatal(err)
	}
	lib, err := library.New(config.Config{
		DataDir:        filepath.ToSlash(t.TempDir()),
		ClaudeProfiles: []string{filepath.ToSlash(filepath.Join(home, ".claude"))},
		CodexHome:      filepath.ToSlash(filepath.Join(home, ".codex")),
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	rec := &recorder{}
	s := NewLibraryService(lib, nil)
	s.emit = rec.emit
	if err := s.start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := s.ServiceShutdown(); err != nil {
			t.Error(err)
		}
	})
	return s, rec
}

func TestLibraryService(t *testing.T) {
	s, rec := fixtureService(t)
	ctx := context.Background()

	projects, err := s.Projects(ctx)
	if err != nil || len(projects) == 0 || projects[0].ID != "global" {
		t.Fatalf("Projects: %+v %v", projects, err)
	}
	sources, err := s.Sources(ctx)
	if err != nil || len(sources) != 2 {
		t.Fatalf("Sources: %+v %v", sources, err)
	}

	page, err := s.ListItems(ctx, ItemQuery{Limit: 2})
	if err != nil || len(page.Items) != 2 || page.Total < 2 {
		t.Fatalf("ListItems: %+v %v", page, err)
	}
	if page.Items[0].Body != "" {
		t.Error("в списке не должно быть тела")
	}

	empty, err := s.ListItems(ctx, ItemQuery{Text: "нетакогослова"})
	if err != nil || empty.Items == nil || empty.Total != 0 {
		t.Errorf("пустая страница должна быть [], а не null: %+v %v", empty, err)
	}

	it, err := s.GetItem(ctx, page.Items[0].ID)
	if err != nil || it.ID != page.Items[0].ID {
		t.Fatalf("GetItem: %+v %v", it, err)
	}
	_, err = s.GetItem(ctx, "nope")
	var appErr *Error
	if !errors.As(err, &appErr) || appErr.Code != "item.not-found" {
		t.Errorf("ожидался код item.not-found, получено %v", err)
	}

	many, err := s.GetItems(ctx, []string{"nope"})
	if err != nil || many == nil || len(many) != 0 {
		t.Errorf("GetItems: %+v %v", many, err)
	}

	if _, err := s.Rebuild(ctx); err != nil {
		t.Fatal(err)
	}
	rec.mu.Lock()
	defer rec.mu.Unlock()
	if len(rec.events) != 1 || rec.events[0] != EventIndexUpdated {
		t.Errorf("события: %v", rec.events)
	}
}
