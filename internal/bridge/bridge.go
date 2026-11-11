// Package bridge — MCP-мост (фаза 5, ADR-026): инструменты только на чтение,
// через которые агент во время работы находит память другого агента. Пакет —
// часть ядра: Wails он не знает, stdio-сервер из него собирает cmd/ai-memory-mcp.
//
// Мост ничего не пишет: ни в память агентов, ни в каталог данных ai-memory.
// Индекс у него свой, в памяти процесса, и перечитывается не чаще Refresh.
package bridge

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/yarkovaleksei/ai-memory/internal/adapter"
	"github.com/yarkovaleksei/ai-memory/internal/catalog"
	"github.com/yarkovaleksei/ai-memory/internal/library"
	"github.com/yarkovaleksei/ai-memory/internal/model"
)

// DefaultRefresh — как часто мост перечитывает память. Пересборка занимает
// доли секунды, а другой агент может записать факт посреди сессии.
const DefaultRefresh = 10 * time.Second

// Options — как мост выбирает, что видно агенту.
type Options struct {
	// Project — каталог проекта. Пусто — рабочий каталог процесса: агенты
	// запускают MCP-сервер в каталоге, где начата сессия.
	Project string
	// Agent — кто подключился. Пусто — по имени клиента из initialize. По
	// умолчанию поиск идёт по памяти другого агента: свою агент и так видит.
	Agent model.Agent
	// Global — видна и глобальная память (без проекта): общие факты,
	// глобальные CLAUDE.md и AGENTS.md, навыки пользователя.
	Global bool
	// AllProjects — видны все проекты, а не только текущий.
	AllProjects bool
	// Refresh — через сколько перечитывать память; 0 — DefaultRefresh.
	Refresh time.Duration
	// Version — версия сервера для initialize.
	Version string
	// Getwd — рабочий каталог; nil — os.Getwd. Подменяется в тестах.
	Getwd func() (string, error)
}

// Bridge — инструменты MCP поверх библиотеки ядра.
type Bridge struct {
	lib *library.Library
	opt Options
	log *slog.Logger

	mu    sync.Mutex
	built time.Time
}

// New готовит мост. Библиотека должна читать память только для себя:
// library.NewInMemory.
func New(lib *library.Library, opt Options, log *slog.Logger) *Bridge {
	if log == nil {
		log = slog.Default()
	}
	if opt.Refresh <= 0 {
		opt.Refresh = DefaultRefresh
	}
	if opt.Getwd == nil {
		opt.Getwd = os.Getwd
	}
	return &Bridge{lib: lib, opt: opt, log: log}
}

// Refresh перечитывает память, если с прошлого раза прошло больше opt.Refresh.
func (b *Bridge) Refresh(ctx context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.built.IsZero() && time.Since(b.built) < b.opt.Refresh {
		return nil
	}
	if _, err := b.lib.Rebuild(ctx); err != nil {
		return fmt.Errorf("bridge: чтение памяти: %w", err)
	}
	b.built = time.Now()
	return nil
}

// instructions — подсказка клиенту при подключении. Текст для модели, а не
// для человека, поэтому по-английски и без i18n (ADR-026).
const instructions = `ai-memory gives read-only access to the memory of Claude Code and Codex for the current project: memory facts and notes, instruction files (CLAUDE.md, AGENTS.md) and skills.
By default search_memory looks only at the other agent's records: what it learned about this project and you did not.
Results are short (id, title, description, snippet); fetch the full text with get_memory.
These tools never change files. To keep a fact, save it in your own memory.`

