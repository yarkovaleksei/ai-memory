// ai-memory запускает Wails-приложение со встроенным React-фронтендом.
//
// Здесь и в internal/app живёт весь код, который знает о Wails; ядро в internal/
// от него не зависит (см. plan/architecture.md).
package main

import (
	"context"
	"embed"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/yarkovaleksei/ai-memory/internal/app"
	"github.com/yarkovaleksei/ai-memory/internal/config"
	"github.com/yarkovaleksei/ai-memory/internal/library"
	"github.com/yarkovaleksei/ai-memory/internal/updates"
)

// version подменяется при сборке релиза через -ldflags "-X main.version=…".
var version = "0.0.1-dev"

// packageFormat — формат пакета, в котором поставляется этот бинарь (dmg,
// nsis); вшивается задачами упаковки (ADR-031). Пакеты Linux вместо этого
// ставят метку (updates.LinuxMarker). Пусто — обновление вручную.
var packageFormat = ""

// Собранный фронтенд встраивается в бинарь. Каталог появляется после
// `wails3 task common:build:frontend`, поэтому go vet и go test для этого
// пакета запускаются только после сборки фронтенда (см. Taskfile.yml).
//
//go:embed all:frontend/dist
var assets embed.FS

// icon — иконка окна и панели задач на Linux (GTK берёт её из Options.Icon).
// Тот же файл, из которого собираются .icns и .ico.
//
//go:embed build/appicon.png
var icon []byte

func main() {
	env := config.FromEnvironment()
	base, err := config.Load(env)
	if err != nil {
		log.Fatalf("не удалось определить каталоги ai-memory: %v", err)
	}
	settings, err := config.ReadSettings(base.ConfigDir)
	if err != nil {
		// Испорченный config.toml не должен мешать открыть память: работаем на умолчаниях.
		slog.Warn("настройки не прочитаны, используются умолчания", "error", err)
	}
	cfg := base.Apply(env, settings)
	rt := app.NewRuntime(cfg)

	lib, err := library.New(cfg, slog.Default())
	if err != nil {
		log.Fatalf("не удалось открыть индекс: %v", err)
	}

	exe, _ := os.Executable()
	if real, err := filepath.EvalSymlinks(exe); err == nil {
		exe = real
	}
	format := updates.DetectFormat(packageFormat, runtime.GOOS, exe, os.ReadFile)
	upd := app.NewUpdatesService(version, format, filepath.Join(filepath.FromSlash(cfg.DataDir), "updates"), slog.Default())

	plans, history := app.NewEditing(context.Background(), lib, rt, slog.Default())

	wailsApp := application.New(application.Options{
		Name:        "ai-memory",
		Description: "Просмотр и правка памяти Claude Code и Codex",
		Icon:        icon,
		Services: []application.Service{
			application.NewService(app.NewInfoService(version, rt)),
			application.NewService(app.NewLibraryService(lib, slog.Default())),
			application.NewService(plans),
			application.NewService(history),
			application.NewService(app.NewSettingsService(env, base, rt, lib, slog.Default())),
			application.NewService(app.NewLLMService(lib, rt, slog.Default())),
			application.NewService(upd),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})

	wailsApp.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:     "ai-memory",
		Width:     1280,
		Height:    800,
		MinWidth:  960,
		MinHeight: 600,
		URL:       "/",
	})

	if err := wailsApp.Run(); err != nil {
		log.Fatal(err)
	}
}
