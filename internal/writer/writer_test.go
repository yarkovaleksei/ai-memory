package writer

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/yarkovaleksei/ai-memory/internal/backup"
)

type env struct {
	root string
	w    *Writer
}

func newEnv(t *testing.T, guards ...Guard) env {
	t.Helper()
	base := t.TempDir()
	root := filepath.Join(base, "memory")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	repo, err := backup.Open(context.Background(), filepath.Join(base, "data"))
	if err != nil {
		t.Fatal(err)
	}
	return env{root: root, w: &Writer{Roots: []string{root}, Backup: repo, Guards: guards}}
}

func (e env) file(t *testing.T, name, content string) string {
	t.Helper()
	p := filepath.Join(e.root, name)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func read(t *testing.T, p string) (string, bool) {
	t.Helper()
	data, err := os.ReadFile(p)
	if errors.Is(err, os.ErrNotExist) {
		return "", false
	}
	if err != nil {
		t.Fatal(err)
	}
	return string(data), true
}

func code(err error) string {
	var we *Error
	if errors.As(err, &we) {
		return we.Code
	}
	if err != nil {
		return err.Error()
	}
	return ""
}

func TestApplyAndRevert(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	upd := e.file(t, "a.md", "старое\r\nсодержимое\n") // CRLF должен вернуться байт в байт
	del := e.file(t, "b.md", "удаляемое\n")
	created := filepath.Join(e.root, "sub", "new.md")

	plan := NewPlan(Op{Code: "test.op"},
		Change{Path: upd, BeforeExists: true, Before: "старое\r\nсодержимое\n", AfterExists: true, After: "новое\n"},
		Change{Path: del, BeforeExists: true, Before: "удаляемое\n"},
		Change{Path: created, AfterExists: true, After: "создано\n"},
	)
	res, err := e.w.Apply(ctx, plan)
	if err != nil {
		t.Fatal(err)
	}
	if s, _ := read(t, upd); s != "новое\n" {
		t.Errorf("upd = %q", s)
	}
	if _, ok := read(t, del); ok {
		t.Error("b.md не удалён")
	}
	if s, _ := read(t, created); s != "создано\n" {
		t.Errorf("created = %q", s)
	}
	if matches, _ := filepath.Glob(filepath.Join(e.root, ".ai-memory-*.tmp")); len(matches) > 0 {
		t.Errorf("остались временные файлы: %v", matches)
	}

	hist, err := e.w.History(ctx, 10)
	if err != nil || len(hist) != 1 || hist[0].Commit != res.Commit || hist[0].Op.Code != "test.op" || len(hist[0].Paths) != 3 {
		t.Fatalf("History: %+v %v", hist, err)
	}

	rev, err := e.w.PlanRevert(ctx, res.Commit)
	if err != nil {
		t.Fatal(err)
	}
	if rev.Op.Code != "revert" || rev.Op.Params["op"] != "test.op" {
		t.Errorf("rev.Op = %+v", rev.Op)
	}
	if _, err := e.w.Apply(ctx, rev); err != nil {
		t.Fatal(err)
	}
	if s, _ := read(t, upd); s != "старое\r\nсодержимое\n" {
		t.Errorf("после отката upd = %q", s)
	}
	if s, _ := read(t, del); s != "удаляемое\n" {
		t.Errorf("после отката del = %q", s)
	}
	if _, ok := read(t, created); ok {
		t.Error("после отката созданный файл остался")
	}
	if hist, _ := e.w.History(ctx, 10); len(hist) != 2 || hist[0].Op.Code != "revert" {
		t.Errorf("откат должен попасть в историю: %+v", hist)
	}
}

func TestApplyRefuses(t *testing.T) {
	ctx := context.Background()
	outside := t.TempDir()
	outsideFile := filepath.Join(outside, "x.md")
	if err := os.WriteFile(outsideFile, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name  string
		setup func(e env) Plan
		want  string
	}{
		{"файл изменился с момента плана", func(e env) Plan {
			p := e.file(t, "a.md", "на диске")
			return NewPlan(Op{Code: "t"}, Change{Path: p, BeforeExists: true, Before: "видел пользователь", AfterExists: true, After: "новое"})
		}, "write.conflict"},
		{"файл появился с момента плана", func(e env) Plan {
			p := e.file(t, "a.md", "кто-то создал")
			return NewPlan(Op{Code: "t"}, Change{Path: p, AfterExists: true, After: "новое"})
		}, "write.conflict"},
		{"файл пропал с момента плана", func(e env) Plan {
			return NewPlan(Op{Code: "t"}, Change{Path: filepath.Join(e.root, "gone.md"), BeforeExists: true, Before: "x", AfterExists: true, After: "y"})
		}, "write.conflict"},
		{"путь вне корней", func(e env) Plan {
			return NewPlan(Op{Code: "t"}, Change{Path: outsideFile, BeforeExists: true, Before: "x", AfterExists: true, After: "y"})
		}, "write.outside-roots"},
		{"выход через ..", func(e env) Plan {
			return NewPlan(Op{Code: "t"}, Change{Path: e.root + "/../escape.md", AfterExists: true, After: "y"})
		}, "write.path-invalid"},
		{"относительный путь", func(e env) Plan {
			return NewPlan(Op{Code: "t"}, Change{Path: "rel.md", AfterExists: true, After: "y"})
		}, "write.path-invalid"},
		{"файл-симлинк", func(e env) Plan {
			link := filepath.Join(e.root, "link.md")
			if err := os.Symlink(outsideFile, link); err != nil {
				t.Skip("симлинки недоступны")
			}
			return NewPlan(Op{Code: "t"}, Change{Path: link, BeforeExists: true, Before: "x", AfterExists: true, After: "y"})
		}, "write.symlink"},
		{"каталог-симлинк наружу", func(e env) Plan {
			link := filepath.Join(e.root, "out")
			if err := os.Symlink(outside, link); err != nil {
				t.Skip("симлинки недоступны")
			}
			return NewPlan(Op{Code: "t"}, Change{Path: filepath.Join(link, "new.md"), AfterExists: true, After: "y"})
		}, "write.outside-roots"},
		{"один файл дважды", func(e env) Plan {
			p := filepath.Join(e.root, "d.md")
			return NewPlan(Op{Code: "t"}, Change{Path: p, AfterExists: true, After: "1"}, Change{Path: p, AfterExists: true, After: "2"})
		}, "write.path-duplicate"},
		{"пустой план", func(e env) Plan { return NewPlan(Op{Code: "t"}) }, "write.empty-plan"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newEnv(t)
			p := tt.setup(e)
			before := snapshot(t, e.root)
			_, err := e.w.Apply(ctx, p)
			if got := code(err); got != tt.want {
				t.Errorf("код = %q, want %q", got, tt.want)
			}
			if after := snapshot(t, e.root); !equalMaps(before, after) {
				t.Errorf("после отказа диск изменился:\n%v\n%v", before, after)
			}
			if data, _ := os.ReadFile(outsideFile); string(data) != "x" {
				t.Error("файл вне корней изменён")
			}
		})
	}
}

