package index

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/yarkovaleksei/ai-memory/internal/model"
)

// Query — фильтры, поиск, сортировка и страница списка записей.
// KindMemory — фильтр «вся автопамять»: любые виды, кроме инструкций и навыков.
const KindMemory = "memory"

type Query struct {
	ProjectID    string // пусто — все проекты
	Agent        string
	Kind         string
	Type         string
	Text         string // строка поиска как её ввёл пользователь
	IncludeTemp  bool   // показывать записи временных проектов (если проект не выбран явно)
	WarningsOnly bool
	Sort         string // "title", "modTime", "agent", "type"; пусто — по релевантности при поиске, иначе по дате
	Desc         bool
	Offset       int
	Limit        int // 0 — без ограничения
}

// Hit — запись в списке: без тела, со сниппетом совпадения при поиске.
type Hit struct {
	model.Item
	Snippet string `json:"snippet"` // с маркерами MarkStart/MarkEnd вокруг совпадений
}

// Page — страница списка и общее число записей под фильтром.
type Page struct {
	Hits  []Hit
	Total int
}

// MatchExpr превращает ввод пользователя в выражение FTS5: каждое слово — префикс,
// слова соединяются через AND. Кавычки и операторы FTS из ввода не проходят.
func MatchExpr(text string) string {
	var parts []string
	for _, w := range strings.Fields(Normalize(text)) {
		w = strings.ReplaceAll(w, `"`, "")
		if w == "" {
			continue
		}
		parts = append(parts, `"`+w+`"*`)
	}
	return strings.Join(parts, " ")
}

var sortColumns = map[string]string{
	"title":   "i.sort_title",
	"modTime": "i.mod_time",
	"agent":   "i.agent",
	"type":    "i.type",
}

// Search возвращает страницу записей под фильтром.
func (ix *Index) Search(ctx context.Context, q Query) (Page, error) {
	ix.mu.RLock()
	defer ix.mu.RUnlock()

	var (
		where []string
		args  []any
		from  = "items i"
		cols  = "i.data, ''"
	)
	match := MatchExpr(q.Text)
	if match != "" {
		from = "items i JOIN items_fts f ON f.rowid = i.rid"
		where = append(where, "items_fts MATCH ?")
		args = append(args, match)
		cols = "i.data, snippet(items_fts, -1, '" + MarkStart + "', '" + MarkEnd + "', '…', 16)"
	}
	add := func(cond string, v any) {
		where = append(where, cond)
		args = append(args, v)
	}
	if q.ProjectID != "" {
		add("i.project_id = ?", q.ProjectID)
	} else if !q.IncludeTemp {
		where = append(where, "i.temp = 0")
	}
	if q.Agent != "" {
		add("i.agent = ?", q.Agent)
	}
	switch q.Kind {
	case "":
	case KindMemory:
		where = append(where, "i.kind NOT IN (?, ?)")
		args = append(args, string(model.KindInstructions), string(model.KindSkill))
	default:
		add("i.kind = ?", q.Kind)
	}
	if q.Type != "" {
		add("i.type = ?", q.Type)
	}
	if q.WarningsOnly {
		where = append(where, "i.warnings = 1")
	}
	cond := ""
	if len(where) > 0 {
		cond = " WHERE " + strings.Join(where, " AND ")
	}

	var total int
	if err := ix.db.QueryRowContext(ctx, "SELECT count(*) FROM "+from+cond, args...).Scan(&total); err != nil {
		return Page{}, fmt.Errorf("index: подсчёт: %w", err)
	}

	order := "i.mod_time DESC"
	if col, ok := sortColumns[q.Sort]; ok {
		dir := " ASC"
		if q.Desc {
			dir = " DESC"
		}
		order = col + dir + ", i.id"
	} else if match != "" {
		order = "f.rank"
	}
	sqlText := "SELECT " + cols + " FROM " + from + cond + " ORDER BY " + order
	if q.Limit > 0 {
		sqlText += fmt.Sprintf(" LIMIT %d OFFSET %d", q.Limit, max(q.Offset, 0))
	}
	rows, err := ix.db.QueryContext(ctx, sqlText, args...)
	if err != nil {
		return Page{}, fmt.Errorf("index: поиск: %w", err)
	}
	defer rows.Close()

	page := Page{Total: total}
	for rows.Next() {
		var data, snippet string
		if err := rows.Scan(&data, &snippet); err != nil {
			return Page{}, err
		}
		var it model.Item
		if err := json.Unmarshal([]byte(data), &it); err != nil {
			return Page{}, fmt.Errorf("index: разбор записи: %w", err)
		}
		it.Body = "" // в списке тело не нужно
		page.Hits = append(page.Hits, Hit{Item: it, Snippet: snippet})
	}
	return page, rows.Err()
}

