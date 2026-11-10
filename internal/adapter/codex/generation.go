package codex

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
	_ "modernc.org/sqlite" // драйвер "sqlite" для чтения баз Codex в режиме только чтения
)

// Generation — поколение памяти Codex.
type Generation string

const (
	GenV1 Generation = "v1"
	GenV2 Generation = "v2"
)

// Dir — каталог поколения в CODEX_HOME.
func (g Generation) Dir() string {
	if g == GenV2 {
		return dirV2
	}
	return dirV1
}

// LockFile — файл блокировки консолидации рядом с каталогом поколения.
func (g Generation) LockFile() string { return g.Dir() + ".lock" }

// DB — база конвейера памяти поколения в sqlite_home.
func (g Generation) DB() string {
	if g == GenV2 {
		return "memories_v2_1.sqlite"
	}
	return "memories_1.sqlite"
}

// Parse принимает «v1»/«v2»; иное — пустое поколение.
func Parse(s string) Generation {
	switch Generation(s) {
	case GenV1, GenV2:
		return Generation(s)
	}
	return ""
}

// Причины выбора поколения — коды для интерфейса.
const (
	ReasonSettings = "settings" // задано в config.toml ai-memory
	ReasonConfig   = "config"   // [memories] version в config.toml Codex
	ReasonActivity = "activity" // самая свежая работа конвейера в базах Codex
	ReasonOnlyOne  = "only-one" // есть каталог только одного поколения
	ReasonDefault  = "default"  // умолчание Codex — v1
)

type codexConfig struct {
	SQLiteHome string `toml:"sqlite_home"`
	Memories   struct {
		Version string `toml:"version"`
	} `toml:"memories"`
}

func readCodexConfig(home string) codexConfig {
	var c codexConfig
	// Ошибки разбора не мешают: настройки Codex — подсказка, а не требование.
	_, _ = toml.DecodeFile(filepath.Join(home, "config.toml"), &c)
	return c
}

// SQLiteHome — каталог баз Codex: sqlite_home из его config.toml или CODEX_HOME.
func SQLiteHome(home string) string {
	if h := readCodexConfig(home).SQLiteHome; h != "" {
		return h
	}
	return home
}

// DetectGeneration определяет, в какое поколение Codex сейчас пишет память
// (Q3: клиент Codex решает это сам, и заметка в чужом поколении не будет учтена).
func DetectGeneration(ctx context.Context, home string) (Generation, string) {
	cfg := readCodexConfig(home)
	if g := Parse(cfg.Memories.Version); g != "" {
		return g, ReasonConfig
	}
	v1, v2 := isDir(filepath.Join(home, dirV1)), isDir(filepath.Join(home, dirV2))
	switch {
	case v2 && !v1:
		return GenV2, ReasonOnlyOne
	case v1 && !v2:
		return GenV1, ReasonOnlyOne
	}
	dbHome := cfg.SQLiteHome
	if dbHome == "" {
		dbHome = home
	}
	a1 := lastActivity(ctx, filepath.Join(dbHome, GenV1.DB()))
	a2 := lastActivity(ctx, filepath.Join(dbHome, GenV2.DB()))
	switch {
	case a2 > a1:
		return GenV2, ReasonActivity
	case a1 > a2:
		return GenV1, ReasonActivity
	}
	return GenV1, ReasonDefault
}

// lastActivity — время последней работы конвейера памяти по таблице jobs.
func lastActivity(ctx context.Context, path string) int64 {
	if _, err := os.Stat(path); err != nil {
		return 0 // базы нет — не создаём её открытием
	}
	db, err := openRO(path)
	if err != nil {
		return 0
	}
	defer db.Close()
	var t sql.NullInt64
	_ = db.QueryRowContext(ctx, `SELECT max(coalesce(finished_at, started_at, 0)) FROM jobs`).Scan(&t)
	return t.Int64
}

// openRO открывает базу Codex только на чтение (AGENTS.md).
func openRO(path string) (*sql.DB, error) {
	return sql.Open("sqlite", "file:"+filepath.ToSlash(path)+"?mode=ro")
}
