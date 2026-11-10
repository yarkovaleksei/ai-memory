package agentfiles

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/yarkovaleksei/ai-memory/internal/model"
)

func fixtureHome(t *testing.T) string {
	t.Helper()
	home, err := filepath.Abs(filepath.Join("..", "..", "..", "fixtures", "home"))
	if err != nil {
		t.Fatal(err)
	}
	return home
}

func scanAll(t *testing.T, a Adapter) ([]model.Source, []model.Item) {
	t.Helper()
	ctx := context.Background()
	srcs, err := a.Discover(ctx)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	var items []model.Item
	for _, s := range srcs {
		got, err := a.Scan(ctx, s)
		if err != nil {
			t.Fatalf("Scan %s: %v", s.ID, err)
		}
		items = append(items, got...)
	}
	MarkShared(items)
	return srcs, items
}

func codes(ws []model.Warning) string {
	var out []string
	for _, w := range ws {
		out = append(out, w.Code)
	}
	return strings.Join(out, ",")
}

func TestScanFixtures(t *testing.T) {
	home := fixtureHome(t)
	demo := filepath.Join(home, "projects", "demo")
	a := Adapter{
		ClaudeProfiles: []string{filepath.Join(home, ".claude"), filepath.Join(home, ".claude-work")},
		CodexHome:      filepath.Join(home, ".codex"),
		AgentsDir:      filepath.Join(home, ".agents"),
		// Несуществующий проект пропускается молча: путь из памяти не обязан существовать.
		Projects: []string{demo, filepath.Join(home, "projects", "gone"), demo},
	}
	srcs, items := scanAll(t, a)
	if len(srcs) != 6 {
		t.Fatalf("хранилищ %d, want 6: %+v", len(srcs), srcs)
	}
	for _, s := range srcs {
		if !s.Files {
			t.Errorf("%s: Files = false", s.ID)
		}
	}

	type row struct {
		source, title, kind, role, write, warnings string
		project                                    string // "" — глобальная
	}
	want := []row{
		{".claude", "CLAUDE.md", "instructions", RoleMain, "direct", "", ""},
		{".claude", "rules/style.md", "instructions", RoleRule, "direct", "", ""},
		{".claude", "release-notes", "skill", RoleUser, "direct", "", ""},
		{".claude", "report-writer", "skill", RoleSynced, "none", "", ""},
		{".claude-work", "CLAUDE.md", "instructions", RoleMain, "direct", "", ""},
		{".codex", "AGENTS.md", "instructions", RoleMain, "direct", "", ""},
		{".codex", "lint-fix", "skill", RoleUser, "direct", "", ""},
		{".codex", "no-description", "skill", RoleUser, "direct", "codex.skill.no-description", ""},
		{".codex", "plan-helper", "skill", RoleSystem, "none", "", ""},
		{".agents", "commit-message", "skill", RoleUser, "direct", "", ""},
		{ClaudeProjects, "CLAUDE.md", "instructions", RoleMain, "direct", "", demo},
		{ClaudeProjects, "CLAUDE.local.md", "instructions", RoleLocal, "direct", "", demo},
		{ClaudeProjects, ".claude/rules/testing.md", "instructions", RoleRule, "direct", "", demo},
		{ClaudeProjects, "deploy-preview", "skill", RoleProject, "direct", "", demo},
		{CodexProjects, "AGENTS.override.md", "instructions", RoleOverride, "direct", "", demo},
		{CodexProjects, "AGENTS.md", "instructions", RoleMain, "direct", "codex.instructions.shadowed", demo},
		{CodexProjects, "changelog", "skill", RoleProject, "direct", "", demo},
	}
	label := map[string]string{}
	for _, s := range srcs {
		label[s.ID] = s.Label
		if s.Label == "" {
			label[s.ID] = s.ID
		}
	}
	var got []row
	for _, it := range items {
		r := row{label[it.SourceID], it.Title, string(it.Kind), it.Role, string(it.Write), codes(it.Warnings), it.Project.Path}
		if it.Project.Global != (r.project == "") {
			t.Errorf("%s: Global = %v при проекте %q", it.Title, it.Project.Global, r.project)
		}
		got = append(got, r)
	}
	key := func(r row) string { return r.source + "|" + r.title }
	sort.Slice(want, func(i, j int) bool { return key(want[i]) < key(want[j]) })
	sort.Slice(got, func(i, j int) bool { return key(got[i]) < key(got[j]) })
	if len(got) != len(want) {
		t.Fatalf("записей %d, want %d:\n got %+v\nwant %+v", len(got), len(want), got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("запись %d:\n got %+v\nwant %+v", i, got[i], want[i])
		}
	}

	byTitle := map[string]model.Item{}
	for _, it := range items {
		byTitle[it.SourceID+"|"+it.Title] = it
	}
	rule := byTitle[prefixClaude+filepath.Join(home, ".claude")+"|rules/style.md"]
	if rule.Frontmatter == nil || rule.Description != "Стиль Go (фикстура)" || strings.Contains(rule.Body, "paths:") {
		t.Errorf("правило с paths: frontmatter %v, описание %q, тело %q", rule.Frontmatter, rule.Description, rule.Body)
	}
	skill := byTitle[ClaudeProjects+"|deploy-preview"]
	if skill.Description != "Выкатывает превью ветки. Фикстура проектного навыка Claude." || skill.Frontmatter["allowed-tools"] == nil {
		t.Errorf("навык: описание %q, frontmatter %v", skill.Description, skill.Frontmatter)
	}
	// Фикстуры лежат внутри репозитория ai-memory: файлы проекта в git.
	if skill.Repo == "" || byTitle[ClaudeProjects+"|CLAUDE.md"].Repo == "" {
		t.Errorf("файлы проекта не помечены как файлы в git-репозитории")
	}
	if g := byTitle[prefixClaude+filepath.Join(home, ".claude")+"|CLAUDE.md"]; g.Repo != "" {
		t.Errorf("глобальный CLAUDE.md помечен как файл проекта в git: %q", g.Repo)
	}
}