// Get возвращает запись целиком.
func (ix *Index) Get(ctx context.Context, id string) (model.Item, error) {
	items, err := ix.GetMany(ctx, []string{id})
	if err != nil {
		return model.Item{}, err
	}
	if len(items) == 0 {
		return model.Item{}, ErrNotFound
	}
	return items[0], nil
}

// ByFile возвращает запись файла целиком (без раздела): по пути находится
// факт или заметка, созданные операцией, — так история связывает перенос с
// записью, которая из него получилась.
func (ix *Index) ByFile(ctx context.Context, file string) (model.Item, error) {
	ix.mu.RLock()
	var id string
	err := ix.db.QueryRowContext(ctx, `SELECT id FROM items WHERE file = ? AND anchor = '' LIMIT 1`, file).Scan(&id)
	ix.mu.RUnlock()
	if errors.Is(err, sql.ErrNoRows) {
		return model.Item{}, ErrNotFound
	}
	if err != nil {
		return model.Item{}, fmt.Errorf("index: поиск по файлу: %w", err)
	}
	return ix.Get(ctx, id)
}

// All возвращает все записи целиком — для сравнения «каждая с каждой» при
// поиске дублей. Project в JSON не хранится, поэтому признак временного
// проекта берётся из своей колонки.
func (ix *Index) All(ctx context.Context) ([]model.Item, error) {
	ix.mu.RLock()
	defer ix.mu.RUnlock()
	rows, err := ix.db.QueryContext(ctx, `SELECT data, temp FROM items ORDER BY rid`)
	if err != nil {
		return nil, fmt.Errorf("index: все записи: %w", err)
	}
	defer rows.Close()
	var out []model.Item
	for rows.Next() {
		var data string
		var temp int
		if err := rows.Scan(&data, &temp); err != nil {
			return nil, fmt.Errorf("index: все записи: %w", err)
		}
		var it model.Item
		if err := json.Unmarshal([]byte(data), &it); err != nil {
			return nil, fmt.Errorf("index: запись: %w", err)
		}
		it.Project.Temp = temp == 1
		out = append(out, it)
	}
	return out, rows.Err()
}

// GetMany возвращает найденные записи в порядке запрошенных ID; отсутствующие пропускаются.
func (ix *Index) GetMany(ctx context.Context, ids []string) ([]model.Item, error) {
	ix.mu.RLock()
	defer ix.mu.RUnlock()

	out := make([]model.Item, 0, len(ids))
	for _, id := range ids {
		var data string
		err := ix.db.QueryRowContext(ctx, `SELECT data FROM items WHERE id = ?`, id).Scan(&data)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("index: запись %s: %w", id, err)
		}
		var it model.Item
		if err := json.Unmarshal([]byte(data), &it); err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, nil
}

// Projects возвращает проекты в порядке показа.
func (ix *Index) Projects(ctx context.Context) ([]model.Project, error) {
	return decodeAll[model.Project](ctx, ix, `SELECT data FROM projects ORDER BY ord`)
}

// Sources возвращает хранилища памяти.
func (ix *Index) Sources(ctx context.Context) ([]model.Source, error) {
	return decodeAll[model.Source](ctx, ix, `SELECT data FROM sources ORDER BY ord`)
}

func decodeAll[T any](ctx context.Context, ix *Index, query string) ([]T, error) {
	ix.mu.RLock()
	defer ix.mu.RUnlock()
	rows, err := ix.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []T
	for rows.Next() {
		var data string
		if err := rows.Scan(&data); err != nil {
			return nil, err
		}
		var v T
		if err := json.Unmarshal([]byte(data), &v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
