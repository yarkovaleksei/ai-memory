package writer

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// Симлинк каталога навыка создаётся и удаляется откатом; через него writer
// не пишет, цель не трогает (ADR-023).
func TestLinkApplyAndRevert(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	target := filepath.Join(t.TempDir(), "skill") // вне корней: туда не пишем
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "SKILL.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(e.root, "skills", "shared")
	script := filepath.Join(e.root, "skills", "copy", "run.sh")

	res, err := e.w.Apply(ctx, NewPlan(Op{Code: "skill.install"},
		Change{Path: link, AfterExists: true, Link: target},
		Change{Path: script, AfterExists: true, After: "#!/bin/sh\n", Exec: true},
	))
	if err != nil {
		if os.IsPermission(err) {
			t.Skipf("симлинки недоступны: %v", err)
		}
		t.Fatal(err)
	}
	if got, err := os.Readlink(link); err != nil || got != target {
		t.Fatalf("симлинк: %q %v", got, err)
	}
	// На Windows бита исполнения нет: os.Stat всегда даёт rw-rw-rw-.
	if fi, err := os.Stat(script); runtime.GOOS != "windows" && (err != nil || fi.Mode().Perm()&0o100 == 0) {
		t.Errorf("скрипт не исполняемый: %v %v", fi.Mode(), err)
	}

	rev, err := e.w.PlanRevert(ctx, res.Commit)
	if err != nil {
		t.Fatal(err)
	}
	if rev.Changes[0].BeforeLink != target || rev.Changes[0].Before != "" {
		t.Errorf("откат не знает, что это симлинк: %+v", rev.Changes[0])
	}
	res2, err := e.w.Apply(ctx, rev)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(link); !os.IsNotExist(err) {
		t.Errorf("симлинк остался: %v", err)
	}
	if _, err := os.Stat(filepath.Join(target, "SKILL.md")); err != nil {
		t.Errorf("цель симлинка пострадала: %v", err)
	}

	// Откат отката возвращает симлинк и права скрипта.
	rev2, err := e.w.PlanRevert(ctx, res2.Commit)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.w.Apply(ctx, rev2); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.Readlink(link); got != target {
		t.Errorf("симлинк после отката отката: %q", got)
	}
	if fi, err := os.Stat(script); runtime.GOOS != "windows" && (err != nil || fi.Mode().Perm()&0o100 == 0) {
		t.Errorf("скрипт после отката отката: %v %v", fi.Mode(), err)
	}
}

func TestLinkRefuses(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	target := t.TempDir()
	existing := e.file(t, "skills/x/SKILL.md", "x")
	link := filepath.Join(e.root, "skills", "l")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("симлинки недоступны: %v", err)
	}
	tests := []struct {
		name   string
		change Change
		want   string
	}{
		{"относительная цель", Change{Path: filepath.Join(e.root, "skills", "y"), AfterExists: true, Link: "rel/dir"}, "write.path-invalid"},
		{"симлинк с содержимым", Change{Path: filepath.Join(e.root, "skills", "y"), AfterExists: true, After: "x", Link: target}, "write.path-invalid"},
		{"на месте файла", Change{Path: existing, AfterExists: true, Link: target}, "write.conflict"},
		{"на месте другого симлинка", Change{Path: link, AfterExists: true, Link: target}, "write.symlink"},
		{"удаление чужого симлинка", Change{Path: link, BeforeExists: true, BeforeLink: "/elsewhere"}, "write.conflict"},
		{"запись файла через симлинк", Change{Path: link, BeforeExists: true, BeforeLink: target, AfterExists: true, After: "x"}, "write.path-invalid"},
		{"вне корней", Change{Path: filepath.Join(t.TempDir(), "l"), AfterExists: true, Link: target}, "write.outside-roots"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := e.w.Apply(ctx, NewPlan(Op{Code: "t"}, tt.change))
			if code(err) != tt.want {
				t.Errorf("код %v, want %s", err, tt.want)
			}
		})
	}
	if got, _ := os.Readlink(link); got != target {
		t.Error("существующий симлинк изменён")
	}

	// А ожидаемый симлинк удаляется.
	if _, err := e.w.Apply(ctx, NewPlan(Op{Code: "t"}, Change{Path: link, BeforeExists: true, BeforeLink: target})); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(link); !os.IsNotExist(err) {
		t.Errorf("симлинк не удалён: %v", err)
	}
}

// Пустые каталоги, оставшиеся от отката копии навыка, заменяются
// симлинком; каталог с файлом — нет.
func TestLinkReplacesEmptyDir(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	target := t.TempDir()
	empty := filepath.Join(e.root, "skills", "empty")
	if err := os.MkdirAll(filepath.Join(empty, "scripts"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Каталог с файлом — не место для симлинка.
	file := filepath.Join(empty, "scripts", "run.sh")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := e.w.Apply(ctx, NewPlan(Op{Code: "t"}, Change{Path: empty, AfterExists: true, Link: target})); code(err) != "write.not-regular" {
		t.Fatalf("каталог с файлом: %v", err)
	}
	// Дерево пустых каталогов, как после отката копии, заменяется.
	if err := os.Remove(file); err != nil {
		t.Fatal(err)
	}
	res, err := e.w.Apply(ctx, NewPlan(Op{Code: "t"}, Change{Path: empty, AfterExists: true, Link: target}))
	if err != nil {
		if os.IsPermission(err) {
			t.Skipf("симлинки недоступны: %v", err)
		}
		t.Fatal(err)
	}
	if got, err := os.Readlink(empty); err != nil || got != target {
		t.Fatalf("симлинк: %q %v", got, err)
	}
	rev, err := e.w.PlanRevert(ctx, res.Commit)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.w.Apply(ctx, rev); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(empty); !os.IsNotExist(err) {
		t.Errorf("после отката на месте симлинка что-то осталось: %v", err)
	}
}
