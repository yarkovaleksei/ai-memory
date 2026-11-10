# Фаза 0. Основа

**Статус:** не начата

**Цель.** Подготовить почву: каркас Wails-приложения, фикстуры вместо настоящей памяти, закрытые вопросы о форматах, без которых нельзя писать адаптеры.

## Задачи

### Исследование
- [ ] Закрыть Q1: какое поколение памяти Codex активно ([open-questions.md](../open-questions.md)).
- [ ] Закрыть Q4: восстанавливается ли путь проекта Claude по `cwd` из jsonl.
- [ ] Закрыть Q7: как показывать временные проекты.
- [ ] Обновить [storage-formats.md](../storage-formats.md) по итогам: снять пометки «предположение» там, где проверено.
- [x] Выбрать стек — принят [ADR-006](../decisions.md#adr-006-стек-go--wails-v3-react-admin-yarn-4) (Go + Wails v3, react-admin, yarn 4) вместо ADR-003 и ADR-004.
- [ ] Проверить `modernc.org/sqlite`: FTS5 доступен, кириллица ищется, префиксный поиск работает. Если нет — перейти на `mattn/go-sqlite3` с тегом `sqlite_fts5` и записать это в ADR.
- [ ] Проверить, что в `wails3 dev` привязки недоступны со сторонних страниц; итог — в раздел «Безопасность» [architecture.md](../architecture.md).

### Каркас
- [ ] `wails3 init -t react` в корне репозитория; модуль Go `ai-memory`, структура `internal/` по [architecture.md](../architecture.md#структура-репозитория).
- [ ] Фронтенд на yarn 4: `packageManager: yarn@4.18.1` в `frontend/package.json`, `.yarnrc.yml` с `nodeLinker: node-modules`, `yarn.lock` в репозитории.
- [ ] `Taskfile.yml`: задачи фронтенда через `yarn` (`yarn install --immutable`, `yarn build`), без вызовов `npm`.
- [ ] Задачи проверок: `test` (`go test ./...` и Vitest), `lint` (`gofmt -l`, `go vet`, `staticcheck`, ESLint, Prettier), `typecheck` (`tsc --noEmit`), общая `check`.
- [ ] Фронтенд: react-admin 5 и MUI подключены, пустой `Admin` открывается в окне Wails.
- [ ] Пустой сервис в `internal/app` вызывается из интерфейса через сгенерированные привязки — доказательство, что мост Go ↔ TS работает.
- [ ] `.gitignore` для `build/bin`, `frontend/node_modules`, `frontend/dist`, `.yarn/` (кроме `releases` и `plugins`, если появятся).
- [ ] Обновить раздел «Команды» в [AGENTS.md](../../AGENTS.md) по фактическим задачам.

### Фикстуры
- [ ] `fixtures/home/` — поддельный домашний каталог: два профиля Claude (с фактами всех типов, кривым frontmatter, расхождением индекса и файлов, временным проектом) и Codex v1 + v2 (summary, rollout-файлы, ad-hoc-заметки, lock-файл).
- [ ] Фикстуры собраны вручную или анонимизированы — **никаких реальных данных из памяти владельца** в репозитории.
- [ ] Корни памяти задаются переменными окружения (`AI_MEMORY_CLAUDE_DIRS`, `AI_MEMORY_CODEX_HOME`) в `internal/config`; тесты всегда смотрят в фикстуры.

## Готово, когда
- На чистом клоне `wails3 task check` проходит, а `wails3 dev` открывает окно с пустым react-admin и рабочим вызовом Go-сервиса.
- Q1, Q4, Q7 закрыты, `storage-formats.md` обновлён.
- FTS5 в выбранном драйвере SQLite проверен.

## Риски
- Формат памяти Codex поменяется с обновлением. Смягчение: в фикстурах фиксируем версии и дату снятия, парсеры терпимы к неизвестному.
- Шаблон `wails3 init` рассчитан на npm. Смягчение: сразу переводим задачи на yarn и сверяемся с SongFrame, где это уже сделано.
