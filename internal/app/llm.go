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
	items := make([]model.Item, 0, len(itemIDs))
	for _, id := range itemIDs {
		it, err := s.lib.Get(ctx, id)
		if err != nil {
			return llm.MergeDraft{}, toError(err, s.log)
		}
		items = append(items, it)
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
	Items   []model.Item `json:"items"`
}

// ContradictionReport — итог проверки. Checked < Groups — не все группы
// поместились в один запрос: стоит сузить выбор (проект, порог).
type ContradictionReport struct {
	Groups         int             `json:"groups"`
	Checked        int             `json:"checked"`
	Contradictions []Contradiction `json:"contradictions"`
}

// Contradictions ищет противоречия внутри групп похожих записей: кандидатов
// отбирает TF-IDF экрана «Похожие», модель только сравнивает их тексты. Так
// запрос один и небольшой, а не «вся память против всей памяти».
func (s *LLMService) Contradictions(ctx context.Context, q SimilarQuery, agent model.Agent) (ContradictionReport, error) {
	groups, err := similarGroups(ctx, s.lib, q)
	if err != nil {
		return ContradictionReport{}, toError(err, s.log)
	}
	in := make([][]model.Item, len(groups))
	byID := map[string]model.Item{}
	for i, g := range groups {
		in[i] = g.Items
		for _, it := range g.Items {
			byID[it.ID] = it
		}
	}
	found, checked, err := s.client().Contradictions(ctx, agent, in)
	if err != nil {
		return ContradictionReport{}, s.error(err)
	}
	out := ContradictionReport{Groups: len(groups), Checked: checked, Contradictions: []Contradiction{}}
	for _, c := range found {
		v := Contradiction{Summary: c.Summary}
		for _, id := range c.ItemIDs {
			v.Items = append(v.Items, byID[id])
		}
		out.Contradictions = append(out.Contradictions, v)
	}
	return out, nil
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
