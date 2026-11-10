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

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/yarkovaleksei/ai-memory/internal/app"
	"github.com/yarkovaleksei/ai-memory/internal/config"
	"github.com/yarkovaleksei/ai-memory/internal/library"
)

// version подменяется при сборке релиза через -ldflags "-X main.version=…".
var version = "0.0.1-dev"

// Собранный фронтенд встраивается в бинарь. Каталог появляется после
// `wails3 task common:build:frontend`, поэтому go vet и go test для этого
// пакета запускаются только после сборки фронтенда (см. Taskfile.yml).
//
//go:embed all:frontend/dist
var assets embed.FS

func main() {
	env := config.FromEnvironment()
	cfg, err := config.Load(env)
	if err != nil {
		log.Fatalf("не удалось определить каталоги ai-memory: %v", err)
	}
	settings, err := config.ReadSettings(cfg.ConfigDir)
	if err != nil {
		// Испорченный config.toml не должен мешать открыть память: работаем на умолчаниях.
		slog.Warn("настройки не прочитаны, используются умолчания", "error", err)
	}
	cfg = cfg.Apply(env, settings)

	lib, err := library.New(cfg, slog.Default())
	if err != nil {
		log.Fatalf("не удалось открыть индекс: %v", err)
	}

	plans, history := app.NewEditing(context.Background(), lib, cfg, slog.Default())

	wailsApp := application.New(application.Options{
		Name:        "ai-memory",
		Description: "Просмотр и правка памяти Claude Code и Codex",
		Services: []application.Service{
			application.NewService(app.NewInfoService(version, cfg)),
			application.NewService(app.NewLibraryService(lib, slog.Default())),
			application.NewService(plans),
			application.NewService(history),
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
