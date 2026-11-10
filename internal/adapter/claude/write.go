package claude

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/yarkovaleksei/ai-memory/internal/markdown"
	"github.com/yarkovaleksei/ai-memory/internal/model"
	"github.com/yarkovaleksei/ai-memory/internal/writer"
)

// Планы записи для памяти Claude: факт — это файл <name>.md с frontmatter и
// строка «- [Заголовок](файл.md) — зацепка» в MEMORY.md того же каталога.
// Функции только читают диск и строят writer.Plan; пишет writer.

// FactInput — то, что пользователь задаёт для факта.
type FactInput struct {
	Name        string `json:"name"`        // slug, он же имя файла; не меняется при правке
	Description string `json:"description"` // одна строка: по ней Claude решает, нужен ли факт
	Type        string `json:"type"`        // user | feedback | project | reference
	Title       string `json:"title"`       // заголовок строки в MEMORY.md
	Hook        string `json:"hook"`        // зацепка в MEMORY.md; пусто — берётся description
	Body        string `json:"body"`
}

// Types — допустимые типы фактов Claude Code.
var Types = []string{"user", "feedback", "project", "reference"}

var slugRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,79}$`)

// now подменяется в тестах.
var now = time.Now

// MemoryDir — каталог памяти проекта в профиле Claude.
func MemoryDir(profile, encodedProject string) string {
	return filepath.Join(filepath.FromSlash(profile), "projects", encodedProject, "memory")
}

func (in FactInput) validate(requireName bool) error {
	if requireName && !slugRe.MatchString(in.Name) {
		return writer.Fail("claude.fact.name-invalid", "name", in.Name)
	}
	if strings.TrimSpace(in.Description) == "" || strings.Contains(in.Description, "\n") {
		return writer.Fail("claude.fact.description-invalid")
	}
	if !contains(Types, in.Type) {
		return writer.Fail("claude.fact.type-invalid", "type", in.Type)
	}
	if strings.TrimSpace(in.Body) == "" {
		return writer.Fail("claude.fact.body-empty")
	}
	if strings.ContainsAny(in.Title, "\n[]") || strings.Contains(in.Hook, "\n") {
		return writer.Fail("claude.fact.index-line-invalid")
	}
	return nil
}

func (in FactInput) title() string {
	if t := strings.TrimSpace(in.Title); t != "" {
		return t
	}
	return in.Name
}

func (in FactInput) hook() string {
	if h := strings.TrimSpace(in.Hook); h != "" {
		return h
	}
	return strings.TrimSpace(in.Description)
}

// PlanCreate — новый факт и его строка в MEMORY.md (индекс создаётся, если его нет).
func PlanCreate(memDir string, in FactInput) (writer.Plan, error) {
	if err := in.validate(true); err != nil {
		return writer.Plan{}, err
	}
	path := filepath.Join(memDir, in.Name+".md")
	if _, err := os.Lstat(path); err == nil {
		return writer.Plan{}, writer.Fail("claude.fact.exists", "name", in.Name)
	}
	fm := markdown.NewFrontmatter()
	fm.Set(in.Name, "name")
	fm.Set(strings.TrimSpace(in.Description), "description")
	fm.Set(in.Type, "metadata", "type")
	fm.Set(timestamp(), "metadata", "modified")
	content, err := markdown.Join(fm, in.Body)
	if err != nil {
		return writer.Plan{}, writer.Fail("claude.frontmatter.invalid", "error", err.Error())
	}
	idx, err := readIndex(memDir)
	if err != nil {
		return writer.Plan{}, err
	}
	newIdx := idx
	newIdx.after = appendLine(idx.before, indexLineFor(in.title(), in.Name+".md", in.hook()), idx.eol)
	return writer.NewPlan(
		writer.Op{Code: "claude.fact.create", Params: map[string]string{"name": in.Name}},
		writer.Change{Path: path, AfterExists: true, After: content},
		newIdx.change(),
	), nil
}

// PlanUpdate меняет описание, тип и текст факта, сохраняя остальной frontmatter,
// и обновляет его строку в MEMORY.md (или добавляет, если строки не было).
// hash — ContentHash файла, который видел пользователь.
func PlanUpdate(file, hash string, in FactInput) (writer.Plan, error) {
	if err := in.validate(false); err != nil {
		return writer.Plan{}, err
	}
	current, err := readExpected(file, hash)
	if err != nil {
		return writer.Plan{}, err
	}
	fmText, _, has := markdown.SplitFrontmatter(current)
	fm := markdown.NewFrontmatter()
	if has {
		if fm, err = markdown.ParseFrontmatterNode(fmText); err != nil {
			// Сломанный YAML чинится правкой исходника целиком (PlanUpdateRaw).
			return writer.Plan{}, writer.Fail("claude.frontmatter.invalid", "error", err.Error())
		}
	}
	name := strings.TrimSuffix(filepath.Base(file), filepath.Ext(file))
	if !fm.Has("name") {
		fm.Set(name, "name")
	}
	fm.Set(strings.TrimSpace(in.Description), "description")
	if _, legacy := fm.Get("type"); legacy && !fm.Has("metadata", "type") {
		fm.Set(in.Type, "type") // старый формат остаётся старым
	} else {
		fm.Set(in.Type, "metadata", "type")
	}
	if fm.Has("metadata") {
		fm.Set(timestamp(), "metadata", "modified")
	}
	content, err := markdown.Join(fm, in.Body)
	if err != nil {
		return writer.Plan{}, writer.Fail("claude.frontmatter.invalid", "error", err.Error())
	}
	changes := []writer.Change{{Path: file, BeforeExists: true, Before: current, AfterExists: true, After: content}}

	idx, err := readIndex(filepath.Dir(file))
	if err != nil {
		return writer.Plan{}, err
	}
	if in.Title == "" {
		in.Title = idx.titleOf(filepath.Base(file))
	}
	if in.Title == "" {
		in.Title = name
	}
	idx.after = idx.upsert(filepath.Base(file), indexLineFor(in.Title, filepath.Base(file), in.hook()))
	if idx.after != idx.before {
		changes = append(changes, idx.change())
	}
	return writer.NewPlan(writer.Op{Code: "claude.fact.update", Params: map[string]string{"name": name}}, changes...), nil
}

// PlanUpdateRaw заменяет файл факта целиком: так правится и сломанный frontmatter.
func PlanUpdateRaw(file, hash, content string) (writer.Plan, error) {
	current, err := readExpected(file, hash)
	if err != nil {
		return writer.Plan{}, err
	}
	if content == current {
		return writer.Plan{}, writer.Fail("write.no-changes")
	}
	name := strings.TrimSuffix(filepath.Base(file), filepath.Ext(file))
	return writer.NewPlan(writer.Op{Code: "claude.fact.update", Params: map[string]string{"name": name}},
		writer.Change{Path: file, BeforeExists: true, Before: current, AfterExists: true, After: content}), nil
}

// PlanDelete удаляет факт и все строки MEMORY.md, которые на него ссылаются.
func PlanDelete(file, hash string) (writer.Plan, error) {
	current, err := readExpected(file, hash)
	if err != nil {
		return writer.Plan{}, err
	}
	name := strings.TrimSuffix(filepath.Base(file), filepath.Ext(file))
	changes := []writer.Change{{Path: file, BeforeExists: true, Before: current}}
	idx, err := readIndex(filepath.Dir(file))
	if err != nil {
		return writer.Plan{}, err
	}
	idx.after = idx.remove(filepath.Base(file))
	if idx.after != idx.before {
		changes = append(changes, idx.change())
	}
	return writer.NewPlan(writer.Op{Code: "claude.fact.delete", Params: map[string]string{"name": name}}, changes...), nil
}

// PlanFixIndex приводит MEMORY.md в соответствие с файлами: убирает строки без
// файлов и добавляет строки для фактов, которых в индексе нет.
func PlanFixIndex(memDir string) (writer.Plan, error) {
	idx, err := readIndex(memDir)
	if err != nil {
		return writer.Plan{}, err
	}
	entries, err := os.ReadDir(memDir)
	if err != nil {
		return writer.Plan{}, writer.Fail("write.unreadable", "path", memDir, "error", err.Error())
	}
	present := map[string]bool{}
	text := idx.before
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || name == IndexFile || !strings.EqualFold(filepath.Ext(name), ".md") {
			continue
		}
		present[name] = true
	}
	for _, e := range parseIndex(text) {
		if !present[e.File] {
			text = removeLines(text, e.File)
		}
	}
	listed := map[string]bool{}
	for _, e := range parseIndex(text) {
		listed[e.File] = true
	}
	for _, e := range entries {
		name := e.Name()
		if !present[name] || listed[name] {
			continue
		}
		title, hook := strings.TrimSuffix(name, filepath.Ext(name)), ""
		if content, err := os.ReadFile(filepath.Join(memDir, name)); err == nil {
			if fmText, _, has := markdown.SplitFrontmatter(string(content)); has {
				if fm, err := markdown.ParseFrontmatter(fmText); err == nil {
					if n := markdown.String(fm, "name"); n != "" {
						title = n
					}
					hook = markdown.String(fm, "description")
				}
			}
		}
		text = appendLine(text, indexLineFor(title, name, hook), idx.eol)
	}
	if text == idx.before {
		return writer.Plan{}, writer.Fail("write.no-changes")
	}
	idx.after = text
	return writer.NewPlan(writer.Op{Code: "claude.index.fix"}, idx.change()), nil
}

// readExpected читает файл и сверяет его с тем, что видел пользователь.
func readExpected(file, hash string) (string, error) {
	data, err := os.ReadFile(file)
	if errors.Is(err, fs.ErrNotExist) {
		return "", writer.Fail("write.conflict", "path", file, "reason", "deleted")
	}
	if err != nil {
		return "", writer.Fail("write.unreadable", "path", file, "error", err.Error())
	}
	if hash != "" && model.ContentHash(data) != hash {
		return "", writer.Fail("write.conflict", "path", file, "reason", "changed")
	}
	return string(data), nil
}

func timestamp() string {
	return now().UTC().Format("2006-01-02T15:04:05.000Z")
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// --- MEMORY.md ---

type index struct {
	path          string
	exists        bool
	before, after string
	eol           string
}

func readIndex(memDir string) (index, error) {
	path := filepath.Join(memDir, IndexFile)
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return index{path: path, eol: "\n"}, nil
	}
	if err != nil {
		return index{}, writer.Fail("write.unreadable", "path", path, "error", err.Error())
	}
	eol := "\n"
	if strings.Contains(string(data), "\r\n") {
		eol = "\r\n"
	}
	return index{path: path, exists: true, before: string(data), after: string(data), eol: eol}, nil
}

func (ix index) change() writer.Change {
	return writer.Change{Path: ix.path, BeforeExists: ix.exists, Before: ix.before, AfterExists: true, After: ix.after}
}

func indexLineFor(title, file, hook string) string {
	line := "- [" + strings.TrimSpace(title) + "](" + file + ")"
	if hook = strings.TrimSpace(hook); hook != "" {
		line += " — " + hook
	}
	return line
}

func (ix index) titleOf(file string) string {
	for _, e := range parseIndex(ix.before) {
		if e.File == file {
			return e.Title
		}
	}
	return ""
}

// upsert заменяет первую строку файла новой (остальные дубли убирает) или
// добавляет строку в конец.
func (ix index) upsert(file, line string) string {
	lines := strings.Split(ix.before, ix.eol)
	replaced := false
	out := lines[:0:0]
	for _, l := range lines {
		if m := indexLine.FindStringSubmatch(l); m != nil && m[2] == file {
			if !replaced {
				out = append(out, line)
				replaced = true
			}
			continue
		}
		out = append(out, l)
	}
	if !replaced {
		return appendLine(ix.before, line, ix.eol)
	}
	return strings.Join(out, ix.eol)
}

func (ix index) remove(file string) string { return removeLines(ix.before, file) }

func removeLines(text, file string) string {
	eol := "\n"
	if strings.Contains(text, "\r\n") {
		eol = "\r\n"
	}
	lines := strings.Split(text, eol)
	out := lines[:0:0]
	for _, l := range lines {
		if m := indexLine.FindStringSubmatch(l); m != nil && m[2] == file {
			continue
		}
		out = append(out, l)
	}
	return strings.Join(out, eol)
}

func appendLine(text, line, eol string) string {
	if text != "" && !strings.HasSuffix(text, eol) {
		text += eol
	}
	return text + line + eol
}
