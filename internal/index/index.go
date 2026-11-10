package index

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	_ "modernc.org/sqlite"

	"github.com/yarkovaleksei/ai-memory/internal/model"
)

// schemaVersion меняется при любом изменении схемы: индекс — кэш, поэтому
// при несовпадении он просто пересоздаётся.
const schemaVersion = "3"

// Маркеры подсветки в сниппетах: управляющие символы не встречаются в тексте памяти.
const (
	MarkStart = "\x02"
	MarkEnd   = "\x03"
)

// ErrNotFound — записи с таким ID нет в индексе.
var ErrNotFound = errors.New("item.not-found")

// Index — SQLite-индекс записей памяти с полнотекстовым поиском.
type Index struct {
	db *sql.DB
	mu sync.RWMutex // Replace исключителен, чтения параллельны
}

// Open открывает (или создаёт) индекс в файле. Путь ":memory:" — индекс в памяти для тестов.
func Open(path string) (*Index, error) {
	dsn := path
	if path != ":memory:" {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return nil, fmt.Errorf("index: каталог данных: %w", err)
		}
		dsn = "file:" + filepath.ToSlash(path) + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)"
	}
	db, err := sql.Open(Driver, dsn)
	if err != nil {
		return nil, fmt.Errorf("index: открытие %s: %w", path, err)
	}
	if path == ":memory:" {
		db.SetMaxOpenConns(1) // у каждого соединения своя база в памяти
	}
	ix := &Index{db: db}
	if err := ix.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return ix, nil
}

// Close закрывает базу.
func (ix *Index) Close() error { return ix.db.Close() }

func (ix *Index) migrate() error {
	var v string
	_ = ix.db.QueryRow(`SELECT value FROM meta WHERE key = 'schema'`).Scan(&v)
	if v == schemaVersion {
		return nil
	}
	stmts := []string{
		`DROP TABLE IF EXISTS meta`,
		`DROP TABLE IF EXISTS sources`,
		`DROP TABLE IF EXISTS projects`,
		`DROP TABLE IF EXISTS items`,
		`DROP TABLE IF EXISTS items_fts`,
		`CREATE TABLE meta (key TEXT PRIMARY KEY, value TEXT NOT NULL)`,
		`CREATE TABLE sources (ord INTEGER NOT NULL, id TEXT PRIMARY KEY, data TEXT NOT NULL)`,
		`CREATE TABLE projects (ord INTEGER NOT NULL, id TEXT PRIMARY KEY, temp INTEGER NOT NULL, label TEXT NOT NULL, data TEXT NOT NULL)`,
		`CREATE TABLE items (
			rid INTEGER PRIMARY KEY,
			id TEXT NOT NULL UNIQUE,
			source_id TEXT NOT NULL,
			agent TEXT NOT NULL,
			kind TEXT NOT NULL,
			type TEXT NOT NULL,
			project_id TEXT NOT NULL,
			temp INTEGER NOT NULL,
			warnings INTEGER NOT NULL,
			title TEXT NOT NULL,
			sort_title TEXT NOT NULL,
			mod_time INTEGER NOT NULL,
			file TEXT NOT NULL,
			anchor TEXT NOT NULL,
			data TEXT NOT NULL
		)`,
		`CREATE INDEX items_project ON items(project_id)`,
		`CREATE INDEX items_file ON items(file)`,
		`CREATE VIRTUAL TABLE items_fts USING fts5(title, description, body, tokenize = 'unicode61 remove_diacritics 2')`,
		`INSERT INTO meta(key, value) VALUES ('schema', '` + schemaVersion + `')`,
	}
	for _, s := range stmts {
		if _, err := ix.db.Exec(s); err != nil {
			return fmt.Errorf("index: схема: %w", err)
		}
	}
	return nil
}

// Normalize приводит текст к виду, в котором он лежит в FTS: «ё» → «е»
// (FTS5 их не приравнивает, см. fts5_test.go).
func Normalize(s string) string {
	return yoReplacer.Replace(s)
}

var yoReplacer = strings.NewReplacer("ё", "е", "Ё", "Е")

// SortKey — ключ сортировки по названию: без учёта регистра и с «ё» как «е».
// SQLite NOCASE понимает только латиницу, а сравнение по кодам ставит «Ё» перед «А».
func SortKey(s string) string {
	return strings.ToLower(Normalize(s))
}

// Replace целиком заменяет содержимое индекса одной транзакцией: читатели видят
// либо старое состояние, либо новое.
func (ix *Index) Replace(ctx context.Context, sources []model.Source, projects []model.Project, items []model.Item) error {
	ix.mu.Lock()
	defer ix.mu.Unlock()

	tx, err := ix.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("index: транзакция: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // после Commit откат ничего не делает

	for _, s := range []string{`DELETE FROM sources`, `DELETE FROM projects`, `DELETE FROM items`, `DELETE FROM items_fts`} {
		if _, err := tx.ExecContext(ctx, s); err != nil {
			return fmt.Errorf("index: очистка: %w", err)
		}
	}
	for i, s := range sources {
		data, _ := json.Marshal(s)
		if _, err := tx.ExecContext(ctx, `INSERT INTO sources(ord, id, data) VALUES (?, ?, ?)`, i, s.ID, string(data)); err != nil {
			return fmt.Errorf("index: источник %s: %w", s.ID, err)
		}
	}
	for i, p := range projects {
		data, _ := json.Marshal(p)
		if _, err := tx.ExecContext(ctx, `INSERT INTO projects(ord, id, temp, label, data) VALUES (?, ?, ?, ?, ?)`,
			i, p.ID, b2i(p.Temp), p.Label, string(data)); err != nil {
			return fmt.Errorf("index: проект %s: %w", p.ID, err)
		}
	}
	insItem, err := tx.PrepareContext(ctx, `INSERT INTO items(id, source_id, agent, kind, type, project_id, temp, warnings, title, sort_title, mod_time, file, anchor, data)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer insItem.Close()
	insFTS, err := tx.PrepareContext(ctx, `INSERT INTO items_fts(rowid, title, description, body) VALUES (?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer insFTS.Close()

	for _, it := range items {
		data, err := json.Marshal(it)
		if err != nil {
			return fmt.Errorf("index: запись %s: %w", it.ID, err)
		}
		res, err := insItem.ExecContext(ctx, it.ID, it.SourceID, string(it.Agent), string(it.Kind), it.Type, it.ProjectID,
			b2i(it.Project.Temp), b2i(len(it.Warnings) > 0), it.Title, SortKey(it.Title), it.ModTime.UnixMilli(), it.File, it.Anchor, string(data))
		if err != nil {
			return fmt.Errorf("index: запись %s: %w", it.ID, err)
		}
		rid, _ := res.LastInsertId()
		if _, err := insFTS.ExecContext(ctx, rid, Normalize(it.Title), Normalize(it.Description), Normalize(it.Body)); err != nil {
			return fmt.Errorf("index: FTS %s: %w", it.ID, err)
		}
	}
	return tx.Commit()
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}
