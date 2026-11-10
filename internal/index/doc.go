// Package index — индекс записей памяти и полнотекстовый поиск (SQLite FTS5).
//
// В фазе 0 здесь только проверка драйвера: modernc.org/sqlite (чистый Go)
// должен поддерживать FTS5, кириллицу и префиксный поиск (ADR-006).
// Сам индекс появится в фазе 1.
package index

// Driver — имя драйвера database/sql для modernc.org/sqlite.
const Driver = "sqlite"
