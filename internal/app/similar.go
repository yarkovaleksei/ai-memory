package app

import (
	"context"

	"github.com/yarkovaleksei/ai-memory/internal/library"

	"github.com/yarkovaleksei/ai-memory/internal/model"
	"github.com/yarkovaleksei/ai-memory/internal/similar"
)

// SimilarQuery — фильтры экрана «Похожие».
type SimilarQuery struct {
	ProjectID  string  `json:"projectId"`
	CrossAgent bool    `json:"crossAgent"`
	MinScore   float64 `json:"minScore"` // 0 — порог по умолчанию
}

// SimilarGroup — группа кандидатов в дубли с записями целиком: интерфейс
// сравнивает их рядом без дополнительных запросов.
type SimilarGroup struct {
	Score float64        `json:"score"`
	Pairs []similar.Pair `json:"pairs"`
	Items []model.Item   `json:"items"`
}

// Similar ищет кандидатов в дубли без LLM (internal/similar).
func (s *LibraryService) Similar(ctx context.Context, q SimilarQuery) ([]SimilarGroup, error) {
	groups, err := similarGroups(ctx, s.lib, q)
	if err != nil {
		return []SimilarGroup{}, toError(err, s.log)
	}
	return groups, nil
}

// similarGroups — группы похожих записей с записями целиком. Ими же
// пользуется проверка противоречий: модель сравнивает только кандидатов.
func similarGroups(ctx context.Context, lib *library.Library, q SimilarQuery) ([]SimilarGroup, error) {
	items, err := lib.All(ctx)
	if err != nil {
		return nil, err
	}
	byID := make(map[string]model.Item, len(items))
	for _, it := range items {
		byID[it.ID] = it
	}
	min := q.MinScore
	if min <= 0 {
		min = similar.DefaultMinScore
	}
	groups := similar.Find(items, similar.Options{MinScore: min, CrossAgent: q.CrossAgent, ProjectID: q.ProjectID})
	out := make([]SimilarGroup, 0, len(groups))
	for _, g := range groups {
		sg := SimilarGroup{Score: g.Score, Pairs: g.Pairs, Items: make([]model.Item, 0, len(g.ItemIDs))}
		for _, id := range g.ItemIDs {
			sg.Items = append(sg.Items, byID[id])
		}
		out = append(out, sg)
	}
	return out, nil
}
