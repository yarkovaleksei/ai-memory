package config

import (
	"errors"
	"os"
	"path/filepath"
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
				AgentsDir:      "/home/me/.agents",
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
				AgentsDir:      "/home/me/.agents",
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
				AgentsDir:      "/home/me/.agents",
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
				EnvAgentsDir:        "/fx/home/.agents/",
			}),
			want: Config{
				ConfigDir:      "/fx/cfg",
				DataDir:        "/fx/data",
				ClaudeProfiles: []string{"/fx/home/.claude", "/fx/home/.claude-work"},
				CodexHome:      "/fx/home/.codex",
				AgentsDir:      "/fx/home/.agents",
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
				AgentsDir:      "/Users/me/.agents",
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
				AgentsDir:      "/Users/me/.agents",
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
				AgentsDir:      `C:\Users\me\.agents`,
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
				AgentsDir:      `C:\Users\me\.agents`,
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

func TestApplySettings(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, SettingsFile), []byte(
		"extra_claude_profiles = [\"~/.claude-work\", \"/home/me/.claude\"]\ncodex_home = \"~/codex\"\nshow_codex_inactive = true\ncodex_generation = \"v2\"\n",
	), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := ReadSettings(dir)
	if err != nil {
		t.Fatalf("ReadSettings: %v", err)
	}

	env := envOf("linux", "/home/me", nil)
	cfg, err := Load(env)
	if err != nil {
		t.Fatal(err)
	}
	got := cfg.Apply(env, s)
	want := []string{"/home/me/.claude", "/home/me/.claude-work"}
	if !reflect.DeepEqual(got.ClaudeProfiles, want) {
		t.Errorf("ClaudeProfiles = %v, want %v", got.ClaudeProfiles, want)
	}
	if got.CodexHome != "/home/me/codex" || !got.ShowCodexInactive || got.CodexGeneration != "v2" {
		t.Errorf("CodexHome = %q, ShowCodexInactive = %v, CodexGeneration = %q", got.CodexHome, got.ShowCodexInactive, got.CodexGeneration)
	}

	// Переменные ai-memory важнее файла настроек.
	envFx := envOf("linux", "/home/me", map[string]string{EnvClaudeDirs: "/fx/.claude", EnvCodexHome: "/fx/.codex"})
	cfgFx, _ := Load(envFx)
	gotFx := cfgFx.Apply(envFx, s)
	if !reflect.DeepEqual(gotFx.ClaudeProfiles, []string{"/fx/.claude"}) || gotFx.CodexHome != "/fx/.codex" {
		t.Errorf("переменные окружения перекрыты файлом: %v %q", gotFx.ClaudeProfiles, gotFx.CodexHome)
	}
}

func TestReadSettingsMissingFile(t *testing.T) {
	s, err := ReadSettings(t.TempDir())
	if err != nil || len(s.ExtraClaudeProfiles) != 0 {
		t.Fatalf("ожидались пустые настройки без ошибки, получено %+v, %v", s, err)
	}
}

func TestSettingsRoundTrip(t *testing.T) {
	dir := t.TempDir()
	in := Settings{ExtraClaudeProfiles: []string{" ~/.claude-personal ", "", "~/.claude-personal", "/opt/claude"}, CodexGeneration: "v2"}
	clean, err := in.Clean()
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteSettings(dir, clean); err != nil {
		t.Fatal(err)
	}
	got, err := ReadSettings(dir)
	if err != nil || len(got.ExtraClaudeProfiles) != 2 || got.ExtraClaudeProfiles[0] != "~/.claude-personal" || got.CodexGeneration != "v2" {
		t.Errorf("прочитано: %+v %v", got, err)
	}
	if _, err := (Settings{CodexGeneration: "v3"}).Clean(); err != ErrGeneration {
		t.Errorf("поколение v3: %v", err)
	}
}
