# Фаза 6. Упаковка

**Статус:** в работе

**Цель.** Устанавливать и запускать ai-memory как обычное приложение на Linux, macOS и Windows ([ADR-009](../decisions.md#adr-009-linux-macos-и-windows-с-самого-начала)).

`wails3 build` уже даёт один бинарь, поэтому фаза сводится к установочным пакетам, ярлыкам и обновлению. Память только локальная: один компьютер — одна установка (Q5 закрыт).

## Задачи
- [x] Linux: AppImage и/или deb через `wails3 package`, ярлык `.desktop` и иконка. Собираются deb, rpm и пакет Arch; AppImage — нет ([ADR-027](../decisions.md#adr-027-упаковка-без-подписи-deb-rpm-arch-dmg-и-nsis-с-mcp-мостом)).
- [x] macOS: `.app` в `.dmg`. Решить, подписывать ли и нотаризовать (нужен аккаунт Apple Developer); без этого — инструкция, как открыть неподписанное приложение. Универсальный `.app`, подпись ad-hoc, без нотаризации; инструкция — в README, раздел «Установка».
- [x] Windows: установщик (NSIS через Wails), проверка WebView2. Решить вопрос подписи. Без подписи, для текущего пользователя; как пройти SmartScreen — в README.
- [x] Записать выбранные форматы и решение о подписи в ADR: [ADR-027](../decisions.md#adr-027-упаковка-без-подписи-deb-rpm-arch-dmg-и-nsis-с-mcp-мостом).
- [x] Сборка пакетов в CI на GitHub Actions по тегу версии, артефакты — в релиз GitHub: `.github/workflows/release.yml`. В CI — только macOS и Windows; Linux владелец собирает и выкладывает локально: `wails3 task release` и `release:publish` ([ADR-029](../decisions.md#adr-029-linux-собирается-локально-macos-и-windows--в-ci)).
- [x] Бинарь MCP-моста ставится вместе с приложением и находится по известному пути на каждой системе: `/usr/bin/ai-memory-mcp`, `/Applications/ai-memory.app/Contents/MacOS/ai-memory-mcp`, `%LOCALAPPDATA%\Programs\ai-memory\ai-memory-mcp.exe`.
- [x] Навыки сборки и выпуска `ai-memory-build` и `ai-memory-release` по образцу проектов владельца: черновик релиза, сверка вложений по API, проверки `smoke:mcp` и `smoke:desktop` ([ADR-030](../decisions.md#adr-030-релиз-выкладывается-черновиком-и-публикуется-после-сверки-по-api)).
- [x] Версия приложения видна в интерфейсе и в `ai-memory-mcp --version`. Берётся из тега `vX.Y.Z` (`wails3 task version`) и вшивается в оба бинаря.
- [ ] Проверки `check.yml` проходят на macOS и Windows. Первый прогон после push упал: на macOS временный каталог лежит за симлинком `/var → /private/var`, и свой навык считался подключённым симлинком; на Windows — CRLF в фикстурах (нужен `.gitattributes`), «Filename too long» в git бэкапа (`core.longpaths`), `syscall.Flock` в тестах, бит исполнения и пути в тестах. Исправлено, проверено на Linux с `TMPDIR` за симлинком и кросс-компиляцией под Windows и macOS; на самих системах — ждёт прогона CI.

### Ручная приёмка
- [x] Linux, локально (2026-11-11): `wails3 task linux:package APP_VERSION=0.6.0` собирает deb, rpm и пакет Arch. В deb — `/usr/bin/ai-memory` и `/usr/bin/ai-memory-mcp`, версия 0.6.0, зависимости `libgtk-4-1` и `libwebkitgtk-6.0-4`; `ai-memory-mcp -version` печатает 0.6.0. Содержимое rpm и пакета Arch не проверено: на машине нет `rpm` и `bsdtar`.
- [x] Windows, кросс-сборка на Linux: установщик NSIS собирается, внутри `ai-memory.exe` (GUI) и `ai-memory-mcp.exe` (консоль). Универсальный мост для macOS собирается (`darwin:build:mcp:universal`).
- [x] Установить deb на Linux: окно открывается из меню, `ai-memory-mcp` подключается к агентам по `/usr/bin`, после переустановки индекс и бэкапы на месте.
  - 2026-11-11, Ubuntu (Linux): deb 0.0.1 встал, ярлык и иконка в меню есть. Мост из `/usr/bin` отвечает по stdio и на копии фикстур находит факт Claude; с агентами мост проверен в приёмке фазы 5.
  - Окно из меню сначала не открывалось: AppArmor в Ubuntu не давал песочнице WebKit создать user namespace. Исправлено профилем AppArmor в deb ([ADR-028](../decisions.md#adr-028-deb-ставит-профиль-apparmor-с-правилом-userns)). После `apt install --reinstall` окно открывается из меню.
  - Переустановка не тронула каталог данных: бэкапы с последней операцией приёмки фазы 4 на месте, индекс окно обновило само.
  - Иконка приложения заменена на собственную (`build/appicon.png`): из неё собираются `.icns`, `.ico`, иконка окна и оформление `.dmg`. `Assets.car` и `appicon.icon` из шаблона Wails удалены.
- [ ] macOS и Windows: установить пакеты из первого релиза CI, открыть по инструкции из README, проверить путь к мосту. Заодно закрыть [Q10](../open-questions.md#q10-где-лежит-память-агентов-на-macos-и-windows).

## Готово, когда
- ai-memory ставится одним пакетом и открывается из меню приложений на каждой из трёх систем.
- Обновление выполняется установкой нового пакета и не трогает индекс и бэкапы в [каталоге данных](../architecture.md#каталоги-ai-memory).
