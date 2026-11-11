package claude

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"

	"github.com/yarkovaleksei/ai-memory/internal/model"
)

func fixtureHome(t *testing.T) string {
	t.Helper()
	p, err := filepath.Abs(filepath.Join("..", "..", "..", "fixtures", "home"))
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func scanFixtures(t *testing.T) ([]model.Source, map[string][]model.Item) {
	t.Helper()
	home := fixtureHome(t)
	a := Adapter{Profiles: []string{
		filepath.Join(home, ".claude"),
		filepath.Join(home, ".claude-work"),
		filepath.Join(home, ".claude-missing"),
	}}
	srcs, err := a.Discover(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	items := map[string][]model.Item{}
	for _, s := range srcs {
		got, err := a.Scan(context.Background(), s)
		if err != nil {
			t.Fatalf("Scan %s: %v", s.ID, err)
		}
		items[s.Label] = got
	}
	return srcs, items
}

func byTitle(items []model.Item, title string) *model.Item {
	for i := range items {
		if items[i].Title == title {
			return &items[i]
		}
	}
	return nil
}

func hasWarning(it *model.Item, code string) bool {
	return slices.ContainsFunc(it.Warnings, func(w model.Warning) bool { return w.Code == code })
}

func TestDiscoverSkipsMissingProfiles(t *testing.T) {
	srcs, _ := scanFixtures(t)
	var labels []string
	for _, s := range srcs {
		labels = append(labels, s.Label)
		if s.Agent != model.AgentClaude {
			t.Errorf("%s: agent %q", s.ID, s.Agent)
		}
	}
	if !slices.Equal(labels, []string{".claude", ".claude-work"}) {
		t.Errorf("labels = %v", labels)
	}
}

func TestScanFacts(t *testing.T) {
	_, items := scanFixtures(t)
	main := items[".claude"]

	tests := []struct {
		title    string
		kind     model.ItemKind
		typ      string
		warnings []string
		project  string
	}{
		{"Пакеты только через yarn", model.KindClaudeFact, "feedback", nil, "/home/me/projects/demo"},
		{"Как выпускается релиз", model.KindClaudeFact, "project", nil, "/home/me/projects/demo"},
		{"Трекер задач", model.KindClaudeFact, "reference", nil, "/home/me/projects/demo"},
		{"Сломанный frontmatter", model.KindClaudeFact, "", []string{"claude.frontmatter.invalid"}, "/home/me/projects/demo"},
		{"orphan-fact", model.KindClaudeFact, "project", []string{"claude.index.not-listed"}, "/home/me/projects/demo"},
		{"MEMORY.md", model.KindClaudeIndex, "", []string{"claude.index.missing-file", "claude.index.unlisted"}, "/home/me/projects/demo"},
		{"Старый формат", model.KindClaudeFact, "project", nil, ""},
		{"Черновик", model.KindClaudeFact, "project", nil, "/tmp/scratch-42"},
	}
	for _, tt := range tests {
		t.Run(tt.title, func(t *testing.T) {
			it := byTitle(main, tt.title)
			if it == nil {
				t.Fatalf("запись %q не найдена", tt.title)
			}
			if it.Kind != tt.kind || it.Type != tt.typ {
				t.Errorf("kind/type = %s/%q, want %s/%q", it.Kind, it.Type, tt.kind, tt.typ)
			}
			for _, w := range tt.warnings {
				if !hasWarning(it, w) {
					t.Errorf("нет предупреждения %s: %+v", w, it.Warnings)
				}
			}
			if len(tt.warnings) == 0 && len(it.Warnings) > 0 {
				t.Errorf("лишние предупреждения: %+v", it.Warnings)
			}
			if tt.project != "" && it.Project.Path != filepath.FromSlash(tt.project) {
				t.Errorf("project = %+v, want %s", it.Project, tt.project)
			}
		})
	}
}

func TestProjectRefs(t *testing.T) {
	_, items := scanFixtures(t)
	main := items[".claude"]

	legacy := byTitle(main, "Старый формат")
	if legacy.Project.Path != "" || legacy.Project.Encoded != "-home-me-projects-my-app-v2" {
		t.Errorf("проект без сессий должен остаться закодированным: %+v", legacy.Project)
	}
	scratch := byTitle(main, "Черновик")
	if !scratch.Project.Temp {
		t.Errorf("проект в /tmp должен быть временным: %+v", scratch.Project)
	}
	if byTitle(main, "Пакеты только через yarn").Project.Temp {
		t.Error("обычный проект помечен временным")
	}
}

func TestDescriptionAndLinks(t *testing.T) {
	_, items := scanFixtures(t)
	main := items[".claude"]

	yarn := byTitle(main, "Пакеты только через yarn")
	if yarn.Description != "Зависимости фронтенда ставятся только через yarn 4" {
		t.Errorf("description = %q", yarn.Description)
	}
	release := byTitle(main, "Как выпускается релиз")
	if len(release.Links) != 1 || release.Links[0].Text != "prefer-yarn" || release.Links[0].TargetID != yarn.ID {
		t.Errorf("links = %+v, want prefer-yarn → %s", release.Links, yarn.ID)
	}
	index := byTitle(main, "MEMORY.md")
	var missing int
	for _, l := range index.Links {
		if l.TargetID == "" {
			missing++
		}
	}
	if len(index.Links) != 5 || missing != 1 {
		t.Errorf("ссылки индекса: %+v", index.Links)
	}
}

func TestIDsAreStableAndUnique(t *testing.T) {
	_, first := scanFixtures(t)
	_, second := scanFixtures(t)
	seen := map[string]bool{}
	for label, items := range first {
		for i, it := range items {
			if seen[it.ID] {
				t.Errorf("повтор ID %s (%s)", it.ID, it.Title)
			}
			seen[it.ID] = true
			if second[label][i].ID != it.ID {
				t.Errorf("ID меняется между сканами: %s", it.Title)
			}
		}
	}
}

func TestEncodeProjectPath(t *testing.T) {
	tests := map[string]string{
		"/home/me/github.com/me/my_app": "-home-me-github-com-me-my-app",
		"/home/me/.claude":              "-home-me--claude",
		`C:\Users\me\app`:               "C--Users-me-app",
		"/tmp/проект":                   "-tmp-------",
	}
	for in, want := range tests {
		if got := EncodeProjectPath(in); got != want {
			t.Errorf("Encode(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDecodeByDisk(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("подбор по диску проверяется на путях Unix; Windows — в CI после публикации")
	}
	root := t.TempDir()
	for _, d := range []string{"work/my_app.v2", "work/my-app/v2", "work/.hidden/x"} {
		if err := os.MkdirAll(filepath.Join(root, filepath.FromSlash(d)), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	tests := []struct {
		target string
		ok     bool
	}{
		{"work/my_app.v2", true},
		{"work/.hidden/x", true},
		{"work/nope", false},
	}
	for _, tt := range tests {
		want := filepath.Join(root, filepath.FromSlash(tt.target))
		got, ok := DecodeByDisk(EncodeProjectPath(want), runtime.GOOS)
		if ok != tt.ok {
			t.Errorf("%s: ok = %v", tt.target, ok)
			continue
		}
		// my_app.v2 и my-app/v2 кодируются одинаково: годится любой существующий вариант.
		if ok && EncodeProjectPath(got) != EncodeProjectPath(want) {
			t.Errorf("%s: got %s", tt.target, got)
		}
	}
}

func TestAutoMemoryDirectoryWarning(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "settings.json"), []byte(`{"autoMemoryDirectory":"/elsewhere"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	srcs, err := Adapter{Profiles: []string{root}}.Discover(context.Background())
	if err != nil || len(srcs) != 1 {
		t.Fatalf("Discover: %v %v", srcs, err)
	}
	if len(srcs[0].Warnings) != 1 || srcs[0].Warnings[0].Code != "claude.settings.auto-memory-directory" {
		t.Errorf("warnings = %+v", srcs[0].Warnings)
	}
}

// TestDiscoverSameDirTwice: один каталог под двумя именами (симлинк и сам каталог,
// оба в настройках) даёт одно хранилище, иначе записи задвоятся.
func TestDiscoverSameDirTwice(t *testing.T) {
	base := t.TempDir()
	work := filepath.Join(base, ".claude-work")
	other := filepath.Join(base, ".claude-personal")
	link := filepath.Join(base, ".claude")
	for _, d := range []string{work, other} {
		if err := os.Mkdir(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(work, link); err != nil {
		t.Skipf("симлинки недоступны: %v", err) // Windows без прав разработчика
	}

	tests := []struct {
		name     string
		profiles []string
		want     []string
	}{
		{"симлинк и цель — одно хранилище, имя первого", []string{link, work, other}, []string{".claude", ".claude-personal"}},
		{"цель раньше симлинка", []string{work, link}, []string{".claude-work"}},
		{"повтор одного пути", []string{other, other}, []string{".claude-personal"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srcs, err := Adapter{Profiles: tt.profiles}.Discover(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			var labels []string
			for _, s := range srcs {
				labels = append(labels, s.Label)
			}
			if !slices.Equal(labels, tt.want) {
				t.Errorf("labels = %v, want %v", labels, tt.want)
			}
		})
	}
}

// Пути проектов для инструкций берутся и из каталогов без памяти;
// нераспознанные пропускаются (ADR-025).
func TestSessionProjects(t *testing.T) {
	home := fixtureHome(t)
	got := SessionProjects(context.Background(), []string{filepath.Join(home, ".claude"), filepath.Join(home, ".claude-work")})
	want := map[string]bool{
		filepath.FromSlash("/home/me/projects/demo"):  true,
		filepath.FromSlash("/home/me/projects/empty"): true, // без каталога memory/
		filepath.FromSlash("/tmp/scratch-42"):         true,
	}
	seen := map[string]bool{}
	for _, p := range got {
		if !want[p] {
			t.Errorf("лишний путь %s", p)
		}
		seen[p] = true
	}
	if len(seen) != len(want) {
		t.Errorf("пути %v, want %v", got, want)
	}
}
