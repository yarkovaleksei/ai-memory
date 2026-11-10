package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// SettingsFile — имя файла настроек в каталоге настроек ai-memory.
const SettingsFile = "config.toml"

// Settings — то, что пользователь задаёт в config.toml. Всё необязательно.
//
//	extra_claude_profiles = ["~/.claude-work"]
//	codex_home = "/opt/codex"
//	codex_generation = "v2"
//	show_codex_inactive = true
type Settings struct {
	ExtraClaudeProfiles []string `toml:"extra_claude_profiles"`
	CodexHome           string   `toml:"codex_home"`
	CodexGeneration     string   `toml:"codex_generation"`
	ShowCodexInactive   bool     `toml:"show_codex_inactive"`
}

// ReadSettings читает config.toml из каталога настроек. Отсутствие файла — не ошибка.
func ReadSettings(configDir string) (Settings, error) {
	var s Settings
	path := filepath.Join(filepath.FromSlash(configDir), SettingsFile)
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return s, fmt.Errorf("чтение %s: %w", path, err)
	}
	if err := toml.Unmarshal(data, &s); err != nil {
		return s, fmt.Errorf("разбор %s: %w", path, err)
	}
	return s, nil
}

// Apply дополняет Config настройками. Переменные окружения ai-memory важнее файла:
// если корни заданы через AI_MEMORY_*, файл их не трогает — так тесты и
// `wails3 task dev` не зависят от настроек на машине разработчика.
func (c Config) Apply(env Env, s Settings) Config {
	if env.Getenv == nil {
		env.Getenv = func(string) string { return "" }
	}
	out := c
	out.ClaudeProfiles = append([]string(nil), c.ClaudeProfiles...)
	out.ShowCodexInactive = s.ShowCodexInactive
	if s.CodexGeneration == "v1" || s.CodexGeneration == "v2" {
		out.CodexGeneration = s.CodexGeneration
	}

	if env.Getenv(EnvClaudeDirs) == "" && len(s.ExtraClaudeProfiles) > 0 {
		all := append(append([]string(nil), c.ClaudeProfiles...), s.ExtraClaudeProfiles...)
		out.ClaudeProfiles = dedupeProfiles(env, all)
	}
	if env.Getenv(EnvCodexHome) == "" && s.CodexHome != "" {
		if abs, err := absClean(env, s.CodexHome); err == nil {
			out.CodexHome = abs
		}
	}
	return out
}
