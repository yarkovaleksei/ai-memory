package bridge

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/yarkovaleksei/ai-memory/internal/adapter"
	"github.com/yarkovaleksei/ai-memory/internal/index"
	"github.com/yarkovaleksei/ai-memory/internal/model"
)

// Пределы выдачи: чужая память не должна раздувать контекст агента. Поиск
// отдаёт заголовок, описание и сниппет, тело — только get_memory.
const (
	defaultLimit   = 10
	maxLimit       = 50
	maxDescription = 300      // символов
	maxBody        = 64 << 10 // байт
)

// Виды записей в выдаче: грубее, чем model.ItemKind, — так агенту понятнее.
const (
	kindMemory       = "memory"
	kindInstructions = "instructions"
	kindSkill        = "skill"
	kindAll          = "all"
	agentAll         = "all"
	projectAll       = "all"
)

func kindOf(k model.ItemKind) string {
	switch k {
	case model.KindInstructions:
		return kindInstructions
	case model.KindSkill:
		return kindSkill
	}
	return kindMemory
}

// --- list_projects ---

// ListProjectsIn — у list_projects нет аргументов.
type ListProjectsIn struct{}

// Project — проект в выдаче.
type Project struct {
	ID      string        `json:"id"`
	Label   string        `json:"label"`
	Path    string        `json:"path,omitempty"`
	Global  bool          `json:"global,omitempty"`
	Current bool          `json:"current,omitempty"`
	Agents  []model.Agent `json:"agents"`
	Items   int           `json:"items"`
}

// ListProjectsOut — видимые проекты и каталог, по которому искался текущий.
type ListProjectsOut struct {
	Dir      string    `json:"dir" jsonschema:"directory used to find the current project"`
	Current  string    `json:"current,omitempty" jsonschema:"id of the current project; empty if the directory is not a known project"`
	Projects []Project `json:"projects"`
}

func (b *Bridge) listProjects(ctx context.Context, req *mcp.CallToolRequest, _ ListProjectsIn) (*mcp.CallToolResult, ListProjectsOut, error) {
	v, err := b.view(ctx, req.Session)
	if err != nil {
		return nil, ListProjectsOut{}, err
	}
	out := ListProjectsOut{Dir: v.dir, Projects: []Project{}}
	if v.current != nil {
		out.Current = v.current.ID
	}
	for _, p := range v.projects {
		if !v.visible(p, b.opt) {
			continue
		}
		out.Projects = append(out.Projects, Project{ID: p.ID, Label: p.Label, Path: p.Path, Global: p.Global,
			Current: v.current != nil && p.ID == v.current.ID, Agents: p.Agents, Items: p.ItemCount})
	}
	return nil, out, nil
}

// --- search_memory ---

// SearchIn — аргументы search_memory.
type SearchIn struct {
	Query   string `json:"query,omitempty" jsonschema:"words to search for (prefix match, all words must occur); empty lists the newest records"`
	Project string `json:"project,omitempty" jsonschema:"project id or path from list_projects; 'all' for every visible project; default: the current project"`
	Agent   string `json:"agent,omitempty" jsonschema:"whose records to search: claude, codex or all; default: the other agent"`
	Kind    string `json:"kind,omitempty" jsonschema:"memory, instructions, skill or all (default)"`
	Limit   int    `json:"limit,omitempty" jsonschema:"maximum number of results, 1-50; default 10"`
}

