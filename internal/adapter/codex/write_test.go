package codex

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"
	"time"

	"github.com/yarkovaleksei/ai-memory/internal/backup"
	"github.com/yarkovaleksei/ai-memory/internal/model"
	"github.com/yarkovaleksei/ai-memory/internal/writer"
)

// copyHome копирует фикстуру .codex во временный каталог: тесты пишут только туда.
func copyHome(t *testing.T) string {
	t.Helper()
	src, err := filepath.Abs(filepath.Join("..", "..", "..", "fixtures", "home", ".codex"))
	if err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(t.TempDir(), ".codex")
	err = filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
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
	return dst
}

func newWriter(t *testing.T, home string, g Generation) *writer.Writer {
	t.Helper()
	repo, err := backup.Open(context.Background(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return &writer.Writer{Roots: []string{home}, Backup: repo, Guards: []writer.Guard{Guard{Home: home, Generation: g}}}
}

func failCode(err error) string {
	var we *writer.Error
	if errors.As(err, &we) {
		return we.Code
	}
	if err != nil {
		return err.Error()
	}
	return ""
}

func TestSlug(t *testing.T) {
	tests := []struct{ in, want string }{
		{"Пакеты фронтенда — только yarn", "pakety-frontenda-tolko-yarn"},
		{"Minimal scope builds", "minimal-scope-builds"},
		{"  Ёлка и щука!  ", "elka-i-schuka"},
		{"!!!", "note"},
	}
	for _, tt := range tests {
		if got := Slug(tt.in); got != tt.want {
			t.Errorf("Slug(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestNotesRoundTrip(t *testing.T) {
	now = func() time.Time { return time.Date(2026, 2, 3, 12, 0, 0, 0, time.UTC) }
	t.Cleanup(func() { now = time.Now })
	ctx := context.Background()
	home := copyHome(t)
	w := newWriter(t, home, GenV2)

	p, err := PlanNewNote(home, GenV2, "Пакеты только через yarn", "- npm не использовать.")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Apply(ctx, p); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(NotesDir(home, GenV2), "2026-02-03-pakety-tolko-cherez-yarn.md")
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "# Пакеты только через yarn\n\n- npm не использовать.\n" {
		t.Fatalf("заметка: %q %v", data, err)
	}

	// Та же тема в тот же день — новый файл, а не перезапись.
	p2, _ := PlanForget(home, GenV2, "Пакеты только через yarn", "Забыть: факт устарел.")
	if p2.Changes[0].Path != filepath.Join(NotesDir(home, GenV2), "2026-02-03-forget-pakety-tolko-cherez-yarn.md") || p2.Op.Code != "codex.note.forget" {
		t.Errorf("план «забыть»: %+v", p2)
	}
	p3, _ := PlanNewNote(home, GenV2, "Пакеты только через yarn", "ещё")
	if filepath.Base(p3.Changes[0].Path) != "2026-02-03-pakety-tolko-cherez-yarn-2.md" {
		t.Errorf("повтор имени: %s", p3.Changes[0].Path)
	}

	// Правка заметки и её откат.
	upd, err := PlanUpdateNote(path, model.ContentHash(data), "# Пакеты только через yarn\n\n- И никакого pnpm.\n")
	if err != nil {
		t.Fatal(err)
	}
	res, err := w.Apply(ctx, upd)
	if err != nil {
		t.Fatal(err)
	}
	rev, err := w.PlanRevert(ctx, res.Commit)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Apply(ctx, rev); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(path); string(got) != string(data) {
		t.Errorf("откат правки: %q", got)
	}

	// Откат создания заметки удалил бы файл — это запрещено (ADR-002).
	hist, _ := w.History(ctx, 10)
	create := hist[len(hist)-1]
	rev, err = w.PlanRevert(ctx, create.Commit)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Apply(ctx, rev); failCode(err) != "codex.note.delete-forbidden" {
		t.Errorf("откат создания заметки: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Error("заметка удалена")
	}
}

// TestGuardProtectsGeneratedFilesAndNotes — критерий фазы 2: writer никогда не
// переписывает сгенерированные файлы Codex и не удаляет заметки.
func TestGuardProtectsGeneratedFilesAndNotes(t *testing.T) {
	ctx := context.Background()
	home := copyHome(t)
	v2 := filepath.Join(home, dirV2)
	note := filepath.Join(NotesDir(home, GenV2), "2026-01-03-prefer-yarn.md")
	read := func(p string) string { d, _ := os.ReadFile(p); return string(d) }

	tests := []struct {
		name   string
		change writer.Change
		want   string
	}{
		{"сводка", writer.Change{Path: filepath.Join(v2, summaryFile), BeforeExists: true, Before: read(filepath.Join(v2, summaryFile)), AfterExists: true, After: "x"}, "codex.generated-readonly"},
		{"rollout-сводка", writer.Change{Path: filepath.Join(v2, rolloutsDir, "2026-01-02T10-00-00-Ab12-demo_release_flow.md"), BeforeExists: true, Before: read(filepath.Join(v2, rolloutsDir, "2026-01-02T10-00-00-Ab12-demo_release_flow.md"))}, "codex.generated-readonly"},
		{"инструкция ad_hoc", writer.Change{Path: filepath.Join(v2, "extensions", "ad_hoc", "instructions.md"), BeforeExists: true, Before: read(filepath.Join(v2, "extensions", "ad_hoc", "instructions.md")), AfterExists: true, After: "x"}, "codex.generated-readonly"},
		{"новый файл в корне поколения", writer.Change{Path: filepath.Join(v2, "extra.md"), AfterExists: true, After: "x"}, "codex.generated-readonly"},
		{"не .md в заметках", writer.Change{Path: filepath.Join(NotesDir(home, GenV2), "x.txt"), AfterExists: true, After: "x"}, "codex.generated-readonly"},
		{"заметка неактивного поколения", writer.Change{Path: filepath.Join(NotesDir(home, GenV1), "new.md"), AfterExists: true, After: "x"}, "codex.generated-readonly"},
		{"удаление заметки", writer.Change{Path: note, BeforeExists: true, Before: read(note)}, "codex.note.delete-forbidden"},
		{"config.toml Codex", writer.Change{Path: filepath.Join(home, "config.toml"), BeforeExists: true, Before: read(filepath.Join(home, "config.toml")), AfterExists: true, After: "x"}, "codex.generated-readonly"},
		// Инструкции и навыки — не память (ADR-020), но системные навыки Codex
		// перезаписывает сам, а удалять файлы ai-memory не умеет.
		{"системный навык", writer.Change{Path: filepath.Join(home, "skills", ".system", "plan-helper", "SKILL.md"), BeforeExists: true, Before: read(filepath.Join(home, "skills", ".system", "plan-helper", "SKILL.md")), AfterExists: true, After: "x"}, "codex.generated-readonly"},
		{"файл в корне skills", writer.Change{Path: filepath.Join(home, "skills", "x.md"), AfterExists: true, After: "x"}, "codex.generated-readonly"},
		{"удаление AGENTS.md", writer.Change{Path: filepath.Join(home, "AGENTS.md"), BeforeExists: true, Before: read(filepath.Join(home, "AGENTS.md"))}, "codex.files.delete-forbidden"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newWriter(t, home, GenV2)
			before := read(tt.change.Path)
			_, err := w.Apply(ctx, writer.NewPlan(writer.Op{Code: "t"}, tt.change))
			if failCode(err) != tt.want {
				t.Errorf("код = %v, want %s", err, tt.want)
			}
			if read(tt.change.Path) != before {
				t.Error("файл изменён")
			}
		})
	}
}

func TestGuardWaitsForConsolidation(t *testing.T) {
	ctx := context.Background()

	t.Run("lock занят (Codex 0.167)", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("проверяется flock; LockFileEx — на Windows в CI")
		}
		home := copyHome(t)
		lockPath := filepath.Join(home, GenV2.LockFile())
		f, err := os.OpenFile(lockPath, os.O_RDWR|os.O_CREATE, 0o644)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
			t.Fatal(err)
		}
		p, _ := PlanNewNote(home, GenV2, "t", "b")
		if _, err := newWriter(t, home, GenV2).Apply(ctx, p); failCode(err) != "codex.consolidation-running" {
			t.Errorf("код = %v", err)
		}
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		p, _ = PlanNewNote(home, GenV2, "t", "b")
		if _, err := newWriter(t, home, GenV2).Apply(ctx, p); err != nil {
			t.Errorf("после снятия lock: %v", err)
		}
	})

	for _, tt := range []struct {
		name   string
		status string
		lease  int64
		want   string
	}{
		{"задача идёт (Codex 0.159)", "running", time.Now().Add(time.Hour).Unix(), "codex.consolidation-running"},
		{"аренда истекла — задача упала", "running", time.Now().Add(-time.Minute).Unix(), ""},
		{"задача завершена", "done", 0, ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			home := copyHome(t)
			makeJobsDB(t, filepath.Join(home, GenV2.DB()), tt.status, tt.lease, 0)
			p, _ := PlanNewNote(home, GenV2, "t", "b")
			if _, err := newWriter(t, home, GenV2).Apply(ctx, p); failCode(err) != tt.want {
				t.Errorf("код = %q, want %q", failCode(err), tt.want)
			}
		})
	}
}

func TestDetectGeneration(t *testing.T) {
	ctx := context.Background()
	mk := func(t *testing.T, dirs ...string) string {
		home := t.TempDir()
		for _, d := range dirs {
			if err := os.MkdirAll(filepath.Join(home, d), 0o755); err != nil {
				t.Fatal(err)
			}
		}
		return home
	}
	tests := []struct {
		name   string
		setup  func(t *testing.T) string
		want   Generation
		reason string
	}{
		{"явная настройка Codex", func(t *testing.T) string {
			h := mk(t, dirV1, dirV2)
			_ = os.WriteFile(filepath.Join(h, "config.toml"), []byte("[memories]\nversion = \"v1\"\n"), 0o644)
			return h
		}, GenV1, ReasonConfig},
		{"только v2", func(t *testing.T) string { return mk(t, dirV2) }, GenV2, ReasonOnlyOne},
		{"только v1", func(t *testing.T) string { return mk(t, dirV1) }, GenV1, ReasonOnlyOne},
		{"оба, v2 работал позже", func(t *testing.T) string {
			h := mk(t, dirV1, dirV2)
			makeJobsDB(t, filepath.Join(h, GenV1.DB()), "pending", 0, 100)
			makeJobsDB(t, filepath.Join(h, GenV2.DB()), "done", 0, 200)
			return h
		}, GenV2, ReasonActivity},
		{"базы в sqlite_home", func(t *testing.T) string {
			h := mk(t, dirV1, dirV2)
			db := t.TempDir()
			_ = os.WriteFile(filepath.Join(h, "config.toml"), []byte("sqlite_home = \""+filepath.ToSlash(db)+"\"\n"), 0o644)
			makeJobsDB(t, filepath.Join(db, GenV1.DB()), "done", 0, 300)
			makeJobsDB(t, filepath.Join(db, GenV2.DB()), "done", 0, 200)
			return h
		}, GenV1, ReasonActivity},
		{"оба, баз нет — умолчание Codex", func(t *testing.T) string { return mk(t, dirV1, dirV2) }, GenV1, ReasonDefault},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := tt.setup(t)
			g, reason := DetectGeneration(ctx, home)
			if g != tt.want || reason != tt.reason {
				t.Errorf("= %s (%s), want %s (%s)", g, reason, tt.want, tt.reason)
			}
			for _, db := range []string{GenV1.DB(), GenV2.DB()} {
				if _, err := os.Stat(filepath.Join(home, db)); err == nil && tt.reason != ReasonActivity {
					t.Errorf("определение создало базу %s", db)
				}
			}
		})
	}
}