func TestNoProjectsNoProjectSources(t *testing.T) {
	home := fixtureHome(t)
	srcs, _ := scanAll(t, Adapter{CodexHome: filepath.Join(home, ".codex")})
	if len(srcs) != 1 || srcs[0].ID != prefixCodex+filepath.Join(home, ".codex") {
		t.Fatalf("хранилища %+v, want только CODEX_HOME", srcs)
	}
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func symlink(t *testing.T, target, link string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("симлинки недоступны: %v", err)
	}
}

const skillMD = "---\nname: shared\ndescription: Общий навык.\n---\n\n# Общий\n"

// Навык из ~/.agents подключён в оба профиля Claude симлинками: три записи,
// все общие, каждая ссылается на две другие. Отдельный навык профиля — не общий.
func TestSharedSkills(t *testing.T) {
	root := t.TempDir()
	agents := filepath.Join(root, ".agents")
	write(t, filepath.Join(agents, "skills", "shared", "SKILL.md"), skillMD)
	symlink(t, filepath.Join("..", "..", ".agents", "skills", "shared"), filepath.Join(root, ".claude", "skills", "shared"))
	symlink(t, filepath.Join(agents, "skills", "shared"), filepath.Join(root, ".claude-work", "skills", "shared"))
	write(t, filepath.Join(root, ".claude", "skills", "own", "SKILL.md"), "---\nname: own\ndescription: Свой.\n---\n")
	// Симлинк на сам SKILL.md: Claude его прочитает, Codex — нет.
	write(t, filepath.Join(root, "elsewhere", "SKILL.md"), skillMD)
	symlink(t, filepath.Join(root, "elsewhere", "SKILL.md"), filepath.Join(agents, "skills", "linked-file", "SKILL.md"))

	_, items := scanAll(t, Adapter{
		ClaudeProfiles: []string{filepath.Join(root, ".claude"), filepath.Join(root, ".claude-work")},
		AgentsDir:      agents,
	})
	shared := 0
	for _, it := range items {
		dir := filepath.Base(filepath.Dir(it.File))
		switch dir {
		case "shared":
			shared++
			if !it.Shared || len(it.Links) != 2 {
				t.Errorf("%s: Shared=%v, ссылок %d, want общий с 2 ссылками", it.File, it.Shared, len(it.Links))
			}
			isLink := it.Agent == model.AgentClaude
			if (it.Target != "") != isLink {
				t.Errorf("%s: Target = %q, симлинк = %v", it.File, it.Target, isLink)
			}
		case "own":
			if it.Shared || it.Target != "" {
				t.Errorf("свой навык помечен общим: %+v", it)
			}
		case "linked-file":
			if codes(it.Warnings) != "codex.skill.file-symlink" {
				t.Errorf("симлинк на SKILL.md: предупреждения %q", codes(it.Warnings))
			}
		}
	}
	if shared != 3 {
		t.Errorf("записей общего навыка %d, want 3", shared)
	}
}

