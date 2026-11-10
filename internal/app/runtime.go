package app

import (
	"sync"

	"github.com/yarkovaleksei/ai-memory/internal/config"
)

// Runtime — текущая конфигурация, общая для сервисов. Экран «Настройки» меняет
// её на лету: корни памяти, корни записи и сведения «О программе» берутся
// отсюда при каждом обращении, а не фиксируются при запуске.
type Runtime struct {
	mu  sync.RWMutex
	cfg config.Config
}

// NewRuntime — конфигурация на момент запуска.
func NewRuntime(cfg config.Config) *Runtime {
	r := &Runtime{}
	r.set(cfg)
	return r
}

// Config возвращает копию текущей конфигурации.
func (r *Runtime) Config() config.Config {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := r.cfg
	out.ClaudeProfiles = append([]string(nil), r.cfg.ClaudeProfiles...)
	return out
}

// set копирует срезы: вызывающий может менять свой Config дальше.
func (r *Runtime) set(cfg config.Config) {
	cfg.ClaudeProfiles = append([]string(nil), cfg.ClaudeProfiles...)
	r.mu.Lock()
	r.cfg = cfg
	r.mu.Unlock()
}