// makeJobsDB создаёт базу Codex со строкой задачи консолидации по схеме из
// storage-formats.md (в фикстурах баз нет: бинарные файлы в репозитории неудобны).
func makeJobsDB(t *testing.T, path, status string, lease, finished int64) {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	stmts := []string{
		`CREATE TABLE jobs (kind TEXT NOT NULL, job_key TEXT NOT NULL, status TEXT NOT NULL, worker_id TEXT,
			ownership_token TEXT, started_at INTEGER, finished_at INTEGER, lease_until INTEGER, retry_at INTEGER,
			retry_remaining INTEGER NOT NULL, last_error TEXT, input_watermark INTEGER, last_success_watermark INTEGER,
			PRIMARY KEY (kind, job_key))`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			t.Fatal(err)
		}
	}
	var leaseV, finV any
	if lease != 0 {
		leaseV = lease
	}
	if finished != 0 {
		finV = finished
	}
	if _, err := db.Exec(`INSERT INTO jobs (kind, job_key, status, lease_until, finished_at, retry_remaining)
		VALUES ('memory_consolidate_global', 'global', ?, ?, ?, 3)`, status, leaseV, finV); err != nil {
		t.Fatal(err)
	}
}

// AGENTS.md и пользовательские навыки в CODEX_HOME правятся, и консолидация
// им не мешает: это не память, блокировка Codex не берётся.
func TestGuardAllowsInstructionsAndSkills(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("проверяется flock; LockFileEx — на Windows в CI")
	}
	ctx := context.Background()
	home := copyHome(t)
	f, err := os.OpenFile(filepath.Join(home, GenV2.LockFile()), os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{
		filepath.Join(home, "AGENTS.md"),
		filepath.Join(home, "skills", "lint-fix", "SKILL.md"),
	} {
		before, _ := os.ReadFile(path)
		w := newWriter(t, home, GenV2)
		change := writer.Change{Path: path, BeforeExists: true, Before: string(before), AfterExists: true, After: "новое\n"}
		if _, err := w.Apply(ctx, writer.NewPlan(writer.Op{Code: "t"}, change)); err != nil {
			t.Errorf("%s: %v", path, err)
		}
	}
}
