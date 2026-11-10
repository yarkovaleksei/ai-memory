package app

import (
	"context"
	"strconv"

	"github.com/yarkovaleksei/ai-memory/internal/adapter/claude"
	"github.com/yarkovaleksei/ai-memory/internal/adapter/codex"
	"github.com/yarkovaleksei/ai-memory/internal/model"
	"github.com/yarkovaleksei/ai-memory/internal/writer"
)

// NoteText — заголовок и текст заметки, которые собирает интерфейс по словарю.
type NoteText struct {
	ItemID string `json:"itemId"`
	Title  string `json:"title"`
	Body   string `json:"body"`
}

// MergeRequest — объединение группы записей в одну.
type MergeRequest struct {
	ItemIDs []string    `json:"itemIds"`
	Target  model.Agent `json:"target"` // куда пишется итог: claude — факт, codex — заметка
	// Итог в Claude.
	SourceID  string           `json:"sourceId"`
	ProjectID string           `json:"projectId"`
	Fact      claude.FactInput `json:"fact"`
	// Итог в Codex.
	Title string `json:"title"`
	Body  string `json:"body"`
	// Заметки «забыть» для записей Codex группы: удалить их нельзя (ADR-002).
	Forget []NoteText `json:"forget"`
}

// Merge строит план объединения: одна новая запись, факты Claude группы
// удаляются, для записей Codex пишутся заметки «забыть». Всё одним планом и
// одним коммитом бэкапа, и откатывается одной операцией (ADR-014).
func (s *PlansService) Merge(ctx context.Context, req MergeRequest) (writer.Plan, error) {
	if len(req.ItemIDs) < 2 {
		return writer.Plan{}, fail("merge.too-few")
	}
	forget := map[string]NoteText{}
	for _, f := range req.Forget {
		forget[f.ItemID] = f
	}
	var facts []claude.FactRef
	var notes []codex.NoteSpec
	for _, id := range req.ItemIDs {
		it, err := s.e.item(ctx, id)
		if err != nil {
			return writer.Plan{}, err
		}
		switch {
		case it.Kind == model.KindClaudeFact && it.Write == model.WriteDirect:
			facts = append(facts, claude.FactRef{File: it.File, Hash: it.Hash})
		case it.Agent == model.AgentCodex && it.Write != model.WriteNone:
			f, ok := forget[id]
			if !ok {
				return writer.Plan{}, fail("codex.note.body-empty")
			}
			notes = append(notes, codex.NoteSpec{Title: f.Title, Body: f.Body, Forget: true})
		default:
			return writer.Plan{}, fail("plan.readonly", "title", it.Title)
		}
	}

	params := map[string]string{"count": strconv.Itoa(len(req.ItemIDs)), "target": string(req.Target)}
	var plans []writer.Plan
	g, _ := s.e.codexAdapter().Active(ctx)
	switch req.Target {
	case model.AgentClaude:
		memDir, err := s.memoryDir(ctx, req.SourceID, req.ProjectID)
		if err != nil {
			return writer.Plan{}, err
		}
		p, err := claude.PlanReplace(memDir, req.Fact, facts)
		if err != nil {
			return writer.Plan{}, s.e.error(err)
		}
		plans = append(plans, p)
		params["title"] = req.Fact.Name
	case model.AgentCodex:
		notes = append([]codex.NoteSpec{{Title: req.Title, Body: req.Body}}, notes...)
		if len(facts) > 0 {
			p, err := claude.PlanDeleteMany(facts)
			if err != nil {
				return writer.Plan{}, s.e.error(err)
			}
			plans = append(plans, p)
		}
		params["title"] = req.Title
	default:
		return writer.Plan{}, fail("merge.target-invalid")
	}
	if len(notes) > 0 {
		p, err := codex.PlanNotes(s.e.rt.Config().CodexHome, g, notes)
		if err != nil {
			return writer.Plan{}, s.e.error(err)
		}
		plans = append(plans, p)
	}
	return s.e.keep(writer.Combine(writer.Op{Code: "merge", Params: params}, plans...))
}
