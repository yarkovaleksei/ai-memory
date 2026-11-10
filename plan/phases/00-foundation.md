# Фаза 0. Основа

**Статус:** готова (2026-11-10, на Linux; macOS и Windows — после публикации репозитория и первого прогона CI)

**Цель.** Подготовить почву: каркас Wails-приложения, фикстуры вместо настоящей памяти, закрытые вопросы о форматах, без которых нельзя писать адаптеры.

## Задачи

### Исследование
- [x] Закрыть Q1: активно поколение памяти Codex v2, v1 заброшено ([open-questions.md](../open-questions.md#закрытые)).
- [x] Закрыть Q4: путь проекта Claude восстанавливается по `cwd` везде, где есть файлы сессий; для остальных нужен подбор по диску.
- [x] Закрыть Q7: временные проекты — отдельная скрытая группа.
- [x] Обновить [storage-formats.md](../storage-formats.md) по итогам: правило кодирования пути, активное поколение Codex, `autoMemoryDirectory`, известное о macOS и Windows.
- [x] Выбрать стек — принят [ADR-006](../decisions.md#adr-006-стек-go--wails-v3-react-admin-yarn-4) (Go + Wails v3, react-admin, yarn 4) вместо ADR-003 и ADR-004.
- [x] Решить, на каких системах работаем и что входит в охват — приняты [ADR-009](../decisions.md#adr-009-linux-macos-и-windows-с-самого-начала) (Linux, macOS, Windows) и [ADR-010](../decisions.md#adr-010-инструкции-и-навыки-агентов--тоже-в-охвате) (инструкции и навыки — фаза 4); закрыты Q5, Q6, Q9.
- [x] Решить, открытый ли проект и на каком языке интерфейс — приняты [ADR-007](../decisions.md#adr-007-открытый-проект-под-лицензией-mit) (MIT) и [ADR-008](../decisions.md#adr-008-интерфейс-на-русском-через-i18n-с-первого-дня) (русский через i18n).
- [x] Русский пакет сообщений react-admin: `ra-language-russian` 5.4.5 совместим с react-admin 5, свой словарь `ra.*` не нужен.
- [x] Проверить `modernc.org/sqlite`: FTS5 доступен, кириллица ищется без учёта регистра, префиксный поиск работает (`internal/index/fts5_test.go`). Найдено: «ё» и «е» FTS5 не приравнивает — нормализация в индексе фазы 1.
- [x] Проверить, что в `wails3 dev` привязки недоступны со сторонних страниц: приложение не слушает портов, Vite отвечает 404 на `/wails/runtime` ([architecture.md](../architecture.md#безопасность)).

### Каркас
- [x] `wails3 init -t react` в корне репозитория; модуль Go `github.com/yarkovaleksei/ai-memory`, структура `internal/` по [architecture.md](../architecture.md#структура-репозитория). Мобильные цели и серверный режим шаблона убраны.
- [x] Фронтенд на yarn 4: `packageManager: yarn@4.18.1` в `frontend/package.json`, `.yarnrc.yml` с `nodeLinker: node-modules` и порогом возраста версий 7 дней, `yarn.lock` в репозитории.
- [x] `Taskfile.yml`: задачи фронтенда через `yarn` (`PACKAGE_MANAGER: yarn`, `yarn install --immutable`), без вызовов `npm`.
- [x] Задачи проверок: `test`, `lint` (`gofmt -l`, `go vet`, `staticcheck` как `go tool`, ESLint, Prettier), `typecheck`, общая `check` — проходят на Linux; прогон не меняет `go.mod`, `go.sum`, `yarn.lock` и привязки.
- [x] Фронтенд: react-admin 5 и MUI подключены, `Admin` открывается в окне Wails (проверено на Linux в `wails3 task dev`).
- [x] `i18nProvider` с русским словарём в `frontend/src/i18n/ru.ts`; правило `i18next/no-literal-string` ловит текст в JSX, тест `src/test/i18n-lint.test.ts` следит, что правило не выключилось.
- [x] Сервис `InfoService` в `internal/app` вызывается из интерфейса через сгенерированные привязки: в живом окне показаны версия и пути к фикстурам; тест фронтенда проверяет экран с подменой привязок.
- [x] `.gitignore` для `bin/`, `.task/`, `frontend/node_modules`, `frontend/dist`, `frontend/.yarn/` (кроме `releases`, `plugins`, `patches`, `sdks`).
- [x] Обновить раздел «Команды» в [AGENTS.md](../../AGENTS.md) по фактическим задачам.
- [x] `internal/config`: каталоги настроек и данных для Linux, macOS и Windows ([architecture.md](../architecture.md#каталоги-ai-memory)) с табличными тестами на каждую систему.
- [x] Workflow GitHub Actions `.github/workflows/check.yml`: `wails3 task check` и сверка привязок на матрице `ubuntu-latest`, `macos-latest`, `windows-latest`. **Не запускался:** заработает после публикации репозитория.

### Фикстуры
- [x] `fixtures/home/` — поддельный домашний каталог: два профиля Claude и Codex v1 + v2; список случаев — в [fixtures/README.md](../../fixtures/README.md).
- [x] Фикстуры написаны вручную, данные выдуманы.
- [x] Пути в тестах собираются через `filepath`; правила путей Windows и macOS проверяются тестами `internal/config` на любой системе.
- [x] Корни памяти задаются переменными `AI_MEMORY_CLAUDE_DIRS`, `AI_MEMORY_CODEX_HOME`, `AI_MEMORY_DATA_DIR`, `AI_MEMORY_CONFIG_DIR`; `wails3 task dev` по умолчанию направляет приложение на фикстуры.

## Готово, когда
- На чистом клоне `wails3 task check` проходит, а `wails3 task dev` открывает окно с react-admin и рабочим вызовом Go-сервиса. ✔ Чистый клон коммита `5b6d03a` на Linux: `check` за 33 с, привязки совпадают с Go-кодом, `wails3 build` собирает `bin/ai-memory`, рабочее дерево после прогона чистое.
- Q1, Q4, Q7 закрыты, `storage-formats.md` обновлён. ✔
- FTS5 в выбранном драйвере SQLite проверен. ✔

## Риски
- Формат памяти Codex поменяется с обновлением. Смягчение: в фикстурах зафиксированы версии и дата снятия, парсеры терпимы к неизвестному.
- macOS и Windows до публикации репозитория не проверяются вовсе: CI-матрица написана, но не запускалась.
