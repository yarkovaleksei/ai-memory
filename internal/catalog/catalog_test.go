package catalog

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/yarkovaleksei/ai-memory/internal/model"
)

func TestBuildMergesProjectsAcrossAgents(t *testing.T) {
	demo := filepath.FromSlash("/home/me/projects/demo")
	items := []model.Item{
		{ID: "1", Agent: model.AgentClaude, SourceID: "claude:a", Project: model.ProjectRef{Path: demo}},
		{ID: "2", Agent: model.AgentClaude, SourceID: "claude:b", Project: model.ProjectRef{Path: demo + string(filepath.Separator)}},
		{ID: "3", Agent: model.AgentCodex, SourceID: "codex:v2", Project: model.ProjectRef{Path: demo}},
		{ID: "4", Agent: model.AgentCodex, SourceID: "codex:v2", Project: model.ProjectRef{Global: true}},
		{ID: "5", Agent: model.AgentClaude, SourceID: "claude:a", Project: model.ProjectRef{Encoded: "-home-me-x"}},
		{ID: "6", Agent: model.AgentClaude, SourceID: "claude:a", Project: model.ProjectRef{Path: filepath.FromSlash("/tmp/s"), Temp: true}},
		{ID: "7", Agent: model.AgentCodex, SourceID: "codex:v2", Project: model.ProjectRef{Path: filepath.FromSlash("/home/me/projects/alpha")}},
	}
	projects, out := Build(items)

	var order []string
	for _, p := range projects {
		order = append(order, p.Label)
	}
	if want := []string{"", "alpha", "demo", "-home-me-x", "s"}; !slices.Equal(order, want) {
		t.Errorf("порядок проектов = %q, want %q", order, want)
	}

	demoProject := projects[2]
	if demoProject.ItemCount != 3 || !slices.Equal(demoProject.Agents, []model.Agent{model.AgentClaude, model.AgentCodex}) {
		t.Errorf("demo: %+v", demoProject)
	}
	if !slices.Equal(demoProject.SourceIDs, []string{"claude:a", "claude:b", "codex:v2"}) {
		t.Errorf("demo sources: %v", demoProject.SourceIDs)
	}
	if !projects[0].Global || projects[3].Resolved || !projects[4].Temp {
		t.Errorf("флаги проектов: %+v", projects)
	}
	for _, it := range out {
		if it.ProjectID == "" {
			t.Errorf("запись %s без ProjectID", it.ID)
		}
	}
	if out[0].ProjectID != out[2].ProjectID {
		t.Error("записи Claude и Codex одного пути попали в разные проекты")
	}
}
