// Package claude читает автопамять Claude Code: <профиль>/projects/<проект>/memory/*.md.
//
// Формат описан в plan/storage-formats.md. Адаптер только читает диск.
package claude

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"

	"github.com/yarkovaleksei/ai-memory/internal/adapter"
	"github.com/yarkovaleksei/ai-memory/internal/markdown"
	"github.com/yarkovaleksei/ai-memory/internal/model"
)

// IndexFile — индекс фактов, который Claude Code загружает в каждую сессию.
const IndexFile = "MEMORY.md"

// Adapter читает профили Claude Code.
type Adapter struct {
	Profiles []string // абсолютные пути профилей из config
}

var _ adapter.Adapter = Adapter{}

// Discover возвращает существующие профили.
func (a Adapter) Discover(ctx context.Context) ([]model.Source, error) {
	var out []model.Source
	seen := map[string]bool{}
	for _, p := range a.Profiles {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		root := filepath.FromSlash(p)
		if fi, err := os.Stat(root); err != nil || !fi.IsDir() {
			continue
		}
		// Один каталог под двумя именами (~/.claude — симлинк на ~/.claude-work,
		// и оба указаны в настройках) читается один раз, иначе все записи задвоятся.
		// Остаётся имя, указанное первым.
		key := realPath(root)
		if seen[key] {
			continue
		}
		seen[key] = true
		src := model.Source{
			ID:    "claude:" + root,
			Agent: model.AgentClaude,
			Label: filepath.Base(root),
			Root:  root,
		}
		if dir := autoMemoryDirectory(root); dir != "" {
			// Семантика настройки не проверена (storage-formats.md): честно сообщаем,
			// что память из этого каталога пока не читаем.
			src.Warnings = append(src.Warnings, model.NewWarning("claude.settings.auto-memory-directory", "dir", dir))
		}
		out = append(out, src)
	}
	return out, nil
}

// realPath — путь с раскрытыми симлинками для сравнения профилей. На macOS и
// Windows файловые системы по умолчанию нечувствительны к регистру.
func realPath(p string) string {
	if r, err := filepath.EvalSymlinks(p); err == nil {
		p = r
	}
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		p = strings.ToLower(p)
	}
	return p
}

// autoMemoryDirectory читает одноимённую настройку из settings.json профиля.
func autoMemoryDirectory(root string) string {
	data, err := os.ReadFile(filepath.Join(root, "settings.json"))
	if err != nil {
		return ""
	}
	var s struct {
		AutoMemoryDirectory string `json:"autoMemoryDirectory"`
	}
	if json.Unmarshal(data, &s) != nil {
		return ""
	}
	return s.AutoMemoryDirectory
}

// WatchDirs: каталог projects, каждый каталог проекта (там появляются memory/ и
// файлы сессий) и каждый memory/.
func (a Adapter) WatchDirs(src model.Source) []string {
	projects := filepath.Join(src.Root, "projects")
	dirs := []string{projects}
	entries, _ := os.ReadDir(projects)
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		p := filepath.Join(projects, e.Name())
		dirs = append(dirs, p)
		if fi, err := os.Stat(filepath.Join(p, "memory")); err == nil && fi.IsDir() {
			dirs = append(dirs, filepath.Join(p, "memory"))
		}
	}
	return dirs
}

// Scan читает память всех проектов профиля.
func (a Adapter) Scan(ctx context.Context, src model.Source) ([]model.Item, error) {
	projects := filepath.Join(src.Root, "projects")
	entries, err := os.ReadDir(projects)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("claude: чтение %s: %w", projects, err)
	}
	var items []model.Item
	for _, e := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if !e.IsDir() {
			continue
		}
		projDir := filepath.Join(projects, e.Name())
		memDir := filepath.Join(projDir, "memory")
		if fi, err := os.Stat(memDir); err != nil || !fi.IsDir() {
			continue
		}
		ref := ResolveProject(projDir, e.Name())
		got, err := scanMemoryDir(src, memDir, ref)
		if err != nil {
			return nil, err
		}
		items = append(items, got...)
	}
	return items, nil
}

// indexEntry — строка MEMORY.md вида «- [Заголовок](файл.md) — зацепка».
type indexEntry struct {
	Title, File, Hook string
}

var indexLine = regexp.MustCompile(`^\s*[-*]\s*\[([^\]]+)\]\(([^)\s]+)\)\s*(?:[—–-]+\s*(.*))?$`)

func parseIndex(text string) []indexEntry {
	var out []indexEntry
	for _, line := range strings.Split(markdown.Normalize(text), "\n") {
		if m := indexLine.FindStringSubmatch(line); m != nil {
			out = append(out, indexEntry{Title: strings.TrimSpace(m[1]), File: m[2], Hook: strings.TrimSpace(m[3])})
		}
	}
	return out
}

