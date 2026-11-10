package codex

import (
	"bytes"
	"context"
	"database/sql"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Сроки, в которые Codex не повторяет консолидацию (Q2, storage-formats.md).
const (
	CooldownAfterSuccess = 6 * time.Hour
	CooldownAfterError   = time.Hour
)

// Consolidation — что известно о консолидации поколения по таблице jobs.
type Consolidation struct {
	Known    bool       `json:"known"`    // база и строка задачи найдены
	Status   string     `json:"status"`   // running | done | error | pending
	Finished *time.Time `json:"finished"` // конец последней завершённой консолидации
	// NotBefore — раньше этого момента Codex консолидацию не запустит; запустит
	// в первом ходе любой корневой сессии после него.
	NotBefore *time.Time `json:"notBefore"`
}

// ReadConsolidation читает строку задачи консолидации, базу открывает только на чтение.
func ReadConsolidation(ctx context.Context, home string, g Generation) Consolidation {
	path := filepath.Join(SQLiteHome(filepath.FromSlash(home)), g.DB())
	if _, err := os.Stat(path); err != nil {
		return Consolidation{}
	}
	db, err := openRO(path)
	if err != nil {
		return Consolidation{}
	}
	defer db.Close()
	var (
		status          string
		finished, retry sql.NullInt64
	)
	err = db.QueryRowContext(ctx,
		`SELECT status, finished_at, retry_at FROM jobs WHERE kind = 'memory_consolidate_global' AND job_key = 'global'`).
		Scan(&status, &finished, &retry)
	if err != nil {
		return Consolidation{}
	}
	c := Consolidation{Known: true, Status: status}
	if finished.Valid && finished.Int64 > 0 {
		t := time.Unix(finished.Int64, 0)
		c.Finished = &t
		cool := CooldownAfterSuccess
		if status == "error" {
			cool = CooldownAfterError
		}
		nb := t.Add(cool)
		if status == "error" && retry.Valid && retry.Int64 > 0 {
			nb = time.Unix(retry.Int64, 0)
		}
		c.NotBefore = &nb
	}
	return c
}

// PendingNotes — заметки, которых нет в базовом коммите каталога поколения или
// которые от него отличаются: Codex их ещё не учёл. Ключ — абсолютный путь.
// ok = false, если базового коммита нет или git недоступен: статус неизвестен.
//
// git запускается только на чтение: --no-optional-locks не даёт status
// обновить индекс в .git самого Codex.
func PendingNotes(ctx context.Context, genDir string) (pending map[string]bool, ok bool) {
	if fi, err := os.Stat(filepath.Join(genDir, ".git")); err != nil || !fi.IsDir() {
		return nil, false
	}
	git, err := exec.LookPath("git")
	if err != nil {
		return nil, false
	}
	rel := filepath.ToSlash(notesDir)
	cmd := exec.CommandContext(ctx, git, "--no-optional-locks", "-C", genDir,
		"-c", "core.quotepath=false",
		"status", "--porcelain=v1", "-z", "--no-renames", "--untracked-files=all", "--", rel)
	cmd.Env = append(os.Environ(),
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_GLOBAL="+os.DevNull,
		"GIT_OPTIONAL_LOCKS=0",
		"GIT_TERMINAL_PROMPT=0",
	)
	var out, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &stderr
	if err := cmd.Run(); err != nil {
		return nil, false
	}
	pending = map[string]bool{}
	for _, rec := range strings.Split(out.String(), "\x00") {
		if len(rec) < 4 {
			continue
		}
		// «XY путь»: с --no-renames у каждой записи ровно один путь.
		p := rec[3:]
		if !strings.EqualFold(filepath.Ext(p), ".md") {
			continue
		}
		pending[filepath.Join(genDir, filepath.FromSlash(p))] = true
	}
	return pending, true
}
