package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/yarkovaleksei/ai-memory/internal/config"
	"github.com/yarkovaleksei/ai-memory/internal/library"
	"github.com/yarkovaleksei/ai-memory/internal/model"
)

// Экран настроек на копии фикстур: второй профиль Claude добавляется в
// config.toml, и его память видна без перезапуска.
func TestSettingsSaveAppliesWithoutRestart(t *testing.T) {
	src, _ := filepath.Abs(filepath.Join("..", "..", "fixtures", "home"))
	home := filepath.Join(t.TempDir(), "home")
	copyDir(t, src, home)
	// Второй профиль — копия первого под другим именем.
	copyDir(t, filepath.Join(home, ".claude"), filepath.Join(home, ".claude-personal"))
	env := config.Env{GOOS: "linux", Home: home, ListSep: ":", Getenv: func(k string) string {
		if k == config.EnvConfigDir {
			return filepath.Join(home, "cfg")
		}
		if k == config.EnvDataDir {
			return filepath.Join(home, "data")
		}
		return ""
	}}
	base, err := config.Load(env)
	if err != nil {
		t.Fatal(err)
	}
	rt := NewRuntime(base)
	lib, err := library.New(base, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = lib.Close() })
	ctx := context.Background()
	if _, err := lib.Rebuild(ctx); err != nil {
		t.Fatal(err)
	}
	s := NewSettingsService(env, base, rt, lib, nil)
	rec := &recorder{}
	s.emit = rec.emit

	claudeSources := func() int {
		srcs, _ := lib.Sources(ctx)
		n := 0
		for _, src := range srcs {
			if src.Agent == model.AgentClaude {
				n++
			}
		}
		return n
	}
	if claudeSources() != 1 {
		t.Fatalf("до настройки профилей Claude: %d", claudeSources())
	}

	v, err := s.Save(ctx, config.Settings{ExtraClaudeProfiles: []string{"~/.claude-personal"}, CodexGeneration: "v2"})
	if err != nil {
		t.Fatal(err)
	}
	if len(v.ClaudeProfiles) != 2 || v.Settings.ExtraClaudeProfiles[0] != "~/.claude-personal" || v.CodexGeneration != "v2" {
		t.Errorf("после сохранения: %+v", v)
	}
	if claudeSources() != 2 || len(rec.events) == 0 {
		t.Errorf("второй профиль не подхвачен: источников %d, событий %d", claudeSources(), len(rec.events))
	}
	if _, err := os.Stat(filepath.Join(home, "cfg", config.SettingsFile)); err != nil {
		t.Errorf("config.toml не записан: %v", err)
	}
	if got := rt.Config().ClaudeProfiles; len(got) != 2 {
		t.Errorf("корни записи не обновлены: %v", got)
	}
	if _, err := s.Save(ctx, config.Settings{CodexGeneration: "v3"}); errCode(err) != "settings.generation-invalid" {
		t.Errorf("поколение v3: %v", err)
	}

	s.pick = func(string, string) (string, error) { return "/picked", nil }
	if dir, err := s.PickDirectory("t"); err != nil || dir != "/picked" {
		t.Errorf("PickDirectory: %q %v", dir, err)
	}
}
