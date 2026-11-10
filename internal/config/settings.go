package config

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

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
	ExtraClaudeProfiles []string `toml:"extra_claude_profiles" json:"extraClaudeProfiles"`
	CodexHome           string   `toml:"codex_home,omitempty" json:"codexHome"`
	CodexGeneration     string   `toml:"codex_generation,omitempty" json:"codexGeneration"`
	ShowCodexInactive   bool     `toml:"show_codex_inactive" json:"showCodexInactive"`
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

// ErrGeneration — поколение Codex не «v1», не «v2» и не пустое (определять самому).
var ErrGeneration = errors.New("settings.generation-invalid")

// Clean убирает пустые и повторные профили и проверяет поколение. Пути
// остаются в том виде, как их ввёл пользователь (с «~»): файл переносим между
// машинами с разными домашними каталогами.
func (s Settings) Clean() (Settings, error) {
	out := s
	out.ExtraClaudeProfiles = nil
	seen := map[string]bool{}
	for _, p := range s.ExtraClaudeProfiles {
		p = strings.TrimSpace(p)
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		out.ExtraClaudeProfiles = append(out.ExtraClaudeProfiles, p)
	}
	out.CodexHome = strings.TrimSpace(s.CodexHome)
	switch s.CodexGeneration {
	case "", "v1", "v2":
	default:
		return s, ErrGeneration
	}
	return out, nil
}

// WriteSettings записывает config.toml атомарно: временный файл в том же
// каталоге и переименование. Это файл самого ai-memory, не память агентов.
func WriteSettings(configDir string, s Settings) error {
	dir := filepath.FromSlash(configDir)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("каталог настроек: %w", err)
	}
	var buf bytes.Buffer
	buf.WriteString("# Настройки ai-memory. Файл правит экран «Настройки»; можно и руками.\n")
	if err := toml.NewEncoder(&buf).Encode(s); err != nil {
		return fmt.Errorf("настройки: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".config-*.toml")
	if err != nil {
		return fmt.Errorf("настройки: %w", err)
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(buf.Bytes()); err != nil {
		tmp.Close()
		return fmt.Errorf("настройки: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("настройки: %w", err)
	}
	if err := os.Rename(tmp.Name(), filepath.Join(dir, SettingsFile)); err != nil {
		return fmt.Errorf("настройки: %w", err)
	}
	return nil
}

// Locked — какие настройки сейчас перекрыты переменными окружения ai-memory
// (тесты, `wails3 task dev`): файл их не меняет, и экран это показывает.
func Locked(env Env) (claudeProfiles, codexHome bool) {
	if env.Getenv == nil {
		return false, false
	}
	return env.Getenv(EnvClaudeDirs) != "", env.Getenv(EnvCodexHome) != ""
}
