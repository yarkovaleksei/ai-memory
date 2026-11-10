// Package config вычисляет, где лежит память агентов и где ai-memory хранит свои файлы.
//
// Это единственное место, которое знает про переменные окружения и правила
// каталогов каждой ОС (ADR-009). Остальной код получает готовый Config.
// Функции не читают окружение сами: всё приходит через Env, поэтому правила
// любой ОС проверяются тестами на любой машине.
package config

import (
	"errors"
	"fmt"
	"path"
	"strings"
)

// Переменные окружения ai-memory. Ими тесты и разработка направляют приложение
// на фикстуры вместо настоящей памяти.
const (
	// EnvClaudeDirs — список профилей Claude Code через разделитель списка путей ОС
	// (":" на Linux и macOS, ";" на Windows). Если задан, заменяет обнаружение по умолчанию.
	EnvClaudeDirs = "AI_MEMORY_CLAUDE_DIRS"
	// EnvCodexHome — корень Codex. Если задан, важнее CODEX_HOME.
	EnvCodexHome = "AI_MEMORY_CODEX_HOME"
	// EnvConfigDir и EnvDataDir переопределяют собственные каталоги ai-memory.
	EnvConfigDir = "AI_MEMORY_CONFIG_DIR"
	EnvDataDir   = "AI_MEMORY_DATA_DIR"
)

// Env — всё, что config берёт из среды выполнения.
type Env struct {
	GOOS    string                  // "linux", "darwin", "windows"
	Home    string                  // домашний каталог пользователя
	Getenv  func(key string) string // чтение переменной окружения
	ListSep string                  // разделитель списка путей ОС
}

// Config — разрешённые пути. Все пути абсолютные и нормализованы по правилам целевой ОС.
type Config struct {
	// ConfigDir — каталог настроек ai-memory (config.toml).
	ConfigDir string
	// DataDir — каталог данных ai-memory (index.db, backups/).
	DataDir string
	// ClaudeProfiles — каталоги профилей Claude Code, без повторов, в порядке приоритета.
	ClaudeProfiles []string
	// CodexHome — корень Codex.
	CodexHome string
}

// ErrNoHome возвращается, когда домашний каталог неизвестен и пути не из чего строить.
var ErrNoHome = errors.New("домашний каталог пользователя не определён")

// Load вычисляет Config по среде выполнения.
func Load(env Env) (Config, error) {
	if env.Home == "" {
		return Config{}, ErrNoHome
	}
	if env.Getenv == nil {
		env.Getenv = func(string) string { return "" }
	}

	configDir, err := ownDir(env, EnvConfigDir, defaultConfigDir)
	if err != nil {
		return Config{}, fmt.Errorf("каталог настроек: %w", err)
	}
	dataDir, err := ownDir(env, EnvDataDir, defaultDataDir)
	if err != nil {
		return Config{}, fmt.Errorf("каталог данных: %w", err)
	}

	return Config{
		ConfigDir:      configDir,
		DataDir:        dataDir,
		ClaudeProfiles: claudeProfiles(env),
		CodexHome:      codexHome(env),
	}, nil
}

func ownDir(env Env, override string, fallback func(Env) (string, error)) (string, error) {
	if v := env.Getenv(override); v != "" {
		return absClean(env, v)
	}
	return fallback(env)
}

// defaultConfigDir повторяет os.UserConfigDir, но для заданной ОС.
func defaultConfigDir(env Env) (string, error) {
	switch env.GOOS {
	case "windows":
		if v := env.Getenv("AppData"); v != "" {
			return join(env, v, "ai-memory"), nil
		}
		return join(env, env.Home, "AppData", "Roaming", "ai-memory"), nil
	case "darwin", "ios":
		return join(env, env.Home, "Library", "Application Support", "ai-memory"), nil
	default:
		if v := env.Getenv("XDG_CONFIG_HOME"); v != "" && isAbs(env, v) {
			return join(env, v, "ai-memory"), nil
		}
		return join(env, env.Home, ".config", "ai-memory"), nil
	}
}

