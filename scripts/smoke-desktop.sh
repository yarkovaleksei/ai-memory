#!/usr/bin/env bash
# Запуск окна так, как его запускает меню приложений (ADR-028): через
# пользовательский systemd, вне профиля AppArmor процесса агента. Из
# терминала Claude Code или Codex окно наследует их профиль с правом на user
# namespaces и открывается даже там, где из меню падает. Окно на несколько
# секунд появится на экране; память — копия фикстур.
#
#   bash scripts/smoke-desktop.sh [путь к ai-memory] [секунды]   (по умолчанию /usr/bin/ai-memory, 8)
set -euo pipefail
cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.."
bin="$(realpath -- "${1:-/usr/bin/ai-memory}")"
secs="${2:-8}"
[[ -x "$bin" ]] || { echo "Нет исполняемого $bin" >&2; exit 1; }
command -v systemd-run > /dev/null || { echo "Нужен systemd-run (пользовательский systemd)" >&2; exit 1; }

source scripts/fixture-env.sh
fixture_env smoke-desktop

unit="ai-memory-smoke-$$"
set +e
systemd-run --user --wait --collect --quiet --unit "$unit" \
  -E AI_MEMORY_CLAUDE_DIRS -E AI_MEMORY_CODEX_HOME -E AI_MEMORY_AGENTS_DIR \
  -E AI_MEMORY_DATA_DIR -E AI_MEMORY_CONFIG_DIR \
  timeout "$secs" "$bin"
code=$?
set -e

# timeout завершает живое окно с кодом 124: значит, оно проработало весь срок.
if [[ $code -eq 124 ]]; then
  echo "OK: $bin проработал ${secs} с при запуске как из меню"
  exit 0
fi
echo "ПРОВАЛ: $bin завершился с кодом $code раньше ${secs} с" >&2
journalctl --user -u "$unit" -o cat --no-pager 2> /dev/null | grep -m 5 -E "SIGTRAP|SIGSEGV|panic|dbus-proxy|Gtk-|error" >&2 || true
echo "Если в журнале ядра есть apparmor=\"DENIED\" … comm=\"bwrap\", не загружен профиль /etc/apparmor.d/ai-memory (ADR-028)." >&2
exit 1
