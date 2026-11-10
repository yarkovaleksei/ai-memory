package app

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yarkovaleksei/ai-memory/internal/adapter/claude"
	"github.com/yarkovaleksei/ai-memory/internal/config"
	"github.com/yarkovaleksei/ai-memory/internal/index"
	"github.com/yarkovaleksei/ai-memory/internal/library"
	"github.com/yarkovaleksei/ai-memory/internal/model"
	"github.com/yarkovaleksei/ai-memory/internal/writer"
)

// editEnv — сервисы записи над копией фикстур во временном каталоге: тесты
// никогда не пишут в fixtures/ и тем более в настоящую память.
type editEnv struct {
	home    string
	lib     *library.Library
	plans   *PlansService
	history *HistoryService
	rec     *recorder
}

func newEditEnv(t *testing.T) editEnv {
	t.Helper()
	return newEditEnvWith(t, nil)
}

// newEditEnvWith — то же, но prepare может изменить копию фикстур до первой
// пересборки индекса (симлинки, git-репозиторий проекта).
func newEditEnvWith(t *testing.T, prepare func(home string)) editEnv {
	t.Helper()
	src, err := filepath.Abs(filepath.Join("..", "..", "fixtures", "home"))
	if err != nil {
		t.Fatal(err)
	}
	home := filepath.Join(t.TempDir(), "home")
	copyDir(t, src, home)
	if prepare != nil {
		prepare(home)
	}
	cfg := config.Config{
		DataDir:        filepath.ToSlash(filepath.Join(t.TempDir(), "data")),
		ClaudeProfiles: []string{filepath.ToSlash(filepath.Join(home, ".claude"))},
		CodexHome:      filepath.ToSlash(filepath.Join(home, ".codex")),
		AgentsDir:      filepath.ToSlash(filepath.Join(home, ".agents")),
	}
	lib, err := library.New(cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = lib.Close() })
	if _, err := lib.Rebuild(context.Background()); err != nil {
		t.Fatal(err)
	}
	plans, history := NewEditing(context.Background(), lib, NewRuntime(cfg), nil)
	rec := &recorder{}
	plans.e.emit = rec.emit
	if !plans.Status(context.Background()).Enabled {
		t.Fatalf("запись выключена: %s", plans.e.disabled)
	}
	return editEnv{home: home, lib: lib, plans: plans, history: history, rec: rec}
}

func copyDir(t *testing.T, src, dst string) {
	t.Helper()
	err := filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		in, err := os.Open(p)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.Create(target)
		if err != nil {
			return err
		}
		if _, err := io.Copy(out, in); err != nil {
			out.Close()
			return err
		}
		return out.Close()
	})
	if err != nil {
		t.Fatal(err)
	}
}