// Hit — найденная запись без тела.
type Hit struct {
	ID          string `json:"id"`
	Agent       string `json:"agent"`
	Kind        string `json:"kind" jsonschema:"memory, instructions or skill"`
	Format      string `json:"format" jsonschema:"storage format of the record, e.g. claude-fact or codex-rollout"`
	Type        string `json:"type,omitempty" jsonschema:"fact type if known: user, feedback, project or reference"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	Project     string `json:"project"`
	Snippet     string `json:"snippet,omitempty" jsonschema:"matching text, matches in **bold**"`
}

// SearchOut — результат поиска.
type SearchOut struct {
	Agent    string   `json:"agent" jsonschema:"whose records were searched"`
	Projects []string `json:"projects" jsonschema:"ids of the searched projects"`
	Total    int      `json:"total" jsonschema:"number of matching records, may exceed the results returned"`
	Results  []Hit    `json:"results"`
}

func (b *Bridge) search(ctx context.Context, req *mcp.CallToolRequest, in SearchIn) (*mcp.CallToolResult, SearchOut, error) {
	v, err := b.view(ctx, req.Session)
	if err != nil {
		return nil, SearchOut{}, err
	}
	q := index.Query{Text: in.Query, Limit: in.Limit, IncludeTemp: true}
	if q.Limit <= 0 {
		q.Limit = defaultLimit
	}
	q.Limit = min(q.Limit, maxLimit)

	agent := strings.ToLower(strings.TrimSpace(in.Agent))
	switch agent {
	case "":
		agent = string(other(v.caller))
		if agent == "" {
			agent = agentAll
		}
	case string(model.AgentClaude), string(model.AgentCodex), agentAll:
	default:
		return nil, SearchOut{}, fmt.Errorf("unknown agent %q: use claude, codex or all", in.Agent)
	}
	if agent != agentAll {
		q.Agent = agent
	}

	switch kind := strings.ToLower(strings.TrimSpace(in.Kind)); kind {
	case "", kindAll:
	case kindMemory:
		q.Kind = index.KindMemory
	case kindInstructions:
		q.Kind = string(model.KindInstructions)
	case kindSkill:
		q.Kind = string(model.KindSkill)
	default:
		return nil, SearchOut{}, fmt.Errorf("unknown kind %q: use memory, instructions, skill or all", in.Kind)
	}

	ids, err := b.projectIDs(v, in.Project)
	if err != nil {
		return nil, SearchOut{}, err
	}
	q.ProjectIDs = ids

	page, err := b.lib.Search(ctx, q)
	if err != nil {
		return nil, SearchOut{}, err
	}
	labels := v.labels()
	out := SearchOut{Agent: agent, Projects: ids, Total: page.Total, Results: []Hit{}}
	if out.Projects == nil {
		out.Projects = []string{projectAll}
	}
	for _, h := range page.Hits {
		out.Results = append(out.Results, Hit{ID: h.ID, Agent: string(h.Agent), Kind: kindOf(h.Kind), Format: string(h.Kind),
			Type: h.Type, Title: h.Title, Description: clip(h.Description, maxDescription),
			Project: labels[h.ProjectID], Snippet: marks.Replace(h.Snippet)})
	}
	return nil, out, nil
}

var marks = strings.NewReplacer(index.MarkStart, "**", index.MarkEnd, "**")

// projectIDs — по каким проектам искать. nil — по всем (только с AllProjects).
func (b *Bridge) projectIDs(v view, arg string) ([]string, error) {
	arg = strings.TrimSpace(arg)
	if arg == "" {
		ids := v.defaults(b.opt)
		if len(ids) == 0 {
			return nil, b.noProject(v)
		}
		return ids, nil
	}
	if arg == projectAll {
		if b.opt.AllProjects {
			return nil, nil
		}
		ids := v.defaults(b.opt)
		if len(ids) == 0 {
			return nil, b.noProject(v)
		}
		return ids, nil
	}
	for _, p := range v.projects {
		if p.ID == arg || (p.Path != "" && adapter.PathKey(p.Path) == adapter.PathKey(absDir(arg))) {
			if !v.visible(p, b.opt) {
				return nil, fmt.Errorf("project %q is not available: this ai-memory server shows %s", arg, b.scopeText())
			}
			return []string{p.ID}, nil
		}
	}
	return nil, fmt.Errorf("unknown project %q: see list_projects", arg)
}

func (b *Bridge) noProject(v view) error {
	return fmt.Errorf("%s is not a project known to ai-memory: neither agent has memory, instructions or skills for it yet; this server shows %s",
		v.dir, b.scopeText())
}

func (b *Bridge) scopeText() string {
	switch {
	case b.opt.AllProjects:
		return "all projects"
	case b.opt.Global:
		return "only the current project and global memory (start it with --all-projects to see others)"
	}
	return "only the current project (start it with --global for global memory, --all-projects for other projects)"
}

func (v view) labels() map[string]string {
	m := make(map[string]string, len(v.projects))
	for _, p := range v.projects {
		m[p.ID] = p.Label
	}
	return m
}

// --- get_memory ---

// GetIn — аргументы get_memory.
type GetIn struct {
	ID string `json:"id" jsonschema:"record id from search_memory"`
}

// Record — запись целиком.
type Record struct {
	Hit
	File      string `json:"file" jsonschema:"file the record lives in"`
	Section   string `json:"section,omitempty" jsonschema:"heading of the record inside the file, if the file holds several records"`
	Modified  string `json:"modified"`
	Pending   bool   `json:"pending,omitempty" jsonschema:"Codex has not consolidated this note yet"`
	Body      string `json:"body"`
	Truncated bool   `json:"truncated,omitempty" jsonschema:"body was cut to 64 KiB"`
}

func (b *Bridge) get(ctx context.Context, req *mcp.CallToolRequest, in GetIn) (*mcp.CallToolResult, Record, error) {
	v, err := b.view(ctx, req.Session)
	if err != nil {
		return nil, Record{}, err
	}
	notFound := fmt.Errorf("record %q not found: ids come from search_memory", in.ID)
	it, err := b.lib.Get(ctx, strings.TrimSpace(in.ID))
	if errors.Is(err, index.ErrNotFound) {
		return nil, Record{}, notFound
	}
	if err != nil {
		return nil, Record{}, err
	}
	var project *model.Project
	for i := range v.projects {
		if v.projects[i].ID == it.ProjectID {
			project = &v.projects[i]
		}
	}
	// Запись чужого проекта для этой сессии не существует: id не должен
	// открывать то, что не находит поиск.
	if project == nil || !v.visible(*project, b.opt) {
		return nil, Record{}, notFound
	}
	body := it.Body
	truncated := len(body) > maxBody
	if truncated {
		body = strings.ToValidUTF8(body[:maxBody], "")
	}
	return nil, Record{
		Hit: Hit{ID: it.ID, Agent: string(it.Agent), Kind: kindOf(it.Kind), Format: string(it.Kind), Type: it.Type,
			Title: it.Title, Description: it.Description, Project: project.Label},
		File: it.File, Section: it.Anchor, Modified: it.ModTime.UTC().Format(time.RFC3339), Pending: it.Pending,
		Body: body, Truncated: truncated,
	}, nil
}

func clip(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}