func TestProjectRepo(t *testing.T) {
	root := t.TempDir()
	inRepo := filepath.Join(root, "repo", "app")
	plain := filepath.Join(root, "plain")
	write(t, filepath.Join(root, "repo", ".git", "HEAD"), "ref: refs/heads/main\n")
	write(t, filepath.Join(inRepo, "AGENTS.md"), "# Правила\n")
	write(t, filepath.Join(plain, "AGENTS.md"), "# Правила\n")

	_, items := scanAll(t, Adapter{Projects: []string{inRepo, plain}})
	repos := map[string]string{}
	for _, it := range items {
		repos[it.Project.Path] = it.Repo
	}
	if repos[inRepo] != filepath.Join(root, "repo") {
		t.Errorf("проект в подкаталоге репозитория: Repo = %q", repos[inRepo])
	}
	if repos[plain] != "" {
		t.Errorf("проект вне git: Repo = %q", repos[plain])
	}
}

// Глобальный пустой AGENTS.override.md Codex не считает, проектный — считает.
func TestCodexOverrideShadowing(t *testing.T) {
	tests := []struct {
		name     string
		override string
		global   bool
		shadowed bool
	}{
		{"глобальный непустой", "# Замена\n", true, true},
		{"глобальный пустой", "  \n", true, false},
		{"проектный пустой", "", false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			write(t, filepath.Join(dir, AgentsMD), "# Правила\n")
			write(t, filepath.Join(dir, AgentsOverMD), tt.override)
			a := Adapter{CodexHome: dir}
			if !tt.global {
				a = Adapter{Projects: []string{dir}}
			}
			_, items := scanAll(t, a)
			for _, it := range items {
				if it.Role != RoleMain {
					continue
				}
				if got := codes(it.Warnings) == "codex.instructions.shadowed"; got != tt.shadowed {
					t.Errorf("AGENTS.md затенён = %v, want %v", got, tt.shadowed)
				}
			}
		})
	}
}

// В корне профиля важны только файлы инструкций: история и логи агента
// индекс не пересобирают.
func TestWatchNames(t *testing.T) {
	home := fixtureHome(t)
	a := Adapter{ClaudeProfiles: []string{filepath.Join(home, ".claude")}, Projects: []string{filepath.Join(home, "projects", "demo")}}
	srcs, _ := scanAll(t, a)
	for _, s := range srcs {
		names := a.WatchNames(s)
		dirs := a.WatchDirs(s)
		for d := range names {
			if !contains(dirs, d) {
				t.Errorf("%s: каталог %s с фильтром не наблюдается", s.ID, d)
			}
		}
		if s.ID == prefixClaude+filepath.Join(home, ".claude") {
			if got := names[s.Root]; !contains(got, ClaudeMD) || contains(got, "history.jsonl") {
				t.Errorf("имена корня профиля: %v", got)
			}
		}
	}
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