// find ищет запись по виду и заголовку.
func (e editEnv) find(t *testing.T, kind model.ItemKind, title string) model.Item {
	t.Helper()
	page, err := e.lib.Search(context.Background(), index.Query{Kind: string(kind), IncludeTemp: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, h := range page.Hits {
		if h.Title == title {
			it, err := e.lib.Get(context.Background(), h.ID)
			if err != nil {
				t.Fatal(err)
			}
			return it
		}
	}
	t.Fatalf("нет записи %s %q", kind, title)
	return model.Item{}
}

// searchKind — все записи вида, включая временные проекты.
func (e editEnv) searchKind(t *testing.T, kind model.ItemKind) []index.Hit {
	t.Helper()
	page, err := e.lib.Search(context.Background(), index.Query{Kind: string(kind), IncludeTemp: true})
	if err != nil {
		t.Fatal(err)
	}
	return page.Hits
}

// findIn ищет запись по виду и заголовку в файле под каталогом с именем dir.
func (e editEnv) findIn(t *testing.T, kind model.ItemKind, title, dir string) model.Item {
	t.Helper()
	for _, h := range e.searchKind(t, kind) {
		if h.Title == title && filepath.Base(filepath.Dir(h.File)) == dir {
			return h.Item
		}
	}
	t.Fatalf("нет записи %s %q в %s", kind, title, dir)
	return model.Item{}
}

// apply(t)(PlanXxx(...)): Go не даёт передать кортеж вместе с другими аргументами.
func (e editEnv) apply(t *testing.T) func(writer.Plan, error) ApplyResult {
	return func(p writer.Plan, err error) ApplyResult {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		res, err := e.plans.Apply(context.Background(), p.ID)
		if err != nil {
			t.Fatal(err)
		}
		return res
	}
}

const cancelMsg = "Пользователь отменил эту заметку: её прежнее содержимое не должно оставаться в памяти."

func errCode(err error) string {
	var ae *Error
	if errors.As(err, &ae) {
		return ae.Code
	}
	if err != nil {
		return "не *Error: " + err.Error()
	}
	return ""
}

func readFile(t *testing.T, p string) string {
	t.Helper()
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestEditFactAndRevert(t *testing.T) {
	e := newEditEnv(t)
	ctx := context.Background()
	it := e.find(t, model.KindClaudeFact, "Пакеты только через yarn")
	index := filepath.Join(filepath.Dir(it.File), claude.IndexFile)
	before, indexBefore := readFile(t, it.File), readFile(t, index)

	d, err := e.plans.Draft(ctx, it.ID)
	if err != nil || d.Fact == nil || d.Content != before {
		t.Fatalf("Draft: %+v %v", d, err)
	}
	if d.Fact.Name != "prefer-yarn" || d.Fact.Type != "feedback" || d.Fact.Title != "Пакеты только через yarn" {
		t.Errorf("поля формы: %+v", d.Fact)
	}

	in := *d.Fact
	in.Body = "Только yarn 4."
	p, err := e.plans.UpdateFact(ctx, it.ID, d.Hash, in)
	if err != nil || p.Op.Code != "claude.fact.update" || len(p.Changes) == 0 {
		t.Fatalf("UpdateFact: %+v %v", p, err)
	}
	if readFile(t, it.File) != before {
		t.Fatal("построение плана записало файл")
	}
	res := e.apply(t)(p, nil)
	if !strings.Contains(readFile(t, it.File), "Только yarn 4.") || res.Commit == "" {
		t.Errorf("правка не записана: %+v", res)
	}
	if len(e.rec.events) == 0 || e.rec.events[0] != EventIndexUpdated {
		t.Errorf("после записи нет события индекса: %v", e.rec.events)
	}
	if _, err := e.plans.Apply(ctx, p.ID); errCode(err) != "plan.not-found" {
		t.Errorf("план одноразовый, повтор: %v", err)
	}

	hist, err := e.history.List(ctx)
	if err != nil || len(hist) != 1 || hist[0].Commit != res.Commit {
		t.Fatalf("История: %+v %v", hist, err)
	}
	e.apply(t)(e.history.Revert(ctx, res.Commit, cancelMsg))
	if readFile(t, it.File) != before || readFile(t, index) != indexBefore {
		t.Error("откат не вернул файлы байт в байт")
	}
	if hist, _ := e.history.List(ctx); len(hist) != 2 || hist[0].Op.Code != "revert" {
		t.Errorf("откат должен попасть в историю: %+v", hist)
	}
}

// Пока пользователь правил, файл поменял агент: план не строится и не затирает
// чужую правку, даже если индекс уже успел перечитать файл.
func TestEditConflict(t *testing.T) {
	e := newEditEnv(t)
	ctx := context.Background()
	it := e.find(t, model.KindClaudeFact, "Пакеты только через yarn")
	d, err := e.plans.Draft(ctx, it.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(it.File, []byte(d.Content+"\nправка агента\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := e.lib.Rebuild(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := e.plans.UpdateFact(ctx, it.ID, d.Hash, *d.Fact); errCode(err) != "write.conflict" {
		t.Errorf("UpdateFact: %v", err)
	}
	if _, err := e.plans.UpdateSource(ctx, it.ID, d.Hash, "x"); errCode(err) != "write.conflict" {
		t.Errorf("UpdateSource: %v", err)
	}
}

func TestCreateAndDeleteFacts(t *testing.T) {
	e := newEditEnv(t)
	ctx := context.Background()
	yarn := e.find(t, model.KindClaudeFact, "Пакеты только через yarn")
	memDir := filepath.Dir(yarn.File)
	in := claude.FactInput{Name: "ci-cache", Description: "Кэш CI сбрасывается вручную", Type: "project", Body: "Текст."}

	e.apply(t)(e.plans.CreateFact(ctx, yarn.SourceID, yarn.ProjectID, in))
	if _, err := os.Stat(filepath.Join(memDir, "ci-cache.md")); err != nil {
		t.Fatalf("факт не создан: %v", err)
	}
	if _, err := e.plans.CreateFact(ctx, yarn.SourceID, "global", in); errCode(err) != "claude.fact.project-required" {
		t.Errorf("факт без проекта: %v", err)
	}
	if _, err := e.plans.CreateFact(ctx, "claude:/nowhere", yarn.ProjectID, in); errCode(err) != "plan.source-invalid" {
		t.Errorf("чужой источник: %v", err)
	}

	created := e.find(t, model.KindClaudeFact, "ci-cache")
	p, err := e.plans.DeleteFacts(ctx, []string{yarn.ID, created.ID})
	if err != nil || len(p.Changes) != 3 || p.Op.Params["count"] != "2" {
		t.Fatalf("массовое удаление одним планом (два файла и индекс): %+v %v", p, err)
	}
	e.apply(t)(p, nil)
	if strings.Contains(readFile(t, filepath.Join(memDir, claude.IndexFile)), "prefer-yarn.md") {
		t.Error("строка индекса осталась")
	}

	note := e.find(t, model.KindCodexAdHocNote, "2026-01-03 — пакеты фронтенда")
	if _, err := e.plans.DeleteFacts(ctx, []string{note.ID}); errCode(err) != "plan.delete-unsupported" {
		t.Errorf("заметку Codex удалять нельзя: %v", err)
	}
}

func TestCodexNotes(t *testing.T) {
	e := newEditEnv(t)
	ctx := context.Background()
	notesDir := filepath.Join(e.home, ".codex", "memories_v2", "extensions", "ad_hoc", "notes")

	res := e.apply(t)(e.plans.CreateNote(ctx, "Сборка демо", "- Собирается командой make demo."))
	entries, _ := os.ReadDir(notesDir)
	if len(entries) != 2 {
		t.Fatalf("в каталоге заметок %d файлов, ожидалось 2", len(entries))
	}
	// Откат создания заметки не удаляет файл, а переписывает его на отмену (ADR-014).
	notePath := filepath.Join(notesDir, entries[1].Name())
	if entries[0].Name() > entries[1].Name() {
		notePath = filepath.Join(notesDir, entries[0].Name())
	}
	if _, err := e.history.Revert(ctx, res.Commit, ""); errCode(err) != "codex.note.body-empty" {
		t.Errorf("отмена без текста: %v", err)
	}
	cancel := e.apply(t)(e.history.Revert(ctx, res.Commit, cancelMsg))
	if got := readFile(t, notePath); got != "# Сборка демо\n\n"+cancelMsg+"\n" {
		t.Errorf("заметка после отмены:\n%s", got)
	}
	e.apply(t)(e.history.Revert(ctx, cancel.Commit, cancelMsg))
	if got := readFile(t, notePath); got != "# Сборка демо\n\n- Собирается командой make demo.\n" {
		t.Errorf("откат отмены должен вернуть заметку:\n%s", got)
	}

	e.apply(t)(e.plans.ForgetNote(ctx, "Забыть: make demo", "Исключи из памяти, что демо собирается через make demo."))
	if entries, _ := os.ReadDir(notesDir); len(entries) != 3 {
		t.Errorf("заметка «забыть» не создана: %d файлов", len(entries))
	}

	note := e.find(t, model.KindCodexAdHocNote, "2026-01-03 — пакеты фронтенда")
	d, err := e.plans.Draft(ctx, note.ID)
	if err != nil || d.Fact != nil || d.Content == "" {
		t.Fatalf("Draft заметки: %+v %v", d, err)
	}
	e.apply(t)(e.plans.UpdateSource(ctx, note.ID, d.Hash, d.Content+"- Ещё факт.\n"))
	if _, err := e.plans.UpdateSource(ctx, note.ID, "", ""); errCode(err) != "codex.note.body-empty" {
		t.Errorf("пустая заметка равна удалению: %v", err)
	}

	section := e.find(t, model.KindCodexSection, "User preferences")
	if _, err := e.plans.Draft(ctx, section.ID); errCode(err) != "plan.readonly" {
		t.Errorf("сгенерированный раздел Codex: %v", err)
	}
	if _, err := e.plans.UpdateSource(ctx, section.ID, "", "x"); errCode(err) != "plan.readonly" {
		t.Errorf("сгенерированный раздел Codex: %v", err)
	}
}

func TestApplyRefusals(t *testing.T) {
	e := newEditEnv(t)
	ctx := context.Background()
	if _, err := e.plans.Apply(ctx, "нет-такого"); errCode(err) != "plan.not-found" {
		t.Errorf("неизвестный план: %v", err)
	}
	if _, err := e.plans.Draft(ctx, "нет-такого"); errCode(err) != "item.not-found" {
		t.Errorf("неизвестная запись: %v", err)
	}

	p, err := e.plans.CreateNote(ctx, "Заметка", "Текст.")
	if err != nil {
		t.Fatal(err)
	}
	e.plans.Discard(p.ID)
	if _, err := e.plans.Apply(ctx, p.ID); errCode(err) != "plan.not-found" {
		t.Errorf("отменённый план применился: %v", err)
	}

	// Без репозитория бэкапов не пишем вовсе.
	p, err = e.plans.CreateNote(ctx, "Заметка", "Текст.")
	if err != nil {
		t.Fatal(err)
	}
	e.plans.e.disabled = "backup.git-missing"
	if _, err := e.plans.Apply(ctx, p.ID); errCode(err) != "backup.git-missing" {
		t.Errorf("запись без бэкапа: %v", err)
	}
	if st := e.plans.Status(ctx); st.Enabled || st.Reason != "backup.git-missing" || st.Codex.Generation != "v2" {
		t.Errorf("Status: %+v", st)
	}
}

// Планы, брошенные в диалоге, не копятся.
func TestPlanStoreIsBounded(t *testing.T) {
	e := newEditEnv(t)
	var first writer.Plan
	for i := 0; i < maxPlans+5; i++ {
		p, err := e.plans.CreateNote(context.Background(), "Заметка", "Текст.")
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			first = p
		}
	}
	if len(e.plans.e.plans) != maxPlans {
		t.Errorf("планов в памяти: %d", len(e.plans.e.plans))
	}
	if _, err := e.plans.Apply(context.Background(), first.ID); errCode(err) != "plan.not-found" {
		t.Errorf("самый старый план должен быть вытеснен: %v", err)
	}
}
