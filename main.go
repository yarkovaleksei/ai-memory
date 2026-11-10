// ai-memory запускает Wails-приложение со встроенным React-фронтендом.
//
// Здесь и в internal/app живёт весь код, который знает о Wails; ядро в internal/
// от него не зависит (см. plan/architecture.md).
package main

import (
	"embed"
	"log"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/yarkovaleksei/ai-memory/internal/app"
	"github.com/yarkovaleksei/ai-memory/internal/config"
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
	cfg, err := config.Load(config.FromEnvironment())
	if err != nil {
		log.Fatalf("не удалось определить каталоги ai-memory: %v", err)
	}

	wailsApp := application.New(application.Options{
		Name:        "ai-memory",
		Description: "Просмотр и правка памяти Claude Code и Codex",
		Services: []application.Service{
			application.NewService(app.NewInfoService(version, cfg)),
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