// defaultDataDir: на Windows данные — в %LocalAppData% (не синхронизируются
// между машинами в роуминг-профиле), на macOS — рядом с настройками,
// на остальных — по XDG.
func defaultDataDir(env Env) (string, error) {
	switch env.GOOS {
	case "windows":
		if v := env.Getenv("LocalAppData"); v != "" {
			return join(env, v, "ai-memory"), nil
		}
		return join(env, env.Home, "AppData", "Local", "ai-memory"), nil
	case "darwin", "ios":
		return join(env, env.Home, "Library", "Application Support", "ai-memory"), nil
	default:
		if v := env.Getenv("XDG_DATA_HOME"); v != "" && isAbs(env, v) {
			return join(env, v, "ai-memory"), nil
		}
		return join(env, env.Home, ".local", "share", "ai-memory"), nil
	}
}

// claudeProfiles: явный список из EnvClaudeDirs, иначе ~/.claude и CLAUDE_CONFIG_DIR.
// Дополнительные профили из настроек приложения добавляются позже (фаза 1).
func claudeProfiles(env Env) []string {
	var raw []string
	if v := env.Getenv(EnvClaudeDirs); v != "" {
		raw = strings.Split(v, env.ListSep)
	} else {
		raw = []string{join(env, env.Home, ".claude")}
		if v := env.Getenv("CLAUDE_CONFIG_DIR"); v != "" {
			raw = append(raw, v)
		}
	}

	seen := make(map[string]bool, len(raw))
	out := make([]string, 0, len(raw))
	for _, p := range raw {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		abs, err := absClean(env, p)
		if err != nil {
			continue
		}
		key := abs
		if env.GOOS == "windows" || env.GOOS == "darwin" {
			// Файловые системы по умолчанию нечувствительны к регистру.
			key = strings.ToLower(abs)
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, abs)
	}
	return out
}

func codexHome(env Env) string {
	for _, key := range []string{EnvCodexHome, "CODEX_HOME"} {
		if v := env.Getenv(key); v != "" {
			if abs, err := absClean(env, v); err == nil {
				return abs
			}
		}
	}
	return join(env, env.Home, ".codex")
}

// Ниже — работа с путями целевой ОС, а не той, где идут тесты.

func sep(env Env) string {
	if env.GOOS == "windows" {
		return `\`
	}
	return "/"
}

func join(env Env, parts ...string) string {
	if env.GOOS == "windows" {
		for i, p := range parts {
			parts[i] = strings.ReplaceAll(p, "/", `\`)
		}
		return cleanWindows(strings.Join(parts, `\`))
	}
	return path.Clean(strings.Join(parts, "/"))
}

func isAbs(env Env, p string) bool {
	if env.GOOS == "windows" {
		p = strings.ReplaceAll(p, "/", `\`)
		return (len(p) >= 3 && p[1] == ':' && p[2] == '\\') || strings.HasPrefix(p, `\\`)
	}
	return strings.HasPrefix(p, "/")
}

// absClean приводит путь к абсолютному: относительные пути считаются от домашнего
// каталога, а не от текущего, чтобы результат не зависел от того, откуда запущено приложение.
func absClean(env Env, p string) (string, error) {
	if strings.HasPrefix(p, "~") && (len(p) == 1 || p[1] == '/' || p[1] == '\\') {
		p = env.Home + p[1:]
	}
	if !isAbs(env, p) {
		p = env.Home + sep(env) + p
	}
	if !isAbs(env, p) {
		return "", fmt.Errorf("путь %q не абсолютный", p)
	}
	return join(env, p), nil
}

// cleanWindows убирает повторные разделители и «.»/«..», не трогая префикс диска.
func cleanWindows(p string) string {
	prefix := ""
	switch {
	case strings.HasPrefix(p, `\\`):
		prefix, p = `\\`, p[2:]
	case len(p) >= 2 && p[1] == ':':
		prefix, p = p[:2]+`\`, strings.TrimLeft(p[2:], `\`)
	}
	var out []string
	for _, part := range strings.Split(p, `\`) {
		switch part {
		case "", ".":
		case "..":
			if len(out) > 0 {
				out = out[:len(out)-1]
			}
		default:
			out = append(out, part)
		}
	}
	return prefix + strings.Join(out, `\`)
}
