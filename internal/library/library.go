// Package library собирает ядро вместе: адаптеры → каталог → индекс, плюс
// наблюдение за файлами. Это то, чем пользуются сервисы Wails и MCP-мост.
package library

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"sync"
	"time"

	"github.com/yarkovaleksei/ai-memory/internal/adapter"
	"github.com/yarkovaleksei/ai-memory/internal/adapter/claude"
	"github.com/yarkovaleksei/ai-memory/internal/adapter/codex"
	"github.com/yarkovaleksei/ai-memory/internal/catalog"
	"github.com/yarkovaleksei/ai-memory/internal/config"
	"github.com/yarkovaleksei/ai-memory/internal/index"
	"github.com/yarkovaleksei/ai-memory/internal/model"
)

// IndexFile — имя файла индекса в каталоге данных.
const IndexFile = "index.db"

// Stats — итог пересборки, уходит во фронтенд с событием «index:updated».
type Stats struct {
	Sources  int       `json:"sources"`
	Projects int       `json:"projects"`
	Items    int       `json:"items"`
	BuiltAt  time.Time `json:"builtAt"`
	Duration string    `json:"duration"`
}

// Library — точка входа ядра для чтения памяти.
type Library struct {
	adapters []adapter.Adapter
	ix       *index.Index
	log      *slog.Logger

	rebuildMu sync.Mutex // пересборки не пересекаются
	mu        sync.Mutex
	watched   []watchedSource // источники последней пересборки — для наблюдения
}

type watchedSource struct {
	ad  adapter.Adapter
	src model.Source
}

// New открывает индекс в каталоге данных и готовит адаптеры по конфигурации.
func New(cfg config.Config, log *slog.Logger) (*Library, error) {
	if log == nil {
		log = slog.Default()
	}
	ix, err := index.Open(filepath.Join(filepath.FromSlash(cfg.DataDir), IndexFile))
	if err != nil {
		return nil, err
	}
	return &Library{
		adapters: []adapter.Adapter{
			claude.Adapter{Profiles: cfg.ClaudeProfiles},
			codex.Adapter{Home: cfg.CodexHome, Generation: codex.Parse(cfg.CodexGeneration), ShowInactive: cfg.ShowCodexInactive},
		},
		ix:  ix,
		log: log,
	}, nil
}

// Close закрывает индекс.
func (l *Library) Close() error { return l.ix.Close() }

// Rebuild читает все хранилища заново и заменяет индекс целиком. Объём памяти —
// сотни файлов, полная пересборка занимает доли секунды и избавляет от
// рассинхронизации инкрементальных обновлений.
func (l *Library) Rebuild(ctx context.Context) (Stats, error) {
	l.rebuildMu.Lock()
	defer l.rebuildMu.Unlock()

	start := time.Now()
	var (
		sources []model.Source
		items   []model.Item
		watched []watchedSource
	)
	for _, ad := range l.adapters {
		srcs, err := ad.Discover(ctx)
		if err != nil {
			return Stats{}, fmt.Errorf("library: поиск хранилищ: %w", err)
		}
		for _, src := range srcs {
			got, err := ad.Scan(ctx, src)
			if err != nil {
				// Одно нечитаемое хранилище не должно прятать остальные.
				l.log.Warn("хранилище не прочитано", "source", src.ID, "error", err)
				src.Warnings = append(src.Warnings, model.NewWarning("source.scan-failed", "error", err.Error()))
			}
			sources = append(sources, src)
			items = append(items, got...)
			watched = append(watched, watchedSource{ad: ad, src: src})
		}
	}
	projects, items := catalog.Build(items)
	if err := l.ix.Replace(ctx, sources, projects, items); err != nil {
		return Stats{}, err
	}

	l.mu.Lock()
	l.watched = watched
	l.mu.Unlock()

	return Stats{
		Sources:  len(sources),
		Projects: len(projects),
		Items:    len(items),
		BuiltAt:  time.Now(),
		Duration: time.Since(start).Round(time.Millisecond).String(),
	}, nil
}

// Search, Get, GetMany, Projects, Sources — чтение из индекса.
func (l *Library) Search(ctx context.Context, q index.Query) (index.Page, error) {
	return l.ix.Search(ctx, q)
}

func (l *Library) Get(ctx context.Context, id string) (model.Item, error) { return l.ix.Get(ctx, id) }

func (l *Library) GetMany(ctx context.Context, ids []string) ([]model.Item, error) {
	return l.ix.GetMany(ctx, ids)
}

func (l *Library) Projects(ctx context.Context) ([]model.Project, error) { return l.ix.Projects(ctx) }

func (l *Library) Sources(ctx context.Context) ([]model.Source, error) { return l.ix.Sources(ctx) }

func (l *Library) watchDirs() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	seen := map[string]bool{}
	var out []string
	for _, w := range l.watched {
		for _, d := range w.ad.WatchDirs(w.src) {
			if !seen[d] {
				seen[d] = true
				out = append(out, d)
			}
		}
	}
	return out
}
