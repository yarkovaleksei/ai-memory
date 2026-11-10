package codex

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// git в каталоге поколения копии фикстур — как базовый коммит Codex.
func gitBaseline(t *testing.T, dir string) {
	t.Helper()
	git, err := exec.LookPath("git")
	if err != nil {
		t.Skip("нет git")
	}
	for _, args := range [][]string{
		{"init", "-q"},
		{"add", "-A"},
		{"-c", "user.name=t", "-c", "user.email=t@localhost", "-c", "commit.gpgsign=false",
			"commit", "-q", "-m", "Initialize Codex git baseline"},
	} {
		cmd := exec.Command(git, append([]string{"-C", dir}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_CONFIG_NOSYSTEM=1")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
}

func TestPendingNotes(t *testing.T) {
	home := copyHome(t)
	gen := filepath.Join(home, dirV2)
	notes := filepath.Join(gen, notesDir)
	old := filepath.Join(notes, "2026-01-03-prefer-yarn.md")

	if _, ok := PendingNotes(context.Background(), gen); ok {
		t.Error("без базового коммита статус неизвестен")
	}
	gitBaseline(t, gen)
	if p, ok := PendingNotes(context.Background(), gen); !ok || len(p) != 0 {
		t.Errorf("сразу после консолидации ничего не ждёт: %v %v", p, ok)
	}

	fresh := filepath.Join(notes, "2026-02-03-new.md")
	if err := os.WriteFile(fresh, []byte("# Новая\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(old, []byte("# Правка\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gen, summaryFile), []byte("v1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	p, ok := PendingNotes(context.Background(), gen)
	if !ok || !p[fresh] || !p[old] || len(p) != 2 {
		t.Errorf("ждут консолидации новая и изменённая заметки, сводка не в счёт: %v", p)
	}

	// Сканер переносит статус в запись.
	a := Adapter{Home: home}
	srcs, _ := a.Discover(context.Background())
	items, err := a.Scan(context.Background(), srcs[0])
	if err != nil {
		t.Fatal(err)
	}
	pending := 0
	for _, it := range items {
		if it.Pending {
			pending++
		}
	}
	if pending != 2 {
		t.Errorf("записей «ожидает консолидации»: %d", pending)
	}
}

func TestReadConsolidation(t *testing.T) {
	finished := time.Date(2026, 2, 3, 4, 0, 0, 0, time.UTC).Unix()
	tests := []struct {
		status string
		after  time.Duration
	}{
		{"done", CooldownAfterSuccess},
		{"pending", CooldownAfterSuccess},
		{"error", CooldownAfterError},
	}
	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			home := t.TempDir()
			makeJobsDB(t, filepath.Join(home, GenV2.DB()), tt.status, 0, finished)
			c := ReadConsolidation(context.Background(), home, GenV2)
			if !c.Known || c.Status != tt.status || c.NotBefore == nil ||
				!c.NotBefore.Equal(time.Unix(finished, 0).Add(tt.after)) {
				t.Errorf("%+v", c)
			}
		})
	}
	if c := ReadConsolidation(context.Background(), t.TempDir(), GenV2); c.Known {
		t.Error("без базы — неизвестно, и база не создаётся")
	}
}
