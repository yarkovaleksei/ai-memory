# shellcheck shell=bash
# Общая часть проверок сборки: свежая копия fixtures/home в .task/<имя>/ и
# переменные окружения, которые направляют ai-memory на неё (AGENTS.md:
# проверки — только на фикстурах). Каталоги данных и настроек не создаются:
# по их отсутствию после проверки видно, что программа ничего не записала.
#
#   source scripts/fixture-env.sh <имя>   → $FX_WORK, $FX_HOME, переменные AI_MEMORY_*

fixture_env() {
  local name="${1:?имя каталога проверки}"
  FX_ROOT="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
  FX_WORK="$FX_ROOT/.task/$name"
  FX_HOME="$FX_WORK/home"
  rm -rf -- "$FX_WORK"
  mkdir -p -- "$FX_WORK"
  (cd -- "$FX_ROOT" && go run ./cmd/devhome fixtures/home "$FX_HOME" > /dev/null)
  export AI_MEMORY_CLAUDE_DIRS="$FX_HOME/.claude:$FX_HOME/.claude-work"
  export AI_MEMORY_CODEX_HOME="$FX_HOME/.codex"
  export AI_MEMORY_AGENTS_DIR="$FX_HOME/.agents"
  export AI_MEMORY_DATA_DIR="$FX_WORK/data"
  export AI_MEMORY_CONFIG_DIR="$FX_WORK/config"
}
