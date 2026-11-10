package app

import (
	"context"
	"errors"
	"log/slog"
	"path/filepath"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/yarkovaleksei/ai-memory/internal/adapter/codex"
	"github.com/yarkovaleksei/ai-memory/internal/config"
	"github.com/yarkovaleksei/ai-memory/internal/library"
)

// SettingsView — что показывает экран «Настройки».
type SettingsView struct {
	Settings config.Settings `json:"settings"` // как в config.toml
	// Профили и корень Codex по умолчанию — без файла настроек.
	DefaultClaudeProfiles []string `json:"defaultClaudeProfiles"`
	DefaultCodexHome      string   `json:"defaultCodexHome"`
	// Что действует сейчас, после применения файла.
	ClaudeProfiles   []string `json:"claudeProfiles"`
	CodexHome        string   `json:"codexHome"`
	CodexGeneration  string   `json:"codexGeneration"`
	GenerationReason string   `json:"generationReason"`
	// Перекрыто переменными окружения AI_MEMORY_*: файл это не меняет.
	LockedClaudeProfiles bool   `json:"lockedClaudeProfiles"`
	LockedCodexHome      bool   `json:"lockedCodexHome"`
	ConfigFile           string `json:"configFile"`
}

// SettingsService читает и пишет config.toml ai-memory и применяет его без
// перезапуска. Память агентов он не трогает.
type SettingsService struct {
	env  config.Env
	base config.Config // конфигурация без файла настроек
	rt   *Runtime
	lib  *library.Library
	log  *slog.Logger
	emit func(name string, data any)
	// pick открывает системный выбор каталога; подменяется в тестах.
	pick func(title, start string) (string, error)
}

// NewSettingsService: base — config.Load без файла настроек, rt — текущая
// конфигурация, которую сервис будет менять.
func NewSettingsService(env config.Env, base config.Config, rt *Runtime, lib *library.Library, log *slog.Logger) *SettingsService {
	if log == nil {
		log = slog.Default()
	}
	return &SettingsService{env: env, base: base, rt: rt, lib: lib, log: log, emit: emitEvent, pick: pickDirectory}
}

// Get возвращает настройки из файла и то, что действует сейчас.
func (s *SettingsService) Get(ctx context.Context) (SettingsView, error) {
	st, err := config.ReadSettings(s.base.ConfigDir)
	if err != nil {
		// Испорченный файл всё равно можно переписать с экрана.
		s.log.Warn("config.toml не прочитан", "error", err)
	}
	cfg := s.rt.Config()
	g, reason := codex.Adapter{Home: cfg.CodexHome, Generation: codex.Parse(cfg.CodexGeneration)}.Active(ctx)
	lockedProfiles, lockedHome := config.Locked(s.env)
	return SettingsView{
		Settings:              withEmpty(st),
		DefaultClaudeProfiles: nonNil(append([]string(nil), s.base.ClaudeProfiles...)),
		DefaultCodexHome:      s.base.CodexHome,
		ClaudeProfiles:        nonNil(cfg.ClaudeProfiles),
		CodexHome:             cfg.CodexHome,
		CodexGeneration:       string(g),
		GenerationReason:      reason,
		LockedClaudeProfiles:  lockedProfiles,
		LockedCodexHome:       lockedHome,
		ConfigFile:            filepath.Join(filepath.FromSlash(s.base.ConfigDir), config.SettingsFile),
	}, nil
}

// Save пишет config.toml, применяет его и перечитывает память.
func (s *SettingsService) Save(ctx context.Context, st config.Settings) (SettingsView, error) {
	clean, err := st.Clean()
	if errors.Is(err, config.ErrGeneration) {
		return SettingsView{}, &Error{Code: config.ErrGeneration.Error()}
	}
	if err := config.WriteSettings(s.base.ConfigDir, clean); err != nil {
		s.log.Error("запись config.toml", "error", err)
		return SettingsView{}, &Error{Code: "settings.write-failed", Params: map[string]string{"error": err.Error()}}
	}
	cfg := s.base.Apply(s.env, clean)
	s.rt.set(cfg)
	s.lib.Configure(cfg)
	if st, err := s.lib.Rebuild(ctx); err != nil {
		s.log.Error("пересборка после настроек", "error", err)
	} else {
		s.emit(EventIndexUpdated, st)
	}
	return s.Get(ctx)
}

// PickDirectory открывает системный выбор каталога. Скрытые каталоги видны:
// профили агентов почти всегда скрытые (.claude, .codex). Отмена — пустая строка.
func (s *SettingsService) PickDirectory(title string) (string, error) {
	start := s.env.Home
	dir, err := s.pick(title, start)
	if err != nil {
		return "", &Error{Code: "settings.dialog-failed", Params: map[string]string{"error": err.Error()}}
	}
	return dir, nil
}

func pickDirectory(title, start string) (string, error) {
	a := application.Get()
	if a == nil {
		return "", errors.New("приложение не запущено")
	}
	return a.Dialog.OpenFile().
		CanChooseDirectories(true).
		CanChooseFiles(false).
		CanCreateDirectories(false).
		ShowHiddenFiles(true).
		SetDirectory(start).
		SetTitle(title).
		PromptForSingleSelection()
}

func withEmpty(s config.Settings) config.Settings {
	s.ExtraClaudeProfiles = nonNil(s.ExtraClaudeProfiles)
	return s
}
