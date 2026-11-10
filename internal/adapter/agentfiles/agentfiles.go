// Package agentfiles читает инструкции и навыки агентов (ADR-010, ADR-019):
// CLAUDE.md, AGENTS.md и правила, каталоги навыков с SKILL.md — в профилях
// Claude, в CODEX_HOME, в общем ~/.agents и в каталогах проектов.
//
// Что и где читает каждый агент — plan/storage-formats.md, «Инструкции и
// навыки». Адаптер только читает диск.
package agentfiles

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/yarkovaleksei/ai-memory/internal/adapter"
	"github.com/yarkovaleksei/ai-memory/internal/markdown"
	"github.com/yarkovaleksei/ai-memory/internal/model"
)

// Префиксы идентификаторов хранилищ: по ним Scan понимает, что читать.
const (
	prefixClaude   = "claude-files:"
	prefixCodex    = "codex-files:"
	prefixAgents   = "agents-files:"
	ClaudeProjects = "claude-projects"
	CodexProjects  = "codex-projects"
)

// Роли записей (model.Item.Role).
const (
	RoleMain     = "main"     // CLAUDE.md, AGENTS.md
	RoleLocal    = "local"    // CLAUDE.local.md
	RoleOverride = "override" // AGENTS.override.md
	RoleRule     = "rule"     // rules/**/*.md
	RoleUser     = "user"     // навык профиля или ~/.agents
	RoleProject  = "project"  // навык в каталоге проекта
	RoleSystem   = "system"   // системный навык Codex
	RoleSynced   = "synced"   // навык, синхронизированный с аккаунтом claude.ai
)

// Имена файлов инструкций.
const (
	ClaudeMD      = "CLAUDE.md"
	ClaudeLocalMD = "CLAUDE.local.md"
	AgentsMD      = "AGENTS.md"
	AgentsOverMD  = "AGENTS.override.md"
)

// Adapter читает инструкции и навыки.
type Adapter struct {
	ClaudeProfiles []string // профили Claude из config
	CodexHome      string
	AgentsDir      string   // ~/.agents
	Projects       []string // пути проектов, известные из памяти; проставляет библиотека
}

var _ adapter.Adapter = Adapter{}

// Discover: хранилище файлов на каждый профиль Claude, на CODEX_HOME, на
// ~/.agents и по одному на проектные файлы каждого агента.
func (a Adapter) Discover(ctx context.Context) ([]model.Source, error) {
	var out []model.Source
	seen := map[string]bool{}
	for _, p := range a.ClaudeProfiles {
		root := filepath.FromSlash(p)
		if !adapter.IsDir(root) {
			continue
		}
		// Один профиль под двумя именами читается один раз, как и память.
		key := realKey(root)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, files(prefixClaude+root, model.AgentClaude, filepath.Base(root), root))
	}
	if home := filepath.FromSlash(a.CodexHome); home != "" && adapter.IsDir(home) {
		out = append(out, files(prefixCodex+home, model.AgentCodex, filepath.Base(home), home))
	}
	if dir := filepath.FromSlash(a.AgentsDir); dir != "" && adapter.IsDir(filepath.Join(dir, "skills")) {
		out = append(out, files(prefixAgents+dir, model.AgentCodex, filepath.Base(dir), dir))
	}
	if len(a.projects()) > 0 {
		out = append(out,
			files(ClaudeProjects, model.AgentClaude, "", ""),
			files(CodexProjects, model.AgentCodex, "", ""))
	}
	return out, ctx.Err()
}

// ClaudeFilesID и CodexFilesID — ID хранилищ глобальных инструкций профиля
// Claude и CODEX_HOME с корнем root.
func ClaudeFilesID(root string) string { return prefixClaude + root }
func CodexFilesID(root string) string  { return prefixCodex + root }

func files(id string, agent model.Agent, label, root string) model.Source {
	return model.Source{ID: id, Agent: agent, Label: label, Root: root, Files: true}
}

