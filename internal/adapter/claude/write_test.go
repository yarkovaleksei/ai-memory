package claude

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yarkovaleksei/ai-memory/internal/backup"
	"github.com/yarkovaleksei/ai-memory/internal/model"
	"github.com/yarkovaleksei/ai-memory/internal/writer"
)

// copyTree копирует фикстуры во временный каталог: тесты записи пишут только туда.
func copyTree(t *testing.T, src string) string {
	t.Helper()
	dst := filepath.Join(t.TempDir(), filepath.Base(src))
	err := filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		in, err := os.Open(p)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.Create(target)
		if err != nil {
			return err
		}
		if _, err := io.Copy(out, in); err != nil {
			out.Close()
			return err
		}
		return out.Close()
	})
	if err != nil {
		t.Fatal(err)
	}
	return dst
}

type fixture struct {
	profile string
	memDir  string
	w       *writer.Writer
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	now = func() time.Time { return time.Date(2026, 2, 3, 4, 5, 6, 0, time.UTC) }
	t.Cleanup(func() { now = time.Now })
	profile := copyTree(t, filepath.Join(fixtureHome(t), ".claude"))
	repo, err := backup.Open(context.Background(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return fixture{
		profile: profile,
		memDir:  MemoryDir(profile, "-home-me-projects-demo"),
		w:       &writer.Writer{Roots: []string{profile}, Backup: repo},
	}
}

func (f fixture) read(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(f.memDir, name))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func (f fixture) hash(t *testing.T, name string) string {
	return model.ContentHash([]byte(f.read(t, name)))
}

// apply(t)(PlanXxx(...)): Go не даёт передать кортеж вместе с другими аргументами.
func (f fixture) apply(t *testing.T) func(writer.Plan, error) writer.Result {
	return func(p writer.Plan, err error) writer.Result {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		res, err := f.w.Apply(context.Background(), p)
		if err != nil {
			t.Fatal(err)
		}
		return res
	}
}

func failCode(err error) string {
	var we *writer.Error
	if errors.As(err, &we) {
		return we.Code
	}
	return ""
}

func TestPlanCreate(t *testing.T) {
	f := newFixture(t)
	indexBefore := f.read(t, IndexFile)
	in := FactInput{Name: "ci-cache", Description: "Кэш CI сбрасывается вручную", Type: "project",
		Title: "Кэш CI", Body: "Сбрасывай кэш через кнопку в CI.\n\n**Why:** иначе тесты берут старые пакеты."}
	res := f.apply(t)(PlanCreate(f.memDir, in))

	want := "---\nname: ci-cache\ndescription: Кэш CI сбрасывается вручную\nmetadata:\n  type: project\n  modified: 2026-02-03T04:05:06.000Z\n---\n\n" +
		"Сбрасывай кэш через кнопку в CI.\n\n**Why:** иначе тесты берут старые пакеты.\n"
	if got := f.read(t, "ci-cache.md"); got != want {
		t.Errorf("файл факта:\n%s\nwant:\n%s", got, want)
	}
	if got := f.read(t, IndexFile); got != indexBefore+"- [Кэш CI](ci-cache.md) — Кэш CI сбрасывается вручную\n" {
		t.Errorf("индекс:\n%s", got)
	}

	// Новый факт читается адаптером как обычный, без предупреждений.
	items, _ := scanMemoryDir(model.Source{ID: "s", Root: f.profile}, f.memDir, model.ProjectRef{})
	var found *model.Item
	for i := range items {
		if filepath.Base(items[i].File) == "ci-cache.md" {
			found = &items[i]
		}
	}
	if found == nil || found.Type != "project" || found.Title != "Кэш CI" || len(found.Warnings) != 0 {
		t.Errorf("адаптер читает новый факт так: %+v", found)
	}

	// Откат убирает и файл, и строку индекса.
	rev, err := f.w.PlanRevert(context.Background(), res.Commit)
	f.apply(t)(rev, err)
	if _, err := os.Stat(filepath.Join(f.memDir, "ci-cache.md")); !errors.Is(err, fs.ErrNotExist) {
		t.Error("после отката файл факта остался")
	}
	if got := f.read(t, IndexFile); got != indexBefore {
		t.Errorf("после отката индекс не совпадает с исходным:\n%s", got)
	}
}

func TestPlanCreateInNewProject(t *testing.T) {
	f := newFixture(t)
	memDir := MemoryDir(f.profile, "-home-me-projects-fresh")
	f.apply(t)(PlanCreate(memDir, FactInput{Name: "a", Description: "d", Type: "user", Body: "b"}))
	data, err := os.ReadFile(filepath.Join(memDir, IndexFile))
	if err != nil || string(data) != "- [a](a.md) — d\n" {
		t.Errorf("индекс нового проекта: %q %v", data, err)
	}
}

func TestPlanCreateRejects(t *testing.T) {
	f := newFixture(t)
	ok := FactInput{Name: "x", Description: "d", Type: "user", Body: "b"}
	tests := []struct {
		name string
		mod  func(*FactInput)
		want string
	}{
		{"имя с пробелом", func(in *FactInput) { in.Name = "bad name" }, "claude.fact.name-invalid"},
		{"имя с ..", func(in *FactInput) { in.Name = "../x" }, "claude.fact.name-invalid"},
		{"заглавные буквы", func(in *FactInput) { in.Name = "Prefer" }, "claude.fact.name-invalid"},
		{"факт уже есть", func(in *FactInput) { in.Name = "prefer-yarn" }, "claude.fact.exists"},
		{"неизвестный тип", func(in *FactInput) { in.Type = "note" }, "claude.fact.type-invalid"},
		{"пустое описание", func(in *FactInput) { in.Description = " " }, "claude.fact.description-invalid"},
		{"многострочное описание", func(in *FactInput) { in.Description = "a\nb" }, "claude.fact.description-invalid"},
		{"пустой текст", func(in *FactInput) { in.Body = "\n" }, "claude.fact.body-empty"},
		{"скобки в заголовке", func(in *FactInput) { in.Title = "a]b" }, "claude.fact.index-line-invalid"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := ok
			tt.mod(&in)
			if _, err := PlanCreate(f.memDir, in); failCode(err) != tt.want {
				t.Errorf("код = %v, want %s", err, tt.want)
			}
		})
	}
}

func TestPlanUpdateKeepsFrontmatter(t *testing.T) {
	f := newFixture(t)
	before := f.read(t, "prefer-yarn.md")
	indexBefore := f.read(t, IndexFile)
	in := FactInput{Description: "Только yarn 4", Type: "feedback", Hook: "никакого npm", Body: "Новый текст."}
	res := f.apply(t)(PlanUpdate(filepath.Join(f.memDir, "prefer-yarn.md"), f.hash(t, "prefer-yarn.md"), in))

	want := "---\nname: prefer-yarn\ndescription: Только yarn 4\nmetadata:\n  type: feedback\n" +
		"  originSessionId: 00000000-0000-0000-0000-000000000001\n  modified: 2026-02-03T04:05:06.000Z\n---\n\nНовый текст.\n"
	if got := f.read(t, "prefer-yarn.md"); got != want {
		t.Errorf("факт:\n%s\nwant:\n%s", got, want)
	}
	if got := f.read(t, IndexFile); !strings.HasPrefix(got, "- [Пакеты только через yarn](prefer-yarn.md) — никакого npm\n- [Как выпускается релиз]") {
		t.Errorf("строка индекса не обновлена на месте:\n%s", got)
	}

	rev, err := f.w.PlanRevert(context.Background(), res.Commit)
	f.apply(t)(rev, err)
	if f.read(t, "prefer-yarn.md") != before || f.read(t, IndexFile) != indexBefore {
		t.Error("откат не вернул файлы байт в байт")
	}
}

func TestPlanUpdateLegacyType(t *testing.T) {
	f := newFixture(t)
	f.memDir = MemoryDir(f.profile, "-home-me-projects-my-app-v2")
	f.apply(t)(PlanUpdate(filepath.Join(f.memDir, "legacy-type.md"), f.hash(t, "legacy-type.md"),
		FactInput{Description: "d", Type: "reference", Body: "b"}))
	got := f.read(t, "legacy-type.md")
	if !strings.Contains(got, "\ntype: reference\n") || strings.Contains(got, "metadata") {
		t.Errorf("старый формат должен остаться старым:\n%s", got)
	}
}

func TestPlanUpdateConflictsAndBrokenYAML(t *testing.T) {
	f := newFixture(t)
	in := FactInput{Description: "d", Type: "user", Body: "b"}
	if _, err := PlanUpdate(filepath.Join(f.memDir, "prefer-yarn.md"), "чужой-хэш", in); failCode(err) != "write.conflict" {
		t.Errorf("устаревший хэш: %v", err)
	}
	broken := filepath.Join(f.memDir, "broken-frontmatter.md")
	if _, err := PlanUpdate(broken, f.hash(t, "broken-frontmatter.md"), in); failCode(err) != "claude.frontmatter.invalid" {
		t.Errorf("сломанный YAML: %v", err)
	}
	// Сломанный frontmatter чинится правкой исходника целиком.
	fixed := "---\nname: broken-frontmatter\ndescription: починено\nmetadata:\n  type: project\n---\n\nТекст.\n"
	f.apply(t)(PlanUpdateRaw(broken, f.hash(t, "broken-frontmatter.md"), fixed))
	if f.read(t, "broken-frontmatter.md") != fixed {
		t.Error("исходник не записан")
	}
}

func TestPlanDelete(t *testing.T) {
	f := newFixture(t)
	f.apply(t)(PlanDelete(filepath.Join(f.memDir, "tracker-link.md"), f.hash(t, "tracker-link.md")))
	if _, err := os.Stat(filepath.Join(f.memDir, "tracker-link.md")); !errors.Is(err, fs.ErrNotExist) {
		t.Error("файл не удалён")
	}
	if strings.Contains(f.read(t, IndexFile), "tracker-link.md") {
		t.Error("строка индекса осталась")
	}
}

func TestPlanFixIndex(t *testing.T) {
	f := newFixture(t)
	f.apply(t)(PlanFixIndex(f.memDir))
	got := f.read(t, IndexFile)
	if strings.Contains(got, "missing-fact.md") {
		t.Error("строка без файла осталась")
	}
	if !strings.Contains(got, "(orphan-fact.md)") {
		t.Errorf("факт без строки не добавлен:\n%s", got)
	}
	if _, err := PlanFixIndex(f.memDir); failCode(err) != "write.no-changes" {
		t.Errorf("повторная починка должна быть пустой: %v", err)
	}
}

func TestReadFact(t *testing.T) {
	f := newFixture(t)
	d, err := ReadFact(filepath.Join(f.memDir, "prefer-yarn.md"))
	if err != nil || !d.Parsed || d.Hash != f.hash(t, "prefer-yarn.md") {
		t.Fatalf("%+v %v", d, err)
	}
	want := FactInput{Name: "prefer-yarn", Type: "feedback", Title: "Пакеты только через yarn"}
	if d.Fact.Name != want.Name || d.Fact.Type != want.Type || d.Fact.Title != want.Title || d.Fact.Description == "" || d.Fact.Body == "" {
		t.Errorf("поля формы: %+v", d.Fact)
	}
	// Без правок форма даёт тот же файл, кроме отметки времени.
	p, err := PlanUpdate(filepath.Join(f.memDir, "prefer-yarn.md"), d.Hash, d.Fact)
	if err != nil {
		t.Fatal(err)
	}
	dropModified := func(s string) string {
		var out []string
		for _, l := range strings.Split(s, "\n") {
			if !strings.HasPrefix(strings.TrimSpace(l), "modified:") {
				out = append(out, l)
			}
		}
		return strings.Join(out, "\n")
	}
	if len(p.Changes) != 1 || dropModified(p.Changes[0].After) != dropModified(d.Content) {
		t.Errorf("план без правок должен менять только modified:\n%s\n%d изменений", p.Changes[0].After, len(p.Changes))
	}

	broken, err := ReadFact(filepath.Join(f.memDir, "broken-frontmatter.md"))
	if err != nil || broken.Parsed || broken.Content == "" {
		t.Errorf("сломанный frontmatter правится только исходником: %+v %v", broken, err)
	}
}

func TestPlanReplace(t *testing.T) {
	f := newFixture(t)
	other := MemoryDir(f.profile, "-home-me-projects-my-app-v2")
	legacy := filepath.Join(other, "legacy-type.md")
	legacyHash := model.ContentHash([]byte(mustRead(t, legacy)))
	in := FactInput{Name: "prefer-yarn", Description: "Только yarn 4", Type: "feedback", Body: "Объединено."}
	tests := []struct {
		name    string
		refs    []FactRef
		changes int
		code    string
	}{
		{"итог на месте одного из фактов, второй в другом проекте", []FactRef{
			{File: filepath.Join(f.memDir, "prefer-yarn.md"), Hash: f.hash(t, "prefer-yarn.md")},
			{File: legacy, Hash: legacyHash},
		}, 4, ""}, // факт, удалённый факт, два индекса
		{"имя занято файлом вне группы", []FactRef{{File: legacy, Hash: legacyHash}}, 0, "claude.fact.exists"},
		{"устаревший хэш", []FactRef{{File: legacy, Hash: "x"}}, 0, "write.conflict"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := PlanReplace(f.memDir, in, tt.refs)
			if failCode(err) != tt.code || len(p.Changes) != tt.changes {
				t.Errorf("изменений %d, код %v", len(p.Changes), err)
			}
		})
	}
}

func mustRead(t *testing.T, p string) string {
	t.Helper()
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
