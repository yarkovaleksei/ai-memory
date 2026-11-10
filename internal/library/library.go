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
	"github.com/yarkovaleksei/ai-memory/internal/adapter/agentfiles"
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
	// files читает инструкции и навыки; пути проектов ему проставляет Rebuild
	// по прочитанной памяти (ADR-019).
	files agentfiles.Adapter
	ix    *index.Index
	log   *slog.Logger

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
	return &Library{adapters: adaptersFor(cfg), files: filesFor(cfg), ix: ix, log: log}, nil
}

func filesFor(cfg config.Config) agentfiles.Adapter {
	return agentfiles.Adapter{ClaudeProfiles: cfg.ClaudeProfiles, CodexHome: cfg.CodexHome, AgentsDir: cfg.AgentsDir}
}

func adaptersFor(cfg config.Config) []adapter.Adapter {
	return []adapter.Adapter{
		claude.Adapter{Profiles: cfg.ClaudeProfiles},
		codex.Adapter{Home: cfg.CodexHome, Generation: codex.Parse(cfg.CodexGeneration), ShowInactive: cfg.ShowCodexInactive},
	}
}

// Configure меняет корни памяти после правки настроек; подействует со
// следующей пересборки. Наблюдатель возьмёт новые каталоги после неё же.
func (l *Library) Configure(cfg config.Config) {
	l.rebuildMu.Lock()
	defer l.rebuildMu.Unlock()
	l.adapters = adaptersFor(cfg)
	l.files = filesFor(cfg)
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
	scan := func(ad adapter.Adapter) ([]model.Item, error) {
		srcs, err := ad.Discover(ctx)
		if err != nil {
			return nil, fmt.Errorf("library: поиск хранилищ: %w", err)
		}
		var out []model.Item
		for _, src := range srcs {
			got, err := ad.Scan(ctx, src)
			if err != nil {
				// Одно нечитаемое хранилище не должно прятать остальные.
				l.log.Warn("хранилище не прочитано", "source", src.ID, "error", err)
				src.Warnings = append(src.Warnings, model.NewWarning("source.scan-failed", "error", err.Error()))
			}
			sources = append(sources, src)
			out = append(out, got...)
			watched = append(watched, watchedSource{ad: ad, src: src})
		}
		return out, nil
	}
	for _, ad := range l.adapters {
		got, err := scan(ad)
		if err != nil {
			return Stats{}, err
		}
		items = append(items, got...)
	}
	// Проектные инструкции и навыки ищутся по путям проектов из памяти: других
	// источников путей у ai-memory нет.
	files := l.files
	files.Projects = projectPaths(items)
	got, err := scan(files)
	if err != nil {
		return Stats{}, err
	}
	agentfiles.MarkShared(got)
	items = append(items, got...)

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

// All — все записи целиком.
func (l *Library) All(ctx context.Context) ([]model.Item, error) { return l.ix.All(ctx) }

// ByFile — запись файла целиком (без раздела).
func (l *Library) ByFile(ctx context.Context, file string) (model.Item, error) {
	return l.ix.ByFile(ctx, file)
}

func (l *Library) GetMany(ctx context.Context, ids []string) ([]model.Item, error) {
	return l.ix.GetMany(ctx, ids)
}

func (l *Library) Projects(ctx context.Context) ([]model.Project, error) { return l.ix.Projects(ctx) }

func (l *Library) Sources(ctx context.Context) ([]model.Source, error) { return l.ix.Sources(ctx) }

// projectPaths — восстановленные пути проектов из памяти. Временные тоже:
// их записи и так скрыты из списка, пока не включён показ временных.
func projectPaths(items []model.Item) []string {
	seen := map[string]bool{}
	var out []string
	for _, it := range items {
		p := it.Project
		if p.Global || p.Path == "" || seen[p.Path] {
			continue
		}
		seen[p.Path] = true
		out = append(out, p.Path)
	}
	return out
}

// watchDirs — наблюдаемые каталоги и фильтр имён для каждого: nil — важно
// всё. Если каталог нужен двум адаптерам и одному важно всё, фильтра нет.
func (l *Library) watchDirs() map[string]map[string]bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := map[string]map[string]bool{}
	all := map[string]bool{}
	for _, w := range l.watched {
		var names map[string][]string
		if nf, ok := w.ad.(adapter.NameFilter); ok {
			names = nf.WatchNames(w.src)
		}
		for _, d := range w.ad.WatchDirs(w.src) {
			list, filtered := names[d]
			if !filtered {
				all[d] = true
				out[d] = nil
				continue
			}
			if all[d] {
				continue
			}
			if out[d] == nil {
				out[d] = map[string]bool{}
			}
			for _, n := range list {
				out[d][n] = true
			}
		}
	}
	return out
}
