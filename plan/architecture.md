# Архитектура

Стек и причины выбора — в [ADR-006](decisions.md#adr-006-стек-go--wails-v3-react-admin-yarn-4).

## Принципы

1. **Родные хранилища — источник правды.** Своей памяти у системы нет: индекс и кэш можно в любой момент удалить и пересобрать с диска.
2. **По умолчанию только чтение.** Запись включается явно и всегда идёт через предпросмотр diff.
3. **Ничего не теряем.** Перед каждой записью — снимок затрагиваемых файлов, после — коммит в собственный репозиторий бэкапов. Любую операцию можно откатить.
4. **Пишем так, как агент ожидает.** Claude — файлы фактов и индекс `MEMORY.md`; Codex — только ad-hoc-заметки (см. [storage-formats.md](storage-formats.md)).
5. **Локально.** Это десктоп-приложение без сетевого порта, данные никуда не уходят. Исключение — LLM-операции, которые явно вызывает пользователь, и они идут через установленные CLI агентов.

## Компоненты

```
┌──────────────────────────── frontend/ ───────────────────────────┐
│ React + react-admin + MUI, dataProvider поверх привязок Wails    │
│ дерево проектов │ список записей │ редактор + diff               │
└───────────────────────────────┬──────────────────────────────────┘
                                │ привязки Wails (вызовы) и события
┌──────────────────────────── internal/app ────────────────────────┐
│ сервисы Wails: Projects, Items, Plans, History                   │
│ тонкий слой: преобразование типов, события «индекс обновлён»     │
└───────────────────────────────┬──────────────────────────────────┘
                                │ обычные вызовы Go
┌──────────────────────────── ядро internal/ ──────────────────────┐
│ model        Source, Project, MemoryItem                         │
│ adapter/claude, adapter/codex   чтение и план записи             │
│ index        SQLite FTS5          watch     fsnotify             │
│ writer       план → снимок → атомарная запись → коммит бэкапа    │
│ backup       git-репозиторий бэкапов    llm   claude -p, codex exec │
└───────────────────────────────┬──────────────────────────────────┘
                                │ файловая система
        ~/.claude*/projects/*/memory          ~/.codex/memories_v2

cmd/ai-memory-mcp  — отдельный бинарь, MCP-сервер поверх того же ядра (фаза 5)
```

Правило зависимостей: ядро не импортирует `internal/app`, Wails и ничего из `frontend/`. Весь Wails-специфичный код живёт в `internal/app` и `main.go`.

## Структура репозитория

```
main.go                 запуск Wails-приложения
internal/
  model/                типы ядра
  adapter/claude/       адаптер Claude Code
  adapter/codex/        адаптер Codex
  index/                индекс и поиск
  watch/                наблюдение за файлами
  writer/               планы записи и их применение
  backup/               репозиторий бэкапов
  llm/                  вызовы CLI агентов (фаза 3)
  config/               корни памяти, переменные окружения
  app/                  сервисы Wails
cmd/ai-memory-mcp/      MCP-сервер (фаза 5)
frontend/               Vite + React + react-admin, yarn 4
fixtures/               поддельные каталоги памяти для тестов
build/                  ресурсы сборки Wails
Taskfile.yml            задачи сборки и проверок
```

## Каталоги ai-memory

Приложение поддерживает Linux, macOS и Windows ([ADR-009](decisions.md#adr-009-linux-macos-и-windows-с-самого-начала)), поэтому свои файлы кладёт по правилам каждой системы. Пути вычисляет `internal/config`, в остальном коде они не повторяются.

| | Linux | macOS | Windows |
|---|---|---|---|
| Настройки (`config.toml`) | `$XDG_CONFIG_HOME/ai-memory`, по умолчанию `~/.config/ai-memory` | `~/Library/Application Support/ai-memory` | `%AppData%\ai-memory` |
| Данные: индекс `index.db`, бэкапы `backups/` | `$XDG_DATA_HOME/ai-memory`, по умолчанию `~/.local/share/ai-memory` | `~/Library/Application Support/ai-memory` | `%LocalAppData%\ai-memory` |

Ниже «каталог данных» и «каталог настроек» означают эти пути.

## Модель данных

```go
type Agent string // "claude" | "codex"

// Source — одно хранилище памяти: профиль Claude или поколение памяти Codex.
type Source struct {
    Agent   Agent
    Profile string // "claude:~/.claude", "claude:~/.claude-work", "codex:v2"
    Root    string // абсолютный путь корня памяти
}

// Project объединяет память одного проекта из всех хранилищ.
type Project struct {
    ID      string  // нормализованный абсолютный путь или "global"
    Path    *string // nil для глобальной памяти
    Label   string
    Sources []Source
}

type ItemKind string

const (
    KindClaudeFact     ItemKind = "claude-fact"           // файл факта Claude
    KindClaudeIndex    ItemKind = "claude-index"          // MEMORY.md
    KindCodexSection   ItemKind = "codex-summary-section" // раздел memory_summary.md
    KindCodexRollout   ItemKind = "codex-rollout"         // rollout_summaries/*.md
    KindCodexAdHocNote ItemKind = "codex-adhoc-note"      // extensions/ad_hoc/notes/*.md

    // Фаза 4 (ADR-010): инструкции и навыки.
    KindInstructions ItemKind = "instructions" // CLAUDE.md, AGENTS.md: глобальные и проектные
    KindSkill        ItemKind = "skill"        // каталог с SKILL.md
)

// WriteMode показывает, что интерфейс может сделать с записью.
type WriteMode string

const (
    WriteDirect  WriteMode = "direct"   // факты Claude, ad-hoc-заметки
    WriteViaNote WriteMode = "via-note" // сгенерированное Codex: только через новую заметку
    WriteNone    WriteMode = "none"     // MEMORY.md: его ведёт writer
)

type MemoryItem struct {
    ID          string // стабильный: агент + относительный путь (+ якорь раздела)
    Source      Source
    ProjectID   string
    Kind        ItemKind
    Type        string // user | feedback | project | reference, если есть
    Title       string
    Description string
    Body        string     // Markdown без frontmatter
    Frontmatter *yaml.Node // исходный узел: сохраняет порядок и неизвестные поля
    Links       []string   // [[slug]] и ссылки на rollout_summaries
    File        string     // абсолютный путь
    Anchor      string     // для разделов внутри файла
    ModTime     time.Time
    Hash        string // для оптимистичной блокировки при записи
    Write       WriteMode
    Warnings    []Warning // неожиданная структура, расхождение с индексом
}

// Warning — код и параметры, а не текст: текст собирает фронтенд по словарю (ADR-008).
type Warning struct {
    Code   string            // например "claude.index.missing-entry"
    Params map[string]string // например {"file": "pr-base-branches.md"}
}
```

Во фронтенд `MemoryItem` уходит через DTO в `internal/app`: без `yaml.Node`, frontmatter — уже разобранным объектом.

## Адаптеры

У каждого адаптера одинаковый контракт:

```go
type Adapter interface {
    Discover(ctx context.Context) ([]Source, error)                      // найти корни
    Scan(ctx context.Context, src Source, emit func(MemoryItem)) error   // прочитать всё
    PlanWrite(ctx context.Context, op WriteOp) (WritePlan, error)        // что изменится, без записи
}
```

- **Claude.** Обходит профили, парсит frontmatter (терпимо к вариантам), восстанавливает путь проекта по `cwd` из jsonl, сверяет индекс `MEMORY.md` с файлами и записывает расхождения в `Warnings` (факт без строки в индексе, строка без файла).
- **Codex.** Читает активное поколение (по умолчанию v2, v1 — опционально, только чтение), режет `memory_summary.md` на разделы, читает заголовки `rollout_summaries`, перечисляет ad-hoc-заметки. SQLite Codex открывает только на чтение и только при необходимости (счётчики `usage_count`, `last_usage`).

## Запись

Операции: `create`, `update`, `delete`, `move` (перенос в другой проект или агент), `merge` (несколько записей → одна).

Каждая операция проходит один и тот же путь:

1. **План.** Адаптер превращает операцию в список файловых изменений. Для Claude: файл факта и строка в `MEMORY.md`. Для Codex: новая или изменённая ad-hoc-заметка.
2. **Предпросмотр.** Интерфейс показывает unified diff по каждому файлу. Без подтверждения ничего не пишется.
3. **Проверки перед записью:**
   - `Hash` каждого файла совпадает с тем, что видел пользователь, иначе конфликт и повторный план;
   - для Codex нет `memories_v2.lock` с живым процессом, иначе ждём или отказываем;
   - путь лежит внутри разрешённых корней (защита от `..` и симлинков). Исключение — цель симлинка навыка: она добавляется в разрешённые корни явно, и план показывает, что изменение затронет всех, кто ссылается на этот навык ([ADR-010](decisions.md#adr-010-инструкции-и-навыки-агентов--тоже-в-охвате));
   - для проектных инструкций — предупреждение, что файл лежит в git-репозитории проекта.
4. **Снимок** текущего содержимого в репозиторий бэкапов (`backups/` в каталоге данных, отдельный git).
5. **Атомарная запись:** временный файл в том же каталоге и `os.Rename`. На Windows замена не удаётся, пока файл открыт другим процессом: writer распознаёт эту ошибку и повторяет попытку с паузой, а после нескольких неудач возвращает конфликт.
6. **Коммит** в репозиторий бэкапов с описанием операции. Откат — обратное применение коммита, тоже через план и diff.

Запрещено всегда: писать в сгенерированные файлы Codex, удалять ad-hoc-заметки, трогать что-либо вне каталогов памяти.

## Индекс и поиск

- SQLite в `index.db` в каталоге данных, таблица записей и FTS5 по заголовку, описанию и телу.
- Пересборка с нуля при запуске и инкрементально по событиям `fsnotify`. После обновления сервис шлёт событие Wails, и интерфейс перечитывает список.
- Стандартный токенизатор не знает русских словоформ, поэтому в фазе 1 поиск идёт по префиксам (`ветк*`); стемминг — позже, если понадобится.
- `remove_diacritics` в FTS5 работает только для латиницы: «ё» и «е» для него разные буквы. Индекс сам приводит «ё» к «е» и в индексируемом тексте, и в запросе. Проверено тестом `internal/index/fts5_test.go` на `modernc.org/sqlite`.
- Поиск дублей ([фаза 3](phases/03-cross-agent.md)): сначала дешёвые кандидаты (FTS и сходство по n-граммам), затем по запросу пользователя — LLM-сравнение.

## LLM-операции

Только по явному действию пользователя: «найти дубли», «объединить», «переформулировать для другого агента». Вызов через установленные CLI в неинтерактивном режиме (`claude -p`, `codex exec`) через `os/exec`, без API-ключей ([ADR-005](decisions.md#adr-005-llm-операции-через-cli-агентов)). Результат LLM — всегда черновик, который проходит обычный путь записи с diff.

## Безопасность

- В собранном приложении сетевого порта нет: интерфейс вызывает Go напрямую через привязки Wails.
- В режиме разработки (`wails3 dev`) фронтенд раздаёт dev-сервер Vite на `127.0.0.1:9245`. Проверено 2026-11-10 на Linux: процесс приложения не слушает ни одного TCP-порта, а страница, открытая в обычном браузере с адреса Vite, получает 404 на `POST /wails/runtime` — вызвать Go-сервисы снаружи окна нельзя, видна только вёрстка. Кроме того, `wails3 task dev` по умолчанию работает на фикстурах.
- Ядро принимает от интерфейса только идентификаторы записей и планов, а не произвольные пути. Каждый путь перед чтением и записью проверяется на принадлежность разрешённым корням.