// projects — существующие каталоги проектов без повторов.
func (a Adapter) projects() []string {
	seen := map[string]bool{}
	var out []string
	for _, p := range a.Projects {
		p = filepath.Clean(filepath.FromSlash(p))
		k := adapter.PathKey(p)
		if seen[k] || !adapter.IsDir(p) {
			continue
		}
		seen[k] = true
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

// Scan читает одно хранилище.
func (a Adapter) Scan(ctx context.Context, src model.Source) ([]model.Item, error) {
	global := model.ProjectRef{Global: true}
	var items []model.Item
	switch {
	case strings.HasPrefix(src.ID, prefixClaude):
		root := src.Root
		items = append(items, instructions(src, root, root, global, "", ClaudeMD, RoleMain)...)
		items = append(items, rules(src, root, filepath.Join(root, "rules"), global, "")...)
		items = append(items, skills(src, filepath.Join(root, "skills"), RoleUser, model.WriteDirect, global)...)
		// Навыки аккаунта claude.ai: <профиль>/skills/synced/<bucket>/<навык>.
		// Их ведёт Claude — только чтение.
		synced := filepath.Join(root, "skills", "synced")
		buckets, _ := os.ReadDir(synced)
		for _, b := range buckets {
			if b.IsDir() && !strings.HasPrefix(b.Name(), ".") {
				items = append(items, skills(src, filepath.Join(synced, b.Name()), RoleSynced, model.WriteNone, global)...)
			}
		}
	case strings.HasPrefix(src.ID, prefixCodex):
		root := src.Root
		items = append(items, codexInstructions(src, root, root, global, "", true)...)
		items = append(items, skills(src, filepath.Join(root, "skills"), RoleUser, model.WriteDirect, global)...)
		// Системные навыки Codex перезаписывает при каждом запуске.
		items = append(items, skills(src, filepath.Join(root, "skills", ".system"), RoleSystem, model.WriteNone, global)...)
	case strings.HasPrefix(src.ID, prefixAgents):
		items = append(items, skills(src, filepath.Join(src.Root, "skills"), RoleUser, model.WriteDirect, global)...)
	case src.ID == ClaudeProjects:
		for _, p := range a.projects() {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			ref := model.ProjectRef{Path: p}
			repo := repoRoot(p)
			items = append(items, instructions(src, p, p, ref, repo, ClaudeMD, RoleMain)...)
			items = append(items, instructions(src, p, filepath.Join(p, ".claude"), ref, repo, ClaudeMD, RoleMain)...)
			items = append(items, instructions(src, p, p, ref, repo, ClaudeLocalMD, RoleLocal)...)
			items = append(items, rules(src, p, filepath.Join(p, ".claude", "rules"), ref, repo)...)
			items = append(items, withRepo(skills(src, filepath.Join(p, ".claude", "skills"), RoleProject, model.WriteDirect, ref), repo)...)
		}
	case src.ID == CodexProjects:
		for _, p := range a.projects() {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			ref := model.ProjectRef{Path: p}
			repo := repoRoot(p)
			items = append(items, codexInstructions(src, p, p, ref, repo, false)...)
			items = append(items, withRepo(skills(src, filepath.Join(p, ".agents", "skills"), RoleProject, model.WriteDirect, ref), repo)...)
			items = append(items, withRepo(skills(src, filepath.Join(p, ".codex", "skills"), RoleProject, model.WriteDirect, ref), repo)...)
		}
	}
	return items, ctx.Err()
}

// instructions читает один файл инструкций, если он есть. base — от чего
// считается подпись: корень профиля или каталог проекта.
func instructions(src model.Source, base, dir string, ref model.ProjectRef, repo, name, role string) []model.Item {
	it, ok := readInstructions(src, base, filepath.Join(dir, name), ref, repo, role)
	if !ok {
		return nil
	}
	return []model.Item{it}
}

// codexInstructions — AGENTS.override.md и AGENTS.md одного каталога. Codex
// берёт из каталога один файл: override, если он есть. На глобальном уровне
// пустой override не считается, в каталоге проекта — считается
// (storage-formats.md, «Инструкции Codex»).
func codexInstructions(src model.Source, base, dir string, ref model.ProjectRef, repo string, global bool) []model.Item {
	over, hasOver := readInstructions(src, base, filepath.Join(dir, AgentsOverMD), ref, repo, RoleOverride)
	main, hasMain := readInstructions(src, base, filepath.Join(dir, AgentsMD), ref, repo, RoleMain)
	var out []model.Item
	if hasOver {
		out = append(out, over)
	}
	if hasMain {
		if hasOver && (!global || strings.TrimSpace(over.Body) != "" || over.Frontmatter != nil) {
			main.Warnings = append(main.Warnings, model.NewWarning("codex.instructions.shadowed", "file", AgentsOverMD))
		}
		out = append(out, main)
	}
	return out
}

func readInstructions(src model.Source, base, file string, ref model.ProjectRef, repo, role string) (model.Item, bool) {
	fi, err := os.Stat(file)
	if err != nil || !fi.Mode().IsRegular() {
		return model.Item{}, false
	}
	data, err := os.ReadFile(file)
	if err != nil {
		return model.Item{}, false
	}
	content := markdown.Normalize(string(data))
	it := model.Item{
		ID:       model.ItemID(src.ID, adapter.RelPath(src.Root, file), ""),
		SourceID: src.ID,
		Agent:    src.Agent,
		Kind:     model.KindInstructions,
		Role:     role,
		Title:    adapter.RelPath(base, file),
		Body:     content,
		File:     file,
		ModTime:  fi.ModTime(),
		Hash:     model.ContentHash(data),
		Write:    model.WriteDirect,
		Repo:     repo,
		Project:  ref,
	}
	// У правил Claude бывает frontmatter с paths: правило грузится, только
	// когда открыты подходящие файлы.
	if fmText, body, has := markdown.SplitFrontmatter(content); has {
		if fm, err := markdown.ParseFrontmatter(fmText); err == nil {
			it.Frontmatter = fm
			it.Body = body
		} else {
			it.Warnings = append(it.Warnings, model.NewWarning("instructions.frontmatter.invalid", "error", err.Error()))
		}
	}
	it.Description = markdown.FirstHeading(it.Body)
	return it, true
}

// rules — правила Claude: все *.md в каталоге rules и его подкаталогах.
func rules(src model.Source, base, dir string, ref model.ProjectRef, repo string) []model.Item {
	var out []model.Item
	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if path != dir && strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.EqualFold(filepath.Ext(path), ".md") {
			if it, ok := readInstructions(src, base, path, ref, repo, RoleRule); ok {
				out = append(out, it)
			}
		}
		return nil
	})
	return out
}

