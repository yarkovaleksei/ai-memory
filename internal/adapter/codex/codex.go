// Package codex читает память Codex: memories_v2 (активное поколение) и, по
// настройке, заброшенное memories (v1, только чтение).
//
// Формат описан в plan/storage-formats.md. Адаптер только читает диск;
// SQLite-базы Codex в фазе 1 не нужны и не открываются.
package codex

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/yarkovaleksei/ai-memory/internal/adapter"
	"github.com/yarkovaleksei/ai-memory/internal/markdown"
	"github.com/yarkovaleksei/ai-memory/internal/model"
)

const (
	dirV2       = "memories_v2"
	dirV1       = "memories"
	summaryFile = "memory_summary.md"
	memoryFile  = "MEMORY.md"       // только v1
	rawFile     = "raw_memories.md" // только v1
	rolloutsDir = "rollout_summaries"
	whatsInMem  = "What's in Memory"
)

var notesDir = filepath.Join("extensions", "ad_hoc", "notes")

// Adapter читает память Codex из CODEX_HOME.
type Adapter struct {
	Home         string     // корень Codex
	Generation   Generation // активное поколение из настроек ai-memory; пусто — определить (DetectGeneration)
	ShowInactive bool       // показывать неактивное поколение (только чтение)
}

var _ adapter.Adapter = Adapter{}

// Active — поколение, в которое Codex сейчас пишет память, и причина выбора.
func (a Adapter) Active(ctx context.Context) (Generation, string) {
	if a.Generation != "" {
		return a.Generation, ReasonSettings
	}
	return DetectGeneration(ctx, filepath.FromSlash(a.Home))
}

// Discover возвращает активное поколение и, если включено, неактивное — только
// для чтения: заметку в нём Codex не прочитает никогда (Q3).
func (a Adapter) Discover(ctx context.Context) ([]model.Source, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	home := filepath.FromSlash(a.Home)
	active, _ := a.Active(ctx)
	inactive := GenV1
	if active == GenV1 {
		inactive = GenV2
	}
	var out []model.Source
	if root := filepath.Join(home, active.Dir()); isDir(root) {
		out = append(out, model.Source{ID: "codex:" + root, Agent: model.AgentCodex, Label: "Codex " + string(active), Root: root})
	}
	if root := filepath.Join(home, inactive.Dir()); a.ShowInactive && isDir(root) {
		out = append(out, model.Source{ID: "codex:" + root, Agent: model.AgentCodex, Label: "Codex " + string(inactive), Root: root, ReadOnly: true})
	}
	return out, nil
}

// WatchDirs: корень поколения, rollout-сводки и ad-hoc-заметки.
func (a Adapter) WatchDirs(src model.Source) []string {
	var dirs []string
	for _, d := range []string{src.Root, filepath.Join(src.Root, rolloutsDir), filepath.Join(src.Root, notesDir)} {
		if isDir(d) {
			dirs = append(dirs, d)
		}
	}
	return dirs
}

// Scan читает сводку, rollout-сводки и заметки; для v1 ещё MEMORY.md и raw_memories.md.
func (a Adapter) Scan(ctx context.Context, src model.Source) ([]model.Item, error) {
	if !isDir(src.Root) {
		return nil, nil
	}
	s := scanner{src: src}

	rollouts, err := s.rollouts(ctx)
	if err != nil {
		return nil, err
	}
	byRel := map[string]*model.Item{}
	for i := range rollouts {
		byRel[adapter.RelPath(src.Root, rollouts[i].File)] = &rollouts[i]
	}

	var items []model.Item
	items = append(items, s.summary(byRel)...)
	if filepath.Base(src.Root) == dirV1 { // форматные отличия v1, а не признак «только чтение»
		items = append(items, s.memoryV1(byRel)...)
		items = append(items, s.raw()...)
	}
	items = append(items, rollouts...)
	notes, err := s.notes(ctx)
	if err != nil {
		return nil, err
	}
	items = append(items, notes...)
	return items, nil
}

type scanner struct {
	src model.Source
}

func (s scanner) write(m model.WriteMode) model.WriteMode {
	if s.src.ReadOnly {
		return model.WriteNone
	}
	return m
}

