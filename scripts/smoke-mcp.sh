#!/usr/bin/env bash
# Проверка собранного MCP-моста по stdio (ADR-026): клиент Codex в проекте
# demo копии фикстур ищет «tracker» и должен найти факт Claude «Трекер
# задач». Мост не должен создать ни каталога данных, ни каталога настроек.
#
#   bash scripts/smoke-mcp.sh [путь к ai-memory-mcp]   (по умолчанию bin/ai-memory-mcp)
set -euo pipefail
cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.."
bin="$(realpath -- "${1:-bin/ai-memory-mcp}")"
[[ -x "$bin" ]] || { echo "Нет исполняемого $bin: сначала wails3 task build:mcp" >&2; exit 1; }

source scripts/fixture-env.sh
fixture_env smoke-mcp

requests=(
  '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"codex-mcp-client","version":"smoke"}}}'
  '{"jsonrpc":"2.0","method":"notifications/initialized"}'
  '{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"search_memory","arguments":{"query":"tracker"}}}'
)
# Пауза держит stdin открытым, пока мост отвечает: по EOF он завершается.
out="$(cd -- "$FX_HOME/projects/demo" && { printf '%s\n' "${requests[@]}"; sleep 3; } | "$bin" 2> "$FX_WORK/stderr.log")"

fail() {
  echo "ПРОВАЛ: $*" >&2
  echo "stderr моста: $FX_WORK/stderr.log" >&2
  exit 1
}
grep -q '"id":1,"result"' <<< "$out" || fail "нет ответа на initialize"
grep -q 'Трекер задач' <<< "$out" || fail "search_memory не нашёл факт Claude «Трекер задач»"
[[ ! -e "$AI_MEMORY_DATA_DIR" && ! -e "$AI_MEMORY_CONFIG_DIR" ]] || fail "мост создал каталог данных или настроек"

echo "OK: $bin, версия $("$bin" -version), факт Claude найден, на диск ничего не записано"