func skills(src model.Source, root, role string, write model.WriteMode, ref model.ProjectRef) []model.Item {
	var out []model.Item
	for _, dir := range adapter.SkillDirs(root) {
		if it, ok := adapter.ReadSkill(src, dir, role, write, ref); ok {
			out = append(out, it)
		}
	}
	return out
}

func withRepo(items []model.Item, repo string) []model.Item {
	for i := range items {
		items[i].Repo = repo
	}
	return items
}

// repoRoot — ближайший сверху каталог с .git (файл или каталог: у worktree
// и подмодулей .git — файл). Пусто — файл не в git-репозитории.
func repoRoot(dir string) string {
	for d := filepath.Clean(dir); ; {
		if _, err := os.Lstat(filepath.Join(d, ".git")); err == nil {
			return d
		}
		parent := filepath.Dir(d)
		if parent == d {
			return ""
		}
		d = parent
	}
}

// realKey — путь с раскрытыми симлинками для сравнения.
func realKey(p string) string {
	if r, err := filepath.EvalSymlinks(p); err == nil {
		p = r
	}
	return adapter.PathKey(p)
}

// WatchDirs — каталоги с инструкциями и навыками. В корне профиля, CODEX_HOME
// и каталоге проекта агент меняет и много другого, поэтому там важны только
// имена из WatchNames.
func (a Adapter) WatchDirs(src model.Source) []string {
	names := a.WatchNames(src)
	dirs := make([]string, 0, len(names))
	for d := range names {
		dirs = append(dirs, d)
	}
	add := func(root string) {
		if !adapter.IsDir(root) {
			return
		}
		dirs = append(dirs, root)
		dirs = append(dirs, adapter.SkillDirs(root)...)
	}
	addTree := func(root string) {
		_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err == nil && d.IsDir() {
				dirs = append(dirs, path)
			}
			return nil
		})
	}
	switch {
	case strings.HasPrefix(src.ID, prefixClaude):
		addTree(filepath.Join(src.Root, "rules"))
		add(filepath.Join(src.Root, "skills"))
	case strings.HasPrefix(src.ID, prefixCodex):
		add(filepath.Join(src.Root, "skills"))
	case strings.HasPrefix(src.ID, prefixAgents):
		add(filepath.Join(src.Root, "skills"))
	case src.ID == ClaudeProjects:
		for _, p := range a.projects() {
			addTree(filepath.Join(p, ".claude", "rules"))
			add(filepath.Join(p, ".claude", "skills"))
		}
	case src.ID == CodexProjects:
		for _, p := range a.projects() {
			add(filepath.Join(p, ".agents", "skills"))
			add(filepath.Join(p, ".codex", "skills"))
		}
	}
	sort.Strings(dirs)
	return dirs
}

