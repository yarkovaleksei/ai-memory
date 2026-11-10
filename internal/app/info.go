// Package app — сервисы Wails: тонкий слой между ядром в internal/ и фронтендом.
//
// Только этот пакет и main.go знают о Wails. Методы сервисов становятся
// TypeScript-привязками во frontend/bindings (`wails3 task bindings`).
package app

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
	version string
	rt      *Runtime
}

// NewInfoService: каталоги берутся из Runtime при каждом вызове — их меняет
// экран «Настройки».
func NewInfoService(version string, rt *Runtime) *InfoService {
	return &InfoService{version: version, rt: rt}
}

// Info возвращает сведения по текущей конфигурации.
func (s *InfoService) Info() AppInfo {
	cfg := s.rt.Config()
	return AppInfo{
		Version:        s.version,
		ConfigDir:      cfg.ConfigDir,
		DataDir:        cfg.DataDir,
		ClaudeProfiles: cfg.ClaudeProfiles,
		CodexHome:      cfg.CodexHome,
	}
}
