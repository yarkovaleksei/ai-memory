// Package index — индекс записей памяти и полнотекстовый поиск (SQLite FTS5
// через modernc.org/sqlite, ADR-006).
//
// Индекс — кэш: он целиком пересобирается из родных файлов агентов
// (Replace) и при смене схемы пересоздаётся. Запись в память агентов здесь нет.
package index

// Driver — имя драйвера database/sql для modernc.org/sqlite.
const Driver = "sqlite"