// WatchNames — каталоги, в которых важны только некоторые имена: файлы
// инструкций и каталоги, внутри которых они появятся.
func (a Adapter) WatchNames(src model.Source) map[string][]string {
	out := map[string][]string{}
	put := func(dir string, names ...string) {
		if adapter.IsDir(dir) {
			out[dir] = append(out[dir], names...)
		}
	}
	switch {
	case strings.HasPrefix(src.ID, prefixClaude):
		put(src.Root, ClaudeMD, "rules", "skills")
	case strings.HasPrefix(src.ID, prefixCodex):
		put(src.Root, AgentsMD, AgentsOverMD, "skills")
	case strings.HasPrefix(src.ID, prefixAgents):
		put(src.Root, "skills")
	case src.ID == ClaudeProjects:
		for _, p := range a.projects() {
			put(p, ClaudeMD, ClaudeLocalMD, ".claude")
			put(filepath.Join(p, ".claude"), ClaudeMD, "rules", "skills")
		}
	case src.ID == CodexProjects:
		for _, p := range a.projects() {
			put(p, AgentsMD, AgentsOverMD, ".agents", ".codex")
			put(filepath.Join(p, ".agents"), "skills")
			put(filepath.Join(p, ".codex"), "skills")
		}
	}
	return out
}

// MarkShared помечает навыки, на один каталог которых (после раскрытия
// симлинков) указывают несколько записей, и связывает их друг с другом:
// правка такого навыка затронет всех (ADR-010).
func MarkShared(items []model.Item) {
	groups := map[string][]int{}
	var order []string
	for i, it := range items {
		if it.Kind != model.KindSkill || it.RealDir == "" {
			continue
		}
		k := adapter.PathKey(it.RealDir)
		if _, ok := groups[k]; !ok {
			order = append(order, k)
		}
		groups[k] = append(groups[k], i)
	}
	for _, k := range order {
		idx := groups[k]
		if len(idx) < 2 {
			continue
		}
		for _, i := range idx {
			items[i].Shared = true
			for _, j := range idx {
				if i != j {
					items[i].Links = append(items[i].Links, model.Link{Text: filepath.Dir(items[j].File), TargetID: items[j].ID})
				}
			}
		}
	}
}
