package bridge

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/yarkovaleksei/ai-memory/internal/catalog"
	"github.com/yarkovaleksei/ai-memory/internal/config"
	"github.com/yarkovaleksei/ai-memory/internal/index"
	"github.com/yarkovaleksei/ai-memory/internal/library"
	"github.com/yarkovaleksei/ai-memory/internal/model"
)

// Протокольные тесты: настоящий MCP-клиент go-sdk говорит с мостом через
// транспорт в памяти, мост читает копию fixtures/home.

// fixtureHome копирует fixtures/home во временный каталог и переписывает
// выдуманный дом /home/me на путь копии: проект /home/me/projects/demo из
// памяти указывает на каталог копии, и видны его инструкции и навыки.
func fixtureHome(t *testing.T) string {
	t.Helper()
	src, err := filepath.Abs(filepath.Join("..", "..", "fixtures", "home"))
	if err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(t.TempDir(), "home")
	err = filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		to := dst + string(filepath.Separator)
		if strings.HasSuffix(p, ".jsonl") {
			to = strings.ReplaceAll(to, `\`, `\\`) // путь внутри JSON-строки
		}
		return os.WriteFile(target, []byte(strings.ReplaceAll(string(data), "/home/me/", to)), 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
	return dst
}

// snapshot — путь → права, sha256 и время изменения каждого файла дерева.
func snapshot(t *testing.T, roots ...string) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, root := range roots {
		err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			fi, err := d.Info()
			if err != nil {
				return err
			}
			state := fi.Mode().String()
			if !d.IsDir() {
				data, err := os.ReadFile(p)
				if err != nil {
					return err
				}
				sum := sha256.Sum256(data)
				state += " " + hex.EncodeToString(sum[:]) + " " + fi.ModTime().String()
			}
			out[p] = state
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	return out
}

type fixture struct {
	home, data string
	lib        *library.Library
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	home := fixtureHome(t)
	data := filepath.Join(t.TempDir(), "data")
	if err := os.MkdirAll(data, 0o700); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{
		DataDir: filepath.ToSlash(data),
		ClaudeProfiles: []string{
			filepath.ToSlash(filepath.Join(home, ".claude")),
			filepath.ToSlash(filepath.Join(home, ".claude-work")),
		},
		CodexHome: filepath.ToSlash(filepath.Join(home, ".codex")),
		AgentsDir: filepath.ToSlash(filepath.Join(home, ".agents")),
	}
	lib, err := library.NewInMemory(cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { lib.Close() })
	return fixture{home: home, data: data, lib: lib}
}

func (f fixture) demo() string { return filepath.Join(f.home, "projects", "demo") }

// connect поднимает мост и подключает к нему клиента с именем client. wd —
// рабочий каталог моста.
func (f fixture) connect(t *testing.T, opt Options, client, wd string) *mcp.ClientSession {
	t.Helper()
	ctx := context.Background()
	opt.Getwd = func() (string, error) { return wd, nil }
	srv := New(f.lib, opt, slog.New(slog.DiscardHandler)).Server()
	st, ct := mcp.NewInMemoryTransports()
	ss, err := srv.Connect(ctx, st, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ss.Close() })
	c := mcp.NewClient(&mcp.Implementation{Name: client, Version: "test"}, nil)
	cs, err := c.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cs.Close() })
	return cs
}

// call вызывает инструмент и разбирает structuredContent в out. Ошибка
// инструмента — текст из IsError-результата.
func call(t *testing.T, cs *mcp.ClientSession, name string, args map[string]any, out any) string {
	t.Helper()
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	if res.IsError {
		var texts []string
		for _, c := range res.Content {
			if tc, ok := c.(*mcp.TextContent); ok {
				texts = append(texts, tc.Text)
			}
		}
		return strings.Join(texts, "\n")
	}
	data, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, out); err != nil {
		t.Fatalf("%s: %v\n%s", name, err, data)
	}
	return ""
}

func search(t *testing.T, cs *mcp.ClientSession, args map[string]any) SearchOut {
	t.Helper()
	var out SearchOut
	if msg := call(t, cs, "search_memory", args, &out); msg != "" {
		t.Fatalf("search_memory %v: %s", args, msg)
	}
	return out
}

func TestToolsAreReadOnly(t *testing.T) {
	f := newFixture(t)
	cs := f.connect(t, Options{}, "test", f.demo())
	var names []string
	for tool, err := range cs.Tools(context.Background(), nil) {
		if err != nil {
			t.Fatal(err)
		}
		names = append(names, tool.Name)
		if tool.Annotations == nil || !tool.Annotations.ReadOnlyHint {
			t.Errorf("%s: нет readOnlyHint", tool.Name)
		}
	}
	if strings.Join(names, ",") != "get_memory,list_projects,search_memory" {
		t.Errorf("инструменты = %v", names)
	}
	if !strings.Contains(cs.InitializeResult().Instructions, "read-only") {
		t.Error("нет инструкций для клиента")
	}
}

// Критерий готовности фазы 5: Codex находит факт, который есть только в
// памяти Claude, и наоборот.
func TestCrossAgent(t *testing.T) {
	f := newFixture(t)
	tests := []struct {
		name, client, query string
		agent               model.Agent // чьи записи должны найтись
		format              model.ItemKind
		title               string
	}{
		{"Codex находит факт Claude", "codex-mcp-client", "tracker", model.AgentClaude, model.KindClaudeFact, "Трекер задач"},
		{"Claude находит сводку Codex", "claude-code", "тег", model.AgentCodex, model.KindCodexRollout, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cs := f.connect(t, Options{}, tt.client, f.demo())
			out := search(t, cs, map[string]any{"query": tt.query})
			if out.Agent != string(tt.agent) {
				t.Errorf("искали по %q, want %q", out.Agent, tt.agent)
			}
			var hit *Hit
			for i, h := range out.Results {
				if h.Agent != string(tt.agent) {
					t.Errorf("запись чужого агента %s в выдаче по умолчанию: %+v", h.Agent, h)
				}
				if h.Format == string(tt.format) && (tt.title == "" || h.Title == tt.title) {
					hit = &out.Results[i]
				}
			}
			if hit == nil {
				t.Fatalf("не найдено: %+v", out)
			}
			if hit.Kind != kindMemory || hit.Project != "demo" || !strings.Contains(hit.Snippet, "**") {
				t.Errorf("hit = %+v", hit)
			}

			var rec Record
			if msg := call(t, cs, "get_memory", map[string]any{"id": hit.ID}, &rec); msg != "" {
				t.Fatal(msg)
			}
			if rec.Body == "" || rec.File == "" || rec.ID != hit.ID || rec.Agent != string(tt.agent) {
				t.Errorf("get_memory = %+v", rec)
			}
		})
	}
}

func TestSearchArguments(t *testing.T) {
	f := newFixture(t)
	cs := f.connect(t, Options{}, "claude-code", f.demo())

	// agent=all — свои записи тоже: «тег» есть и в факте Claude release-flow.
	all := search(t, cs, map[string]any{"query": "тег", "agent": "all"})
	agents := map[string]bool{}
	for _, h := range all.Results {
		agents[h.Agent] = true
	}
	if !agents["claude"] || !agents["codex"] {
		t.Errorf("agent=all: %+v", all.Results)
	}

	// Инструкции и навыки ищутся с видом записи в выдаче.
	instr := search(t, cs, map[string]any{"kind": "instructions"})
	if len(instr.Results) == 0 {
		t.Fatal("нет инструкций Codex у проекта demo")
	}
	for _, h := range instr.Results {
		if h.Kind != kindInstructions || h.Agent != "codex" {
			t.Errorf("kind=instructions: %+v", h)
		}
	}
	skills := search(t, cs, map[string]any{"kind": "skill", "agent": "all"})
	if len(skills.Results) == 0 {
		t.Fatal("нет навыков у проекта demo")
	}
	for _, h := range skills.Results {
		if h.Kind != kindSkill {
			t.Errorf("kind=skill: %+v", h)
		}
	}

	// Лимит: total — сколько всего, results — не больше лимита.
	one := search(t, cs, map[string]any{"agent": "all", "limit": 1})
	if len(one.Results) != 1 || one.Total <= 1 {
		t.Errorf("limit=1: total=%d results=%d", one.Total, len(one.Results))
	}

	for _, args := range []map[string]any{
		{"agent": "gemini"},
		{"kind": "secrets"},
		{"project": "global"},
		{"project": "no-such-project"},
	} {
		var out SearchOut
		if msg := call(t, cs, "search_memory", args, &out); msg == "" {
			t.Errorf("%v: нужна ошибка, получено %+v", args, out)
		}
	}
}

// По умолчанию видна только память текущего проекта; глобальная — с
// флагом Global, другие проекты — с AllProjects.
func TestScope(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	if _, err := f.lib.Rebuild(ctx); err != nil {
		t.Fatal(err)
	}
	page, err := f.lib.Search(ctx, index.Query{ProjectID: catalog.GlobalID})
	if err != nil || len(page.Hits) == 0 {
		t.Fatalf("в фикстурах нет глобальных записей: %v", err)
	}
	globalID := page.Hits[0].ID

	projectsOf := func(cs *mcp.ClientSession) ListProjectsOut {
		var out ListProjectsOut
		if msg := call(t, cs, "list_projects", nil, &out); msg != "" {
			t.Fatal(msg)
		}
		return out
	}

	plain := f.connect(t, Options{}, "codex-mcp-client", filepath.Join(f.demo(), "src"))
	lp := projectsOf(plain)
	if len(lp.Projects) != 1 || !lp.Projects[0].Current || lp.Current != lp.Projects[0].ID {
		t.Errorf("без флагов виден только текущий проект (из подкаталога): %+v", lp)
	}
	var rec Record
	if msg := call(t, plain, "get_memory", map[string]any{"id": globalID}, &rec); !strings.Contains(msg, "not found") {
		t.Errorf("глобальная запись без флага: %q %+v", msg, rec)
	}

	global := f.connect(t, Options{Global: true}, "codex-mcp-client", f.demo())
	if lp := projectsOf(global); len(lp.Projects) != 2 {
		t.Errorf("с Global — текущий и глобальная: %+v", lp)
	}
	out := search(t, global, map[string]any{"project": "global"})
	if len(out.Results) == 0 || out.Projects[0] != "global" {
		t.Errorf("project=global: %+v", out)
	}
	if msg := call(t, global, "get_memory", map[string]any{"id": globalID}, &rec); msg != "" {
		t.Errorf("глобальная запись с флагом: %s", msg)
	}

	// Каталог вне известных проектов: без AllProjects искать негде.
	outside := f.connect(t, Options{}, "codex-mcp-client", f.home)
	var so SearchOut
	if msg := call(t, outside, "search_memory", map[string]any{"query": "yarn"}, &so); !strings.Contains(msg, "not a project") {
		t.Errorf("вне проекта: %q", msg)
	}
	all := f.connect(t, Options{AllProjects: true}, "codex-mcp-client", f.home)
	if lp := projectsOf(all); len(lp.Projects) < 3 || lp.Current != "" {
		t.Errorf("AllProjects: %+v", lp)
	}
	if out := search(t, all, map[string]any{"project": "all", "query": "yarn"}); len(out.Results) == 0 {
		t.Errorf("project=all: %+v", out)
	}
}

// Флаг Project важнее рабочего каталога.
func TestProjectFlag(t *testing.T) {
	f := newFixture(t)
	var lp ListProjectsOut
	cs := f.connect(t, Options{Project: f.demo()}, "codex-mcp-client", f.home)
	if msg := call(t, cs, "list_projects", nil, &lp); msg != "" {
		t.Fatal(msg)
	}
	if lp.Dir != f.demo() || lp.Current == "" {
		t.Errorf("флаг Project: %+v", lp)
	}
}

// Критерий готовности фазы 5: ни один инструмент не меняет файлы на диске —
// ни память агентов, ни каталог данных ai-memory.
func TestToolsDoNotWrite(t *testing.T) {
	f := newFixture(t)
	before := snapshot(t, f.home, f.data)
	cs := f.connect(t, Options{Global: true, AllProjects: true}, "codex-mcp-client", f.demo())
	var lp ListProjectsOut
	call(t, cs, "list_projects", nil, &lp)
	out := search(t, cs, map[string]any{"project": "all", "agent": "all", "limit": 50})
	for _, h := range out.Results {
		var rec Record
		call(t, cs, "get_memory", map[string]any{"id": h.ID}, &rec)
	}
	after := snapshot(t, f.home, f.data)
	for p, st := range after {
		if before[p] != st {
			t.Errorf("изменён или создан %s", p)
		}
	}
	for p := range before {
		if _, ok := after[p]; !ok {
			t.Errorf("удалён %s", p)
		}
	}
}

func TestMatchProject(t *testing.T) {
	root := filepath.Join(string(filepath.Separator), "w")
	pj := func(id, path string) model.Project { return model.Project{ID: id, Path: path} }
	projects := []model.Project{
		{ID: "global", Global: true},
		pj("demo", filepath.Join(root, "demo")),
		pj("inner", filepath.Join(root, "demo", "pkg", "inner")),
		pj("demo2", filepath.Join(root, "demo2")),
		{ID: "encoded:x"},
	}
	tests := []struct{ dir, want string }{
		{filepath.Join(root, "demo"), "demo"},
		{filepath.Join(root, "demo", "src"), "demo"},
		{filepath.Join(root, "demo", "pkg", "inner", "x"), "inner"},
		{filepath.Join(root, "demo2"), "demo2"},
		{filepath.Join(root, "dem"), ""},
		{root, ""},
		{"", ""},
	}
	for _, tt := range tests {
		got := ""
		if p := matchProject(projects, tt.dir); p != nil {
			got = p.ID
		}
		if got != tt.want {
			t.Errorf("matchProject(%q) = %q, want %q", tt.dir, got, tt.want)
		}
	}
}
