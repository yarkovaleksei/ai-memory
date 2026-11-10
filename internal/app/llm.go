package app

import (
	"context"
	"errors"
	"log/slog"
	"path/filepath"

	"github.com/yarkovaleksei/ai-memory/internal/library"
	"github.com/yarkovaleksei/ai-memory/internal/llm"
	"github.com/yarkovaleksei/ai-memory/internal/model"
)

// LLMService — черновики от модели через CLI агентов (ADR-005). Ничего не
// пишет: черновик объединения попадает в форму, и записывает его обычный план
// с diff; отчёт о противоречиях только показывается.
type LLMService struct {
	lib *library.Library
	rt  *Runtime
	log *slog.Logger
	// newClient — точка подмены в тестах: настоящий клиент запускает CLI.
	newClient func(workDir string) *llm.Client
}

// NewLLMService — сервис LLM-черновиков. Рабочий каталог вызовов — постоянный
// подкаталог данных ai-memory (Q8).
func NewLLMService(lib *library.Library, rt *Runtime, log *slog.Logger) *LLMService {
	if log == nil {
		log = slog.Default()
	}
	return &LLMService{lib: lib, rt: rt, log: log, newClient: func(dir string) *llm.Client {
		return &llm.Client{WorkDir: dir}
	}}
}

func (s *LLMService) client() *llm.Client {
	return s.newClient(filepath.Join(filepath.FromSlash(s.rt.Config().DataDir), "llm"))
}

// Agents — агенты, чьи CLI найдены: интерфейс показывает кнопки только для них.
func (s *LLMService) Agents() []model.Agent {
	return s.client().Available()
}

// MergeDraft — черновик объединения записей и противоречия между ними.
func (s *LLMService) MergeDraft(ctx context.Context, itemIDs []string, agent model.Agent) (llm.MergeDraft, error) {
	if len(itemIDs) < 2 {
		return llm.MergeDraft{}, fail("merge.too-few")
	}
	items, _, err := s.items(ctx, itemIDs)
	if err != nil {
		return llm.MergeDraft{}, err
	}
	// Инструкции отмечаются только для проверки противоречий (ADR-024):
	// объединение их не уберёт.
	for _, it := range items {
		if !it.Kind.Memory() {
			return llm.MergeDraft{}, fail("plan.not-memory", "title", it.Title)
		}
	}
	d, err := s.client().Merge(ctx, agent, items)
	if err != nil {
		return llm.MergeDraft{}, s.error(err)
	}
	return d, nil
}

// Contradiction — противоречие с записями целиком: интерфейс показывает их
// рядом без дополнительных запросов.
type Contradiction struct {
	Summary string       `json:"summary"`
	Kind    string       `json:"kind"` // contradiction | duplicate
	Items   []model.Item `json:"items"`
}

// Contradictions ищет противоречия среди записей, которые пользователь
// отметил в списке: уходят только они.
func (s *LLMService) Contradictions(ctx context.Context, itemIDs []string, agent model.Agent) ([]Contradiction, error) {
	if len(itemIDs) < 2 {
		return []Contradiction{}, fail("merge.too-few")
	}
	items, byID, err := s.items(ctx, itemIDs)
	if err != nil {
		return []Contradiction{}, err
	}
	found, err := s.client().Contradictions(ctx, agent, items)
	if err != nil {
		return []Contradiction{}, s.error(err)
	}
	out := make([]Contradiction, 0, len(found))
	for _, c := range found {
		v := Contradiction{Summary: c.Summary, Kind: c.Kind}
		for _, id := range c.ItemIDs {
			v.Items = append(v.Items, byID[id])
		}
		out = append(out, v)
	}
	return out, nil
}

// items читает записи целиком по ID в порядке запроса.
func (s *LLMService) items(ctx context.Context, ids []string) ([]model.Item, map[string]model.Item, error) {
	out := make([]model.Item, 0, len(ids))
	byID := make(map[string]model.Item, len(ids))
	for _, id := range ids {
		it, err := s.lib.Get(ctx, id)
		if err != nil {
			return nil, nil, toError(err, s.log)
		}
		out = append(out, it)
		byID[id] = it
	}
	return out, byID, nil
}

// error переводит отказ llm в код для интерфейса; подробности — в журнал.
func (s *LLMService) error(err error) error {
	var le *llm.Error
	if errors.As(err, &le) {
		if le.Code != llm.ErrCanceled {
			s.log.Warn("LLM-запрос не удался", "code", le.Code, "params", le.Params)
		}
		return &Error{Code: le.Code, Params: le.Params}
	}
	return toError(err, s.log)
}
