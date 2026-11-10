package config

import (
	"errors"
	"reflect"
	"testing"
)

func envOf(goos, home string, vars map[string]string) Env {
	sep := ":"
	if goos == "windows" {
		sep = ";"
	}
	return Env{
		GOOS:    goos,
		Home:    home,
		Getenv:  func(k string) string { return vars[k] },
		ListSep: sep,
	}
}

func TestLoad(t *testing.T) {
	tests := []struct {
		name string
		env  Env
		want Config
	}{
		{
			name: "linux по умолчанию",
			env:  envOf("linux", "/home/me", nil),
			want: Config{
				ConfigDir:      "/home/me/.config/ai-memory",
				DataDir:        "/home/me/.local/share/ai-memory",
				ClaudeProfiles: []string{"/home/me/.claude"},
				CodexHome:      "/home/me/.codex",
			},
		},
		{
			name: "linux: XDG, CLAUDE_CONFIG_DIR и CODEX_HOME",
			env: envOf("linux", "/home/me", map[string]string{
				"XDG_CONFIG_HOME":   "/cfg",
				"XDG_DATA_HOME":     "/data/",
				"CLAUDE_CONFIG_DIR": "~/.claude-work",
				"CODEX_HOME":        "/opt/codex/",
			}),
			want: Config{
				ConfigDir:      "/cfg/ai-memory",
				DataDir:        "/data/ai-memory",
				ClaudeProfiles: []string{"/home/me/.claude", "/home/me/.claude-work"},
				CodexHome:      "/opt/codex",
			},
		},
		{
			name: "linux: относительный XDG по спецификации игнорируется",
			env:  envOf("linux", "/home/me", map[string]string{"XDG_DATA_HOME": "data"}),
			want: Config{
				ConfigDir:      "/home/me/.config/ai-memory",
				DataDir:        "/home/me/.local/share/ai-memory",
				ClaudeProfiles: []string{"/home/me/.claude"},
				CodexHome:      "/home/me/.codex",
			},
		},
		{
			name: "переменные ai-memory важнее всего остального",
			env: envOf("linux", "/home/me", map[string]string{
				EnvClaudeDirs:       "/fx/home/.claude::/fx/home/.claude-work:/fx/home/.claude",
				EnvCodexHome:        "/fx/home/.codex",
				"CODEX_HOME":        "/ignored",
				"CLAUDE_CONFIG_DIR": "/ignored-too",
				EnvConfigDir:        "/fx/cfg",
				EnvDataDir:          "/fx/data",
			}),
			want: Config{
				ConfigDir:      "/fx/cfg",
				DataDir:        "/fx/data",
				ClaudeProfiles: []string{"/fx/home/.claude", "/fx/home/.claude-work"},
				CodexHome:      "/fx/home/.codex",
			},
		},
		{
			name: "macOS",
			env:  envOf("darwin", "/Users/me", map[string]string{"XDG_CONFIG_HOME": "/ignored"}),
			want: Config{
				ConfigDir:      "/Users/me/Library/Application Support/ai-memory",
				DataDir:        "/Users/me/Library/Application Support/ai-memory",
				ClaudeProfiles: []string{"/Users/me/.claude"},
				CodexHome:      "/Users/me/.codex",
			},
		},
		{
			name: "macOS: профили без учёта регистра",
			env:  envOf("darwin", "/Users/me", map[string]string{"CLAUDE_CONFIG_DIR": "/Users/me/.Claude"}),
			want: Config{
				ConfigDir:      "/Users/me/Library/Application Support/ai-memory",
				DataDir:        "/Users/me/Library/Application Support/ai-memory",
				ClaudeProfiles: []string{"/Users/me/.claude"},
				CodexHome:      "/Users/me/.codex",
			},
		},
		{
			name: "Windows",
			env: envOf("windows", `C:\Users\me`, map[string]string{
				"AppData":      `C:\Users\me\AppData\Roaming`,
				"LocalAppData": `C:\Users\me\AppData\Local\`,
			}),
			want: Config{
				ConfigDir:      `C:\Users\me\AppData\Roaming\ai-memory`,
				DataDir:        `C:\Users\me\AppData\Local\ai-memory`,
				ClaudeProfiles: []string{`C:\Users\me\.claude`},
				CodexHome:      `C:\Users\me\.codex`,
			},
		},
		{
			name: "Windows без AppData и со слэшами в путях",
			env: envOf("windows", `C:\Users\me`, map[string]string{
				EnvClaudeDirs: `D:/work/.claude;C:\Users\me\.CLAUDE\..\.claude`,
				"CODEX_HOME":  "codex-home",
			}),
			want: Config{
				ConfigDir:      `C:\Users\me\AppData\Roaming\ai-memory`,
				DataDir:        `C:\Users\me\AppData\Local\ai-memory`,
				ClaudeProfiles: []string{`D:\work\.claude`, `C:\Users\me\.claude`},
				CodexHome:      `C:\Users\me\codex-home`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Load(tt.env)
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Load:\n got  %#v\n want %#v", got, tt.want)
			}
		})
	}
}

func TestLoadWithoutHome(t *testing.T) {
	_, err := Load(envOf("linux", "", nil))
	if !errors.Is(err, ErrNoHome) {
		t.Fatalf("ожидалась ErrNoHome, получено %v", err)
	}
}
