package index

import (
	"database/sql"
	"slices"
	"testing"

	_ "modernc.org/sqlite"
)

// TestFTS5Cyrillic фиксирует то, на что опирается поиск фазы 1: FTS5 доступен,
// unicode61 приводит кириллицу к нижнему регистру, префиксный запрос находит
// словоформы, а точный — нет (стемминга нет, см. architecture.md), «ё» не
// равна «е».
func TestFTS5Cyrillic(t *testing.T) {
	db, err := sql.Open(Driver, ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()

	if _, err := db.Exec(`CREATE VIRTUAL TABLE items USING fts5(title, body, tokenize = 'unicode61 remove_diacritics 2')`); err != nil {
		t.Fatalf("FTS5 недоступен: %v", err)
	}
	rows := [][2]string{
		{"Базовые ветки для PR", "Нацеливай PR на ветку main"},
		{"Проверки без контейнера", "Запускай линтер локально"},
		{"Ёлка", "Ещё одна запись про ёжика"},
	}
	for _, r := range rows {
		if _, err := db.Exec(`INSERT INTO items(title, body) VALUES (?, ?)`, r[0], r[1]); err != nil {
			t.Fatalf("insert: %v", err)
		}
	}

	tests := []struct {
		query string
		want  []string
	}{
		{"ветки", []string{"Базовые ветки для PR"}},
		{"ВЕТКИ", []string{"Базовые ветки для PR"}},
		{"ветка", nil}, // без стемминга словоформа не находится
		{"ветк*", []string{"Базовые ветки для PR"}},
		{"линтер", []string{"Проверки без контейнера"}},
		// remove_diacritics работает только для латиницы: «ё» и «е» — разные буквы.
		// Индекс фазы 1 должен сам приводить «ё» к «е» и в тексте, и в запросе.
		{"елка", nil},
		{"ёлка", []string{"Ёлка"}},
		{"pr AND main", []string{"Базовые ветки для PR"}},
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			got := match(t, db, tt.query)
			if !slices.Equal(got, tt.want) {
				t.Errorf("MATCH %q = %q, want %q", tt.query, got, tt.want)
			}
		})
	}
}

func match(t *testing.T, db *sql.DB, q string) []string {
	t.Helper()
	rs, err := db.Query(`SELECT title FROM items WHERE items MATCH ? ORDER BY rank`, q)
	if err != nil {
		t.Fatalf("query %q: %v", q, err)
	}
	defer rs.Close()
	var out []string
	for rs.Next() {
		var s string
		if err := rs.Scan(&s); err != nil {
			t.Fatalf("scan: %v", err)
		}
		out = append(out, s)
	}
	if err := rs.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}
	return out
}
