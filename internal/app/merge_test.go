package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yarkovaleksei/ai-memory/internal/adapter/claude"
	"github.com/yarkovaleksei/ai-memory/internal/model"
)

func TestMergeIntoClaude(t *testing.T) {
	e := newEditEnv(t)
	ctx := context.Background()
	yarn := e.find(t, model.KindClaudeFact, "Пакеты только через yarn")
	release := e.find(t, model.KindClaudeFact, "Как выпускается релиз")
	note := e.find(t, model.KindCodexAdHocNote, "2026-01-03 — пакеты фронтенда")
	indexPath := filepath.Join(filepath.Dir(yarn.File), claude.IndexFile)
	before := map[string]string{yarn.File: readFile(t, yarn.File), release.File: readFile(t, release.File), indexPath: readFile(t, indexPath)}
	notesBefore := len(e.notes(t))

	// Итог берёт имя одного из исходных фактов: этот файл переписывается, второй
	// удаляется, индекс меняется одним изменением.
	req := MergeRequest{
		ItemIDs:   []string{yarn.ID, release.ID, note.ID},
		Target:    model.AgentClaude,
		SourceID:  yarn.SourceID,
		ProjectID: yarn.ProjectID,
		Fact:      claude.FactInput{Name: "prefer-yarn", Description: "Только yarn 4", Type: "feedback", Body: "Объединено."},
		Forget:    []NoteText{{ItemID: note.ID, Title: "Забыть: пакеты фронтенда", Body: "Это теперь в Claude Code."}},
	}
	p, err := e.plans.Merge(ctx, req)
	if err != nil || p.Op.Code != "merge" || len(p.Changes) != 4 {
		t.Fatalf("план (факт, удалённый факт, индекс, «забыть»): %+v %v", p, err)
	}
	res := e.apply(t)(p, nil)
	if !strings.Contains(readFile(t, yarn.File), "Объединено.") {
		t.Error("итог не записан")
	}
	if _, err := os.Stat(release.File); !os.IsNotExist(err) {
		t.Error("второй факт не удалён")
	}
	if idx := readFile(t, indexPath); strings.Count(idx, "prefer-yarn.md") != 1 || strings.Contains(idx, "release-flow.md") {
		t.Errorf("индекс после объединения:\n%s", idx)
	}
	if len(e.notes(t)) != notesBefore+1 {
		t.Error("заметка «забыть» не создана")
	}

	// Одна операция — один откат.
	e.apply(t)(e.history.Revert(ctx, res.Commit, cancelMsg))
	for path, want := range before {
		if readFile(t, path) != want {
			t.Errorf("после отката %s не совпадает", filepath.Base(path))
		}
	}
}

func TestMergeIntoCodex(t *testing.T) {
	e := newEditEnv(t)
	ctx := context.Background()
	yarn := e.find(t, model.KindClaudeFact, "Пакеты только через yarn")
	section := e.find(t, model.KindCodexSection, "User preferences")
	notesBefore := len(e.notes(t))
	e.apply(t)(e.plans.Merge(ctx, MergeRequest{
		ItemIDs: []string{yarn.ID, section.ID},
		Target:  model.AgentCodex,
		Title:   "Пакеты", Body: "- Только yarn 4.",
		Forget: []NoteText{{ItemID: section.ID, Title: "Пакеты", Body: "Забыть прежнюю формулировку."}},
	}))
	// Итог и «забыть» с одинаковым заголовком не делят один файл.
	if got := len(e.notes(t)); got != notesBefore+2 {
		t.Errorf("заметок прибавилось %d, ожидалось 2", got-notesBefore)
	}
	if _, err := os.Stat(yarn.File); !os.IsNotExist(err) {
		t.Error("факт Claude не удалён")
	}
}

func TestMergeRefuses(t *testing.T) {
	e := newEditEnv(t)
	ctx := context.Background()
	yarn := e.find(t, model.KindClaudeFact, "Пакеты только через yarn")
	section := e.find(t, model.KindCodexSection, "User preferences")
	tests := []struct {
		name string
		req  MergeRequest
		want string
	}{
		{"одна запись", MergeRequest{ItemIDs: []string{yarn.ID}, Target: model.AgentClaude}, "merge.too-few"},
		{"нет «забыть» для Codex", MergeRequest{ItemIDs: []string{yarn.ID, section.ID}, Target: model.AgentCodex, Title: "t", Body: "b"}, "codex.note.body-empty"},
		{"неизвестная цель", MergeRequest{ItemIDs: []string{yarn.ID, section.ID}, Target: "x",
			Forget: []NoteText{{ItemID: section.ID, Title: "t", Body: "b"}}}, "merge.target-invalid"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := e.plans.Merge(ctx, tt.req); errCode(err) != tt.want {
				t.Errorf("код = %v, want %s", err, tt.want)
			}
		})
	}
}
