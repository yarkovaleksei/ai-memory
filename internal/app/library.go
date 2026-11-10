package app

import (
	"context"
	"errors"
	"log/slog"
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/yarkovaleksei/ai-memory/internal/index"
	"github.com/yarkovaleksei/ai-memory/internal/library"
	"github.com/yarkovaleksei/ai-memory/internal/model"
)

// EventIndexUpdated — событие после каждой пересборки индекса; данные — library.Stats.
const EventIndexUpdated = "index:updated"

func init() {
	// Регистрация даёт типизированное событие в привязках (frontend/bindings/events).
	application.RegisterEvent[library.Stats](EventIndexUpdated)
}

// Error — ошибка для фронтенда: код и параметры, текст собирает словарь (ADR-008).
// Wails кладёт её JSON в поле cause ошибки вызова.
type Error struct {
	Code   string            `json:"code"`
	Params map[string]string `json:"params,omitempty"`
}

func (e *Error) Error() string { return e.Code }

// toError переводит ошибку ядра в код. Подробности неожиданных ошибок уходят
// в журнал и параметр detail, но интерфейс показывает их только как пояснение.
func toError(err error, log *slog.Logger) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, index.ErrNotFound) {
		return &Error{Code: index.ErrNotFound.Error()}
	}
	log.Error("ошибка чтения памяти", "error", err)
	return &Error{Code: "library.failed", Params: map[string]string{"detail": err.Error()}}
}

// ItemQuery — фильтры списка записей так, как их присылает dataProvider.
type ItemQuery struct {
	ProjectID    string `json:"projectId"`
	Agent        string `json:"agent"`
	Kind         string `json:"kind"`
	Type         string `json:"type"`
	Text         string `json:"q"`
	IncludeTemp  bool   `json:"includeTemp"`
	WarningsOnly bool   `json:"warningsOnly"`
	Sort         string `json:"sort"`
	Desc         bool   `json:"desc"`
	Offset       int    `json:"offset"`
	Limit        int    `json:"limit"`
}

// ListItem — запись в списке: без тела, со сниппетом совпадения.
type ListItem struct {
	model.Item
	Snippet string `json:"snippet"`
}

// ItemPage — страница списка и общее число записей под фильтром.
type ItemPage struct {
	Items []ListItem `json:"items"`
	Total int        `json:"total"`
}

// maxLimit ограничивает страницу: интерфейс не просит больше, а бесконечный
// список из-за ошибки во фронтенде не должен уронить окно.
const maxLimit = 500

// LibraryService — чтение памяти для интерфейса. Методы принимают только
// идентификаторы и фильтры, путей из фронтенда не берёт (AGENTS.md).
type LibraryService struct {
	lib  *library.Library
	log  *slog.Logger
	emit func(name string, data any)

	mu     sync.Mutex
	cancel context.CancelFunc
	done   chan struct{}
}

// NewLibraryService оборачивает библиотеку ядра. Закрывает её сервис при остановке.
func NewLibraryService(lib *library.Library, log *slog.Logger) *LibraryService {
	if log == nil {
		log = slog.Default()
	}
	return &LibraryService{
		lib: lib,
		log: log,
		emit: func(name string, data any) {
			if a := application.Get(); a != nil {
				a.Event.Emit(name, data)
			}
		},
	}
}

// ServiceStartup строит индекс и запускает наблюдение за файлами.
func (s *LibraryService) ServiceStartup(ctx context.Context, _ application.ServiceOptions) error {
	return s.start(ctx)
}

func (s *LibraryService) start(ctx context.Context) error {
	if _, err := s.lib.Rebuild(ctx); err != nil {
		// Окно всё равно откроется: пустой индекс лучше, чем отказ запуститься.
		s.log.Error("первичная сборка индекса", "error", err)
	}
	wctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	s.mu.Lock()
	s.cancel, s.done = cancel, done
	s.mu.Unlock()
	go func() {
		defer close(done)
		if err := s.lib.Watch(wctx, func(st library.Stats) { s.emit(EventIndexUpdated, st) }); err != nil {
			s.log.Error("наблюдение за файлами остановлено", "error", err)
		}
	}()
	return nil
}

// ServiceShutdown останавливает наблюдение и закрывает индекс.
func (s *LibraryService) ServiceShutdown() error {
	s.mu.Lock()
	cancel, done := s.cancel, s.done
	s.mu.Unlock()
	if cancel != nil {
		cancel()
		<-done
	}
	return s.lib.Close()
}

// Sources возвращает хранилища памяти.
func (s *LibraryService) Sources(ctx context.Context) ([]model.Source, error) {
	out, err := s.lib.Sources(ctx)
	return nonNil(out), toError(err, s.log)
}

// Projects возвращает проекты в порядке показа: глобальная память, проекты,
// нераспознанные, временные.
func (s *LibraryService) Projects(ctx context.Context) ([]model.Project, error) {
	out, err := s.lib.Projects(ctx)
	return nonNil(out), toError(err, s.log)
}

// ListItems возвращает страницу записей под фильтром.
func (s *LibraryService) ListItems(ctx context.Context, q ItemQuery) (ItemPage, error) {
	limit := q.Limit
	if limit <= 0 || limit > maxLimit {
		limit = maxLimit
	}
	page, err := s.lib.Search(ctx, index.Query{
		ProjectID:    q.ProjectID,
		Agent:        q.Agent,
		Kind:         q.Kind,
		Type:         q.Type,
		Text:         q.Text,
		IncludeTemp:  q.IncludeTemp,
		WarningsOnly: q.WarningsOnly,
		Sort:         q.Sort,
		Desc:         q.Desc,
		Offset:       q.Offset,
		Limit:        limit,
	})
	if err != nil {
		return ItemPage{Items: []ListItem{}}, toError(err, s.log)
	}
	items := make([]ListItem, 0, len(page.Hits))
	for _, h := range page.Hits {
		items = append(items, ListItem{Item: h.Item, Snippet: h.Snippet})
	}
	return ItemPage{Items: items, Total: page.Total}, nil
}

// GetItem возвращает запись целиком.
func (s *LibraryService) GetItem(ctx context.Context, id string) (model.Item, error) {
	it, err := s.lib.Get(ctx, id)
	return it, toError(err, s.log)
}

// GetItems возвращает найденные записи; отсутствующие пропускаются.
func (s *LibraryService) GetItems(ctx context.Context, ids []string) ([]model.Item, error) {
	out, err := s.lib.GetMany(ctx, ids)
	return nonNil(out), toError(err, s.log)
}

// Rebuild перечитывает память по запросу пользователя.
func (s *LibraryService) Rebuild(ctx context.Context) (library.Stats, error) {
	st, err := s.lib.Rebuild(ctx)
	if err != nil {
		return st, toError(err, s.log)
	}
	s.emit(EventIndexUpdated, st)
	return st, nil
}

// nonNil превращает nil-срез в пустой: во фронтенд уходит [], а не null.
func nonNil[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}