// Ошибка на втором файле возвращает первый: план применяется целиком или никак.
func TestApplyRollsBackPartialWrite(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("права каталога 0555 на Windows не запрещают запись")
	}
	e := newEnv(t)
	first := e.file(t, "a.md", "старое")
	locked := filepath.Join(e.root, "locked")
	if err := os.Mkdir(locked, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })
	plan := NewPlan(Op{Code: "t"},
		Change{Path: first, BeforeExists: true, Before: "старое", AfterExists: true, After: "новое"},
		Change{Path: filepath.Join(locked, "x.md"), AfterExists: true, After: "y"},
	)
	if _, err := e.w.Apply(context.Background(), plan); code(err) != "write.failed" {
		t.Fatalf("ожидался write.failed, получено %v", err)
	}
	if s, _ := read(t, first); s != "старое" {
		t.Errorf("первый файл не возвращён: %q", s)
	}
}

type fakeGuard struct {
	err      error
	released *bool
}

func (g fakeGuard) Acquire(context.Context, Plan) (func(), error) {
	if g.err != nil {
		return nil, g.err
	}
	return func() { *g.released = true }, nil
}

func TestGuards(t *testing.T) {
	ctx := context.Background()
	released := false
	e := newEnv(t, fakeGuard{released: &released})
	p := filepath.Join(e.root, "a.md")
	if _, err := e.w.Apply(ctx, NewPlan(Op{Code: "t"}, Change{Path: p, AfterExists: true, After: "y"})); err != nil {
		t.Fatal(err)
	}
	if !released {
		t.Error("блокировка защиты не отпущена")
	}

	e = newEnv(t, fakeGuard{err: Fail("codex.consolidation-running")})
	p = filepath.Join(e.root, "b.md")
	_, err := e.w.Apply(ctx, NewPlan(Op{Code: "t"}, Change{Path: p, AfterExists: true, After: "y"}))
	if code(err) != "codex.consolidation-running" {
		t.Errorf("код = %v", err)
	}
	if _, ok := read(t, p); ok {
		t.Error("защита отказала, а файл записан")
	}
}

func snapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	_ = filepath.Walk(root, func(p string, fi os.FileInfo, err error) error {
		if err == nil && fi.Mode().IsRegular() {
			data, _ := os.ReadFile(p)
			out[p] = string(data)
		}
		return nil
	})
	return out
}

func equalMaps(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}
