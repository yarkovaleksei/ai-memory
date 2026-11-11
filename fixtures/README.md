# Фикстуры

Поддельный домашний каталог с памятью Claude Code и Codex. Тесты и `wails3 dev` работают только с ним, а не с настоящей памятью (см. «Безопасность реальной памяти» в [AGENTS.md](../AGENTS.md)).

Все данные выдуманы: пользователь `me`, проекты `/home/me/projects/demo` и `/tmp/scratch-42`, трекер `tracker.example.test`. Форматы повторяют то, что описано в [plan/storage-formats.md](../plan/storage-formats.md) на 2026-11-10 (Claude Code 2.x, Codex CLI 0.159).

## Как направить приложение на фикстуры

`wails3 task dev` делает это сам: готовит копию фикстур в `.task/dev-home` (`cmd/devhome` заменяет в ней выдуманный `/home/me` путём копии, чтобы проекты из памяти указывали на её каталоги), выставляет `AI_MEMORY_CLAUDE_DIRS`, `AI_MEMORY_CODEX_HOME`, `AI_MEMORY_AGENTS_DIR`, а данные и настройки кладёт в `.task/`. На настоящей памяти — `wails3 task dev REAL=true`.

Вручную, например для собранного бинаря:

```bash
export AI_MEMORY_CLAUDE_DIRS="$PWD/fixtures/home/.claude:$PWD/fixtures/home/.claude-work"
export AI_MEMORY_CODEX_HOME="$PWD/fixtures/home/.codex"
export AI_MEMORY_AGENTS_DIR="$PWD/fixtures/home/.agents"
export AI_MEMORY_DATA_DIR="$PWD/.task/ai-memory-data"
```

На Windows разделитель списка в `AI_MEMORY_CLAUDE_DIRS` — `;`.

## Какие случаи покрыты

### Claude Code

| Где | Случай |
|---|---|
| `.claude/projects/-home-me-projects-demo/` | Обычный проект: факты всех типов (`feedback`, `project`, `reference`), путь восстанавливается по `cwd` из `session-0001.jsonl` |
| там же, `broken-frontmatter.md` | Некорректный YAML — нужно предупреждение, а не падение |
| там же, `orphan-fact.md` | Файл есть, строки в `MEMORY.md` нет |
| там же, `MEMORY.md` → `missing-fact.md` | Строка в индексе есть, файла нет |
| там же, `release-flow.md` | Ссылка `[[prefer-yarn]]` на другой факт |
| `.claude/projects/-home-me-projects-my-app-v2/` | Нет `*.jsonl`: путь не восстанавливается, показываем закодированное имя; `legacy-type.md` — старый формат с `type` на верхнем уровне |
| `.claude/projects/-tmp-scratch-42/` | Временный проект (Q7) |
| `.claude/projects/-home-me-projects-empty/` | Проект без каталога `memory/`: путь берётся из сессии, и в списке видны его инструкции из `projects/empty/CLAUDE.md` (ADR-025) |
| `.claude-work/projects/-home-me-projects-demo/` | Тот же проект во втором профиле; `yarn-only.md` — дубль факта `prefer-yarn` |
| `.claude/skills/`, `CLAUDE.md` в обоих профилях | Навык и глобальные инструкции (фаза 4) |
| `.claude/skills/release-notes/scripts/last-tag.sh` | Исполняемый скрипт навыка: копия навыка в другого агента сохраняет права |
| `.claude/rules/style.md` | Глобальное правило с frontmatter `paths` |
| `.claude/skills/synced/bucket-1/` | Навык, синхронизированный с аккаунтом claude.ai: только чтение |

### Codex

| Где | Случай |
|---|---|
| `.codex/memories_v2/` | Активное поколение: сводка, две rollout-сводки (проект и временный каталог), ad-hoc-заметка |
| `.codex/memories_v2.lock` | Файл блокировки (пустой) |
| `.codex/config.toml` | `[memories] version = "v2"`: активное поколение задано явно, баз Codex в фикстурах нет (Q3) |
| `.codex/memories/` | Заброшенное поколение v1: `MEMORY.md`, `raw_memories.md`, навык из памяти, старая заметка |
| `.codex/skills/`, `.codex/AGENTS.md` | Навык и глобальные инструкции (фаза 4) |
| `.codex/skills/lint-fix/agents/openai.yaml` | Оформление навыка для Codex: Claude его не читает — предупреждение при установке в Claude |
| `.codex/skills/.system/plan-helper/` | Системный навык Codex: только чтение |
| `.codex/skills/no-description/` | Навык без `description`: Codex его не загрузит — нужно предупреждение |

### Общий каталог и проект

| Где | Случай |
|---|---|
| `.agents/skills/commit-message/` | Навык в общем `~/.agents/skills`: его читает Codex |
| `projects/demo/` | Каталог проекта `/home/me/projects/demo`: `CLAUDE.md` с импортом `@AGENTS.md`, `CLAUDE.local.md`, `AGENTS.md`, затеняющий его `AGENTS.override.md`, правило `.claude/rules/testing.md`, навыки `.claude/skills/deploy-preview` (с полями, которых нет у Codex) и `.agents/skills/changelog`. Тесты находят его по пути из памяти, заменив `/home/me` путём копии фикстур |

## Чего здесь нет и почему

- **SQLite-баз Codex** (`memories_v2_1.sqlite`) — бинарные файлы в репозитории неудобны; тесты, которым они нужны, создают базу сами по схеме из `storage-formats.md`.
- **git-репозиториев внутри каталогов памяти Codex** — вложенный `.git` нельзя закоммитить; тесты создают его во временном каталоге.
- **Навыков-симлинков** — симлинки в git ненадёжны на Windows; тесты фазы 4 создают их во временном каталоге.
- **Восстановления пути подбором по диску** — зависит от абсолютных путей машины; тест строит дерево каталогов во временном каталоге.

## Правила

- Никаких реальных данных: ни путей, ни имён проектов, ни текста из чьей-то памяти.
- Новый случай — новый файл и строка в таблице выше в том же коммите.
- Фикстуры только читаются. Тест, который пишет, копирует их во временный каталог (`t.TempDir()`).
