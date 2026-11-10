// Package app — сервисы Wails: тонкий слой между ядром в internal/ и фронтендом.
//
// Только этот пакет и main.go знают о Wails. Методы сервисов становятся
// TypeScript-привязками во frontend/bindings (`wails3 task bindings`).
package app

import "github.com/yarkovaleksei/ai-memory/internal/config"

// AppInfo — сведения о приложении для экрана «О программе» и диагностики.
type AppInfo struct {
	Version        string   `json:"version"`
	ConfigDir      string   `json:"configDir"`
	DataDir        string   `json:"dataDir"`
	ClaudeProfiles []string `json:"claudeProfiles"`
	CodexHome      string   `json:"codexHome"`
}

// InfoService отдаёт версию и разрешённые каталоги. В фазе 0 он же доказывает,
// что мост Go ↔ TypeScript работает.
type InfoService struct {
	info AppInfo
}

// NewInfoService фиксирует сведения при запуске: каталоги за время работы не меняются.
func NewInfoService(version string, cfg config.Config) *InfoService {
	profiles := make([]string, len(cfg.ClaudeProfiles))
	copy(profiles, cfg.ClaudeProfiles)
	return &InfoService{info: AppInfo{
		Version:        version,
		ConfigDir:      cfg.ConfigDir,
		DataDir:        cfg.DataDir,
		ClaudeProfiles: profiles,
		CodexHome:      cfg.CodexHome,
	}}
}

// Info возвращает копию сведений, чтобы вызывающий не мог изменить состояние сервиса.
func (s *InfoService) Info() AppInfo {
	out := s.info
	out.ClaudeProfiles = append([]string(nil), s.info.ClaudeProfiles...)
	return out
}
