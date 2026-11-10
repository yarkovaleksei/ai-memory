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

func (e editEnv) notes(t *testing.T) []string {
	t.Helper()
	dir := filepath.Join(e.home, ".codex", "memories_v2", "extensions", "ad_hoc", "notes")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, en := range entries {
		out = append(out, filepath.Join(dir, en.Name()))
	}
	return out
}

func TestTransferClaudeToCodex(t *testing.T) {
	e := newEditEnv(t)
	ctx := context.Background()
	fact := e.find(t, model.KindClaudeFact, "Пакеты только через yarn")
	before := readFile(t, fact.File)
	indexPath := filepath.Join(filepath.Dir(fact.File), claude.IndexFile)
	indexBefore := readFile(t, indexPath)

	res := e.apply(t)(e.plans.TransferToCodex(ctx, fact.ID, "Пакеты только через yarn", "- В проекте demo пакеты ставятся через yarn 4.", true))
	if _, err := os.Stat(fact.File); !os.IsNotExist(err) {
		t.Error("при переносе факт Claude должен быть удалён")
	}
	notes := e.notes(t)
	if len(notes) != 2 {
		t.Fatalf("заметок: %d", len(notes))
	}

	// Пара «источник — копия» видна с обеих сторон, пока источник есть в индексе.
	note := e.find(t, model.KindCodexAdHocNote, "Пакеты только через yarn")
	links, err := e.history.Links(ctx, note.ID)
	if err != nil || len(links) != 1 || links[0].Direction != "from" || !links[0].Move || links[0].Title != "Пакеты только через yarn" {
		t.Fatalf("связь у копии: %+v %v", links, err)
	}

	// Перенос откатывается одной операцией: факт возвращается байт в байт,
	// заметка переписывается на отмену, связь пропадает.
	e.apply(t)(e.history.Revert(ctx, res.Commit, cancelMsg))
	if readFile(t, fact.File) != before || readFile(t, indexPath) != indexBefore {
		t.Error("откат не вернул факт и индекс")
	}
	if got := readFile(t, note.File); !strings.Contains(got, cancelMsg) {
		t.Errorf("заметка не отменена:\n%s", got)
	}
	if links, _ := e.history.Links(ctx, fact.ID); len(links) != 0 {
		t.Errorf("после отката связи быть не должно: %+v", links)
	}

	section := e.find(t, model.KindCodexSection, "User preferences")
	if _, err := e.plans.TransferToCodex(ctx, section.ID, "x", "y", false); errCode(err) != "transfer.same-agent" {
		t.Errorf("Codex в Codex: %v", err)
	}
}

func TestTransferCopyLinks(t *testing.T) {
	e := newEditEnv(t)
	ctx := context.Background()
	fact := e.find(t, model.KindClaudeFact, "Пакеты только через yarn")
	e.apply(t)(e.plans.TransferToCodex(ctx, fact.ID, "Yarn в demo", "- В проекте demo пакеты ставятся через yarn 4.", false))
	if _, err := os.Stat(fact.File); err != nil {
		t.Fatal("при копировании факт остаётся")
	}
	links, err := e.history.Links(ctx, fact.ID)
	note := e.find(t, model.KindCodexAdHocNote, "Yarn в demo")
	if err != nil || len(links) != 1 || links[0].Direction != "to" || links[0].ItemID != note.ID || links[0].Agent != model.AgentCodex {
		t.Errorf("связь у источника: %+v %v", links, err)
	}
}

func TestTransferToClaude(t *testing.T) {
	e := newEditEnv(t)
	ctx := context.Background()
	yarn := e.find(t, model.KindClaudeFact, "Пакеты только через yarn")
	legacy := e.find(t, model.KindClaudeFact, "Старый формат")
	in := claude.FactInput{Name: "short-answers", Description: "Пользователь любит короткие ответы", Type: "user", Body: "Отвечай коротко."}

	// Из Codex: факт в выбранном проекте и заметка «забыть» для источника.
	section := e.find(t, model.KindCodexSection, "User preferences")
	notesBefore := len(e.notes(t))
	p, err := e.plans.TransferToClaude(ctx, section.ID, yarn.SourceID, yarn.ProjectID, in, true,
		"Забыть: User preferences", "Пользователь перенёс это в Claude Code.")
	if err != nil || p.Op.Code != "transfer.move-to-claude" || len(p.Changes) != 3 {
		t.Fatalf("план переноса из Codex (факт, индекс, заметка): %+v %v", p, err)
	}
	e.apply(t)(p, nil)
	if len(e.notes(t)) != notesBefore+1 {
		t.Error("заметка «забыть» не создана")
	}
	created := e.find(t, model.KindClaudeFact, "short-answers")
	if links, _ := e.history.Links(ctx, created.ID); len(links) != 1 || links[0].ItemID != section.ID {
		t.Errorf("связь у факта из Codex: %+v", links)
	}

	// Между проектами Claude: факт переезжает, в том же каталоге — нет.
	in.Name = "yarn-moved"
	if _, err := e.plans.TransferToClaude(ctx, yarn.ID, yarn.SourceID, yarn.ProjectID, in, true, "", ""); errCode(err) != "transfer.same-place" {
		t.Errorf("перенос на то же место: %v", err)
	}
	e.apply(t)(e.plans.TransferToClaude(ctx, yarn.ID, legacy.SourceID, legacy.ProjectID, in, true, "", ""))
	if _, err := os.Stat(yarn.File); !os.IsNotExist(err) {
		t.Error("исходный факт остался")
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(legacy.File), "yarn-moved.md")); err != nil {
		t.Errorf("факт не появился в другом проекте: %v", err)
	}
}