func scanMemoryDir(src model.Source, memDir string, ref model.ProjectRef) ([]model.Item, error) {
	entries, err := os.ReadDir(memDir)
	if err != nil {
		return nil, fmt.Errorf("claude: чтение %s: %w", memDir, err)
	}

	var (
		index     []indexEntry
		indexItem *model.Item
		facts     []model.Item
		present   = map[string]bool{}
	)
	if content, fi, err := readFile(filepath.Join(memDir, IndexFile)); err == nil {
		index = parseIndex(content)
		path := filepath.Join(memDir, IndexFile)
		it := model.Item{
			ID:       model.ItemID(src.ID, adapter.RelPath(src.Root, path), ""),
			SourceID: src.ID,
			Agent:    model.AgentClaude,
			Kind:     model.KindClaudeIndex,
			Title:    IndexFile,
			Body:     markdown.Normalize(content),
			File:     path,
			ModTime:  fi.ModTime(),
			Hash:     model.ContentHash([]byte(content)),
			Write:    model.WriteNone,
			Project:  ref,
		}
		indexItem = &it
	}
	byFile := map[string]indexEntry{}
	for _, e := range index {
		byFile[e.File] = e
	}

	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || name == IndexFile || !strings.EqualFold(filepath.Ext(name), ".md") {
			continue
		}
		path := filepath.Join(memDir, name)
		content, fi, err := readFile(path)
		if err != nil {
			continue // файл исчез между ReadDir и чтением — пересборка подхватит
		}
		present[name] = true
		facts = append(facts, parseFact(src, path, name, content, fi, ref, byFile))
	}

	// Сверка индекса с файлами.
	if indexItem != nil {
		for _, e := range index {
			if !present[e.File] {
				indexItem.Warnings = append(indexItem.Warnings, model.NewWarning("claude.index.missing-file", "file", e.File))
			}
		}
	}
	for i := range facts {
		base := filepath.Base(facts[i].File)
		if indexItem != nil {
			if _, ok := byFile[base]; !ok {
				facts[i].Warnings = append(facts[i].Warnings, model.NewWarning("claude.index.not-listed"))
			}
		}
	}

	resolveLinks(facts, indexItem, index)

	items := facts
	if indexItem != nil {
		items = append([]model.Item{*indexItem}, facts...)
	}
	return items, nil
}

func parseFact(src model.Source, path, name, content string, fi fs.FileInfo, ref model.ProjectRef, byFile map[string]indexEntry) model.Item {
	fmText, body, has := markdown.SplitFrontmatter(content)
	it := model.Item{
		ID:       model.ItemID(src.ID, adapter.RelPath(src.Root, path), ""),
		SourceID: src.ID,
		Agent:    model.AgentClaude,
		Kind:     model.KindClaudeFact,
		Body:     strings.TrimLeft(body, "\n"),
		File:     path,
		ModTime:  fi.ModTime(),
		Hash:     model.ContentHash([]byte(content)),
		Write:    model.WriteDirect,
		Project:  ref,
	}
	var fm map[string]any
	switch {
	case !has:
		it.Warnings = append(it.Warnings, model.NewWarning("claude.frontmatter.missing"))
	default:
		parsed, err := markdown.ParseFrontmatter(fmText)
		if err != nil {
			it.Warnings = append(it.Warnings, model.NewWarning("claude.frontmatter.invalid", "error", err.Error()))
		} else {
			fm = parsed
		}
	}
	it.Frontmatter = fm

	it.Type = markdown.String(fm, "metadata", "type")
	if it.Type == "" {
		it.Type = markdown.String(fm, "type") // старый формат
	}
	entry, listed := byFile[name]
	switch {
	case listed && entry.Title != "":
		it.Title = entry.Title
	case markdown.String(fm, "name") != "":
		it.Title = markdown.String(fm, "name")
	default:
		it.Title = strings.TrimSuffix(name, filepath.Ext(name))
	}
	it.Description = markdown.String(fm, "description")
	if it.Description == "" && listed {
		it.Description = entry.Hook
	}
	for _, slug := range adapter.WikiLinks(it.Body) {
		it.Links = append(it.Links, model.Link{Text: slug})
	}
	return it
}

// resolveLinks находит цели [[slug]] среди фактов того же каталога памяти:
// по полю name во frontmatter или по имени файла без расширения.
func resolveLinks(facts []model.Item, indexItem *model.Item, index []indexEntry) {
	bySlug := map[string]string{}
	byFile := map[string]string{}
	for _, f := range facts {
		base := filepath.Base(f.File)
		byFile[base] = f.ID
		bySlug[strings.TrimSuffix(base, filepath.Ext(base))] = f.ID
	}
	for _, f := range facts {
		if n := markdown.String(f.Frontmatter, "name"); n != "" {
			bySlug[n] = f.ID
		}
	}
	for i := range facts {
		for j := range facts[i].Links {
			facts[i].Links[j].TargetID = bySlug[facts[i].Links[j].Text]
		}
	}
	if indexItem != nil {
		for _, e := range index {
			indexItem.Links = append(indexItem.Links, model.Link{Text: e.File, TargetID: byFile[e.File]})
		}
	}
}

func readFile(path string) (string, fs.FileInfo, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return "", nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", nil, err
	}
	return string(data), fi, nil
}

// sortedJSONL — файлы сессий проекта в стабильном порядке.
func sortedJSONL(projDir string) []string {
	matches, _ := filepath.Glob(filepath.Join(projDir, "*.jsonl"))
	sort.Strings(matches)
	return matches
}
