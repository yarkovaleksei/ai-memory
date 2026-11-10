package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestLoadPointsAtFixtures проверяет связку, на которой держатся все тесты адаптеров:
// переменные ai-memory направляют приложение в fixtures/home, и там действительно
// лежат оба профиля Claude и корень Codex.
func TestLoadPointsAtFixtures(t *testing.T) {
	home, err := filepath.Abs(filepath.Join("..", "..", "fixtures", "home"))
	if err != nil {
		t.Fatal(err)
	}
	env := FromEnvironment()
	vars := map[string]string{
		EnvClaudeDirs: strings.Join([]string{
			filepath.Join(home, ".claude"),
			filepath.Join(home, ".claude-work"),
		}, string(os.PathListSeparator)),
		EnvCodexHome: filepath.Join(home, ".codex"),
		EnvDataDir:   t.TempDir(),
	}
	env.Getenv = func(k string) string { return vars[k] }

	cfg, err := Load(env)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cfg.ClaudeProfiles) != 2 {
		t.Fatalf("ClaudeProfiles = %v, want 2", cfg.ClaudeProfiles)
	}
	for _, dir := range append(cfg.ClaudeProfiles, cfg.CodexHome) {
		if fi, err := os.Stat(filepath.FromSlash(dir)); err != nil || !fi.IsDir() {
			t.Errorf("%s: каталог фикстуры не найден (%v)", dir, err)
		}
	}
}