func (s scanner) base(path, anchor string, kind model.ItemKind, content string, fi fs.FileInfo) model.Item {
	return model.Item{
		ID:       model.ItemID(s.src.ID, adapter.RelPath(s.src.Root, path), anchor),
		SourceID: s.src.ID,
		Agent:    model.AgentCodex,
		Kind:     kind,
		File:     path,
		Anchor:   anchor,
		ModTime:  fi.ModTime(),
		Hash:     model.ContentHash([]byte(content)),
	}
}

var (
	projectLine = regexp.MustCompile("Project:\\s*`([^`]+)`")
	cwdInline   = regexp.MustCompile(`cwd=([^,\s)]+)`)
	rolloutRef  = regexp.MustCompile(`rollout_summaries/[^\s)` + "`" + `,]+\.md`)
)

// summary режет memory_summary.md на разделы «## …», а «What's in Memory» —
// дальше на группы «### …»: у каждой группы свой проект.
func (s scanner) summary(rollouts map[string]*model.Item) []model.Item {
	path := filepath.Join(s.src.Root, summaryFile)
	content, fi, err := readFile(path)
	if err != nil {
		return nil
	}
	var out []model.Item
	for _, sec := range markdown.Sections(content, 2) {
		if sec.Heading == "" {
			continue // строка версии «v1» до первого раздела
		}
		if sec.Heading != whatsInMem {
			it := s.base(path, sec.Heading, model.KindCodexSection, content, fi)
			it.Title, it.Body = sec.Heading, sec.Body
			it.Description = markdown.FirstParagraph(sec.Body, 200)
			it.Write = s.write(model.WriteViaNote)
			it.Project = model.ProjectRef{Global: true}
			out = append(out, it)
			continue
		}
		for _, group := range markdown.Sections(sec.Body, 3) {
			if group.Heading == "" {
				continue
			}
			anchor := whatsInMem + "/" + group.Heading
			it := s.base(path, anchor, model.KindCodexSection, content, fi)
			it.Title, it.Body = group.Heading, group.Body
			it.Description = firstListItem(group.Body)
			it.Write = s.write(model.WriteViaNote)
			it.Project = projectOf(group.Body, rollouts)
			it.Links = rolloutLinks(group.Body, rollouts)
			out = append(out, it)
		}
	}
	return out
}

// memoryV1 — MEMORY.md поколения v1, по разделу на «# Task Group».
func (s scanner) memoryV1(rollouts map[string]*model.Item) []model.Item {
	path := filepath.Join(s.src.Root, memoryFile)
	content, fi, err := readFile(path)
	if err != nil {
		return nil
	}
	var out []model.Item
	for _, sec := range markdown.Sections(content, 1) {
		if sec.Heading == "" {
			continue
		}
		it := s.base(path, sec.Heading, model.KindCodexSection, content, fi)
		it.Title = strings.TrimPrefix(sec.Heading, "Task Group: ")
		it.Body = sec.Body
		it.Description = markdown.FirstParagraph(sec.Body, 200)
		it.Write = model.WriteNone
		it.Project = projectOf(sec.Body, rollouts)
		it.Links = rolloutLinks(sec.Body, rollouts)
		out = append(out, it)
	}
	return out
}

func (s scanner) raw() []model.Item {
	path := filepath.Join(s.src.Root, rawFile)
	content, fi, err := readFile(path)
	if err != nil {
		return nil
	}
	it := s.base(path, "", model.KindCodexRaw, content, fi)
	it.Title = rawFile
	it.Body = markdown.Normalize(content)
	it.Description = markdown.FirstParagraph(content, 200)
	it.Write = model.WriteNone
	it.Project = model.ProjectRef{Global: true}
	return []model.Item{it}
}