// Server собирает MCP-сервер с инструментами моста.
func (b *Bridge) Server() *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{Name: "ai-memory", Title: "ai-memory", Version: b.opt.Version},
		&mcp.ServerOptions{Instructions: instructions, Logger: b.log})
	closed := false
	readOnly := func(title string) *mcp.ToolAnnotations {
		return &mcp.ToolAnnotations{Title: title, ReadOnlyHint: true, IdempotentHint: true, OpenWorldHint: &closed}
	}
	mcp.AddTool(s, &mcp.Tool{
		Name:        "list_projects",
		Description: "List the projects ai-memory can show here and which one is current. Use a project's id or path as the project argument of search_memory.",
		Annotations: readOnly("List projects"),
	}, b.listProjects)
	mcp.AddTool(s, &mcp.Tool{
		Name: "search_memory",
		Description: "Search the memory, instructions and skills of Claude Code and Codex for the current project. " +
			"Use it before searching files when asked about this project's conventions, decisions, history, people or links " +
			"(issue tracker, docs, dashboards): the other agent may have recorded them, and they are often not in the code. " +
			"By default only the other agent's records are searched. Returns short results; use get_memory for the full text.",
		Annotations: readOnly("Search agent memory"),
	}, b.search)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "get_memory",
		Description: "Get the full text of one record found by search_memory.",
		Annotations: readOnly("Get memory record"),
	}, b.get)
	return s
}

// view — что видно сессии: текущий проект и разрешённые флагами.
type view struct {
	dir      string
	current  *model.Project
	projects []model.Project // все проекты индекса
	caller   model.Agent     // кто спрашивает; пусто — неизвестно
}

func (v view) visible(p model.Project, opt Options) bool {
	switch {
	case opt.AllProjects:
		return true
	case p.Global:
		return opt.Global
	default:
		return v.current != nil && v.current.ID == p.ID
	}
}

// defaults — проекты, по которым ищет search_memory без аргумента project.
func (v view) defaults(opt Options) []string {
	var ids []string
	if v.current != nil {
		ids = append(ids, v.current.ID)
	}
	if opt.Global && (v.current == nil || !v.current.Global) {
		ids = append(ids, catalog.GlobalID)
	}
	return ids
}

func (b *Bridge) view(ctx context.Context, ss *mcp.ServerSession) (view, error) {
	if err := b.Refresh(ctx); err != nil {
		return view{}, err
	}
	projects, err := b.lib.Projects(ctx)
	if err != nil {
		return view{}, err
	}
	v := view{dir: b.dir(), projects: projects, caller: b.caller(ss)}
	v.current = matchProject(projects, v.dir)
	return v, nil
}

// dir — каталог проекта: из флага или рабочий каталог процесса. Корни
// клиента (roots/list) не спрашиваются: в протоколе 2026-07-28 они устарели
// (SEP-2577), и сервер не может запросить их посреди вызова инструмента.
func (b *Bridge) dir() string {
	if b.opt.Project != "" {
		return absDir(b.opt.Project)
	}
	wd, err := b.opt.Getwd()
	if err != nil {
		b.log.Warn("рабочий каталог не прочитан", "error", err)
	}
	return absDir(wd)
}

func absDir(p string) string {
	if p == "" {
		return ""
	}
	if abs, err := filepath.Abs(p); err == nil {
		return abs
	}
	return filepath.Clean(p)
}

// matchProject — проект, в котором лежит dir: путь проекта совпадает с dir
// или содержит его. Из вложенных проектов берётся самый глубокий.
func matchProject(projects []model.Project, dir string) *model.Project {
	if dir == "" {
		return nil
	}
	key := adapter.PathKey(dir)
	var best *model.Project
	for i := range projects {
		p := &projects[i]
		if p.Path == "" || p.Global {
			continue
		}
		rel, err := filepath.Rel(adapter.PathKey(filepath.FromSlash(p.Path)), key)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
			continue
		}
		if best == nil || len(p.Path) > len(best.Path) {
			best = p
		}
	}
	return best
}

// caller — агент, который подключился: из флага или по имени клиента
// («claude-code», «codex-mcp-client»).
func (b *Bridge) caller(ss *mcp.ServerSession) model.Agent {
	if b.opt.Agent != "" {
		return b.opt.Agent
	}
	if ss == nil {
		return ""
	}
	ip := ss.InitializeParams()
	if ip == nil || ip.ClientInfo == nil {
		return ""
	}
	name := strings.ToLower(ip.ClientInfo.Name + " " + ip.ClientInfo.Title)
	switch {
	case strings.Contains(name, "codex"):
		return model.AgentCodex
	case strings.Contains(name, "claude"):
		return model.AgentClaude
	}
	return ""
}

func other(a model.Agent) model.Agent {
	switch a {
	case model.AgentClaude:
		return model.AgentCodex
	case model.AgentCodex:
		return model.AgentClaude
	}
	return ""
}