var rolloutName = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}-\d{2}-\d{2}-[A-Za-z0-9]+-(.+)\.md$`)

func (s scanner) rollouts(ctx context.Context) ([]model.Item, error) {
	dir := filepath.Join(s.src.Root, rolloutsDir)
	names, err := mdFiles(dir)
	if err != nil {
		return nil, err
	}
	var out []model.Item
	for _, name := range names {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		path := filepath.Join(dir, name)
		content, fi, err := readFile(path)
		if err != nil {
			continue
		}
		header, body := splitHeader(content)
		it := s.base(path, "", model.KindCodexRollout, content, fi)
		it.Title = rolloutTitle(name)
		it.Body = body
		it.Description = markdown.FirstParagraph(body, 200)
		it.Write = s.write(model.WriteViaNote)
		fm := map[string]any{}
		for k, v := range header {
			fm[k] = v
		}
		it.Frontmatter = fm
		if cwd := header["cwd"]; cwd != "" {
			p := filepath.Clean(cwd)
			it.Project = model.ProjectRef{Path: p, Temp: adapter.IsTempPath(p)}
		} else {
			it.Project = model.ProjectRef{Global: true}
		}
		out = append(out, it)
	}
	return out, nil
}

func (s scanner) notes(ctx context.Context) ([]model.Item, error) {
	dir := filepath.Join(s.src.Root, notesDir)
	names, err := mdFiles(dir)
	if err != nil {
		return nil, err
	}
	// Статус консолидации нужен только там, куда пишем: в неактивном поколении
	// заметку Codex не учтёт никогда.
	var pending map[string]bool
	if !s.src.ReadOnly && len(names) > 0 {
		pending, _ = PendingNotes(ctx, s.src.Root)
	}
	var out []model.Item
	for _, name := range names {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		path := filepath.Join(dir, name)
		content, fi, err := readFile(path)
		if err != nil {
			continue
		}
		it := s.base(path, "", model.KindCodexAdHocNote, content, fi)
		it.Body = markdown.Normalize(content)
		it.Title = markdown.FirstHeading(content)
		if it.Title == "" {
			it.Title = strings.TrimSuffix(name, filepath.Ext(name))
		}
		it.Description = markdown.FirstParagraph(content, 200)
		it.Write = s.write(model.WriteDirect)
		it.Pending = pending[path]
		it.Project = model.ProjectRef{Global: true}
		out = append(out, it)
	}
	return out, nil
}

// splitHeader разбирает заголовок rollout-сводки «ключ: значение» до первой пустой строки.
func splitHeader(content string) (map[string]string, string) {
	content = markdown.Normalize(content)
	header := map[string]string{}
	lines := strings.Split(content, "\n")
	i := 0
	for ; i < len(lines); i++ {
		line := lines[i]
		if strings.TrimSpace(line) == "" {
			break
		}
		k, v, ok := strings.Cut(line, ":")
		if !ok || strings.ContainsAny(k, " \t") {
			return map[string]string{}, content // не заголовок — тело целиком
		}
		header[k] = strings.TrimSpace(v)
	}
	return header, strings.TrimLeft(strings.Join(lines[i:], "\n"), "\n")
}

func rolloutTitle(name string) string {
	if m := rolloutName.FindStringSubmatch(name); m != nil {
		return strings.ReplaceAll(m[1], "_", " ")
	}
	return strings.TrimSuffix(name, filepath.Ext(name))
}

// projectOf: явная строка «Project: `путь`», иначе cwd первой упомянутой
// rollout-сводки, иначе «cwd=…» в тексте; не нашлось — глобальная память.
func projectOf(text string, rollouts map[string]*model.Item) model.ProjectRef {
	if m := projectLine.FindStringSubmatch(text); m != nil {
		p := filepath.Clean(m[1])
		return model.ProjectRef{Path: p, Temp: adapter.IsTempPath(p)}
	}
	for _, ref := range rolloutRef.FindAllString(text, -1) {
		if r, ok := rollouts[ref]; ok && r.Project.Path != "" {
			return r.Project
		}
	}
	if m := cwdInline.FindStringSubmatch(text); m != nil {
		p := filepath.Clean(m[1])
		return model.ProjectRef{Path: p, Temp: adapter.IsTempPath(p)}
	}
	return model.ProjectRef{Global: true}
}

func rolloutLinks(text string, rollouts map[string]*model.Item) []model.Link {
	var out []model.Link
	seen := map[string]bool{}
	for _, ref := range rolloutRef.FindAllString(text, -1) {
		if seen[ref] {
			continue
		}
		seen[ref] = true
		l := model.Link{Text: ref}
		if r, ok := rollouts[ref]; ok {
			l.TargetID = r.ID
		}
		out = append(out, l)
	}
	return out
}

func firstListItem(text string) string {
	for _, line := range strings.Split(text, "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "- ") {
			return markdown.FirstParagraph(strings.TrimPrefix(t, "- "), 200)
		}
	}
	return markdown.FirstParagraph(text, 200)
}

func mdFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("codex: чтение %s: %w", dir, err)
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.EqualFold(filepath.Ext(e.Name()), ".md") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	return names, nil
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

func isDir(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}
