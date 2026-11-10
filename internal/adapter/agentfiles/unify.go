package agentfiles

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/yarkovaleksei/ai-memory/internal/adapter"
	"github.com/yarkovaleksei/ai-memory/internal/markdown"
	"github.com/yarkovaleksei/ai-memory/internal/model"
	"github.com/yarkovaleksei/ai-memory/internal/writer"
)

// Приём «AGENTS.md — основной, CLAUDE.md импортирует его» (ADR-022): общие
// правила проекта живут в AGENTS.md, который читают оба агента, а в CLAUDE.md
// остаются импорт @AGENTS.md и то, что нужно только Claude Code.

// Состояния инструкций проекта для приёма.
const (
	UnifyBoth       = "both"        // есть оба файла, CLAUDE.md не импортирует AGENTS.md
	UnifyClaudeOnly = "claude-only" // есть только CLAUDE.md: AGENTS.md создаётся
	UnifyAgentsOnly = "agents-only" // есть только AGENTS.md: CLAUDE.md создаётся с импортом
	UnifyDone       = "done"        // CLAUDE.md уже импортирует AGENTS.md
	UnifyNone       = "none"        // инструкций нет ни у одного агента
)

// Block — кусок CLAUDE.md, который переносится в AGENTS.md целиком. Блоки
// идут подряд и покрывают весь файл: текст до первого заголовка и разделы
// верхнего уровня. Если весь файл — один раздел «# Заголовок», блоки — его
// вступление и его подразделы.
type Block struct {
	Index   int      `json:"index"`
	Level   int      `json:"level"`
	Heading string   `json:"heading"`
	Text    string   `json:"text"`
	Imports []string `json:"imports"`
	// Intro — заголовок документа с текстом до первого подраздела: обычно
	// остаётся в CLAUDE.md.
	Intro bool `json:"intro"`

	start, end int
}

// Blocks режет CLAUDE.md на блоки. prefix — frontmatter, если есть: он
// остаётся в CLAUDE.md.
func Blocks(content string) (prefix string, out []Block) {
	content = markdown.Normalize(content)
	secs := Sections(content)
	// Всё до первого блока — frontmatter и пустые строки: в блоки не входит.
	base := len(content)
	if len(secs) > 0 {
		base = secs[0].start
	}
	top := outermost(secs, nil)
	headed := 0
	for _, s := range top {
		if s.Level > 0 {
			headed++
		}
	}
	for _, s := range top {
		kids := outermost(secs, &s)
		if headed == 1 && s.Level > 0 && len(kids) > 0 {
			out = append(out, block(content, s, s.start, kids[0].start, true))
			for _, k := range kids {
				out = append(out, block(content, k, k.start, k.end, false))
			}
			continue
		}
		out = append(out, block(content, s, s.start, s.end, false))
	}
	for i := range out {
		out[i].Index = i
	}
	return content[:base], out
}

func block(content string, s Section, start, end int, intro bool) Block {
	text := strings.Trim(content[start:end], "\n")
	return Block{Level: s.Level, Heading: s.Heading, Text: text, Imports: Imports(text), Intro: intro, start: start, end: end}
}

// outermost — разделы, не вложенные ни в какой другой; внутри parent, если он задан.
func outermost(secs []Section, parent *Section) []Section {
	inside := func(a, b Section) bool {
		return a.Index != b.Index && b.start <= a.start && a.end <= b.end
	}
	var out []Section
	for _, s := range secs {
		if parent != nil && !inside(s, *parent) {
			continue
		}
		nested := false
		for _, o := range secs {
			if (parent == nil || o.Index != parent.Index) && inside(s, o) && (parent == nil || inside(o, *parent)) {
				nested = true
				break
			}
		}
		if !nested {
			out = append(out, s)
		}
	}
	return out
}

// UnifyState — инструкции проекта глазами приёма.
type UnifyState struct {
	State        string  `json:"state"`
	ClaudeFile   string  `json:"claudeFile"`
	AgentsFile   string  `json:"agentsFile"`
	ClaudeExists bool    `json:"claudeExists"`
	AgentsExists bool    `json:"agentsExists"`
	ClaudeHash   string  `json:"claudeHash"`
	AgentsHash   string  `json:"agentsHash"`
	Import       string  `json:"import"`   // строка импорта для CLAUDE.md: «@AGENTS.md» или «@../AGENTS.md»
	Override     bool    `json:"override"` // рядом AGENTS.override.md: Codex читает его, а не AGENTS.md
	Blocks       []Block `json:"blocks"`   // блоки CLAUDE.md

	claude, agents string // содержимое файлов
}

// ReadUnify смотрит инструкции проекта в каталоге dir. Главный файл Claude —
// CLAUDE.md в корне или, если его нет, .claude/CLAUDE.md.
func ReadUnify(dir string) (UnifyState, error) {
	st := UnifyState{
		ClaudeFile: filepath.Join(dir, ClaudeMD),
		AgentsFile: filepath.Join(dir, AgentsMD),
		Override:   adapter.IsFile(filepath.Join(dir, AgentsOverMD)),
		Blocks:     []Block{},
	}
	if alt := filepath.Join(dir, ".claude", ClaudeMD); !adapter.IsFile(st.ClaudeFile) && adapter.IsFile(alt) {
		st.ClaudeFile = alt
	}
	var err error
	if st.claude, st.ClaudeExists, st.ClaudeHash, err = readOptional(st.ClaudeFile); err != nil {
		return st, err
	}
	if st.agents, st.AgentsExists, st.AgentsHash, err = readOptional(st.AgentsFile); err != nil {
		return st, err
	}
	rel, err := filepath.Rel(filepath.Dir(st.ClaudeFile), st.AgentsFile)
	if err != nil {
		rel = AgentsMD
	}
	st.Import = "@" + filepath.ToSlash(rel)

	switch {
	case !st.ClaudeExists && !st.AgentsExists:
		st.State = UnifyNone
	case !st.ClaudeExists:
		st.State = UnifyAgentsOnly
	case importsFile(st.ClaudeFile, st.claude, st.AgentsFile):
		st.State = UnifyDone
	case !st.AgentsExists:
		st.State = UnifyClaudeOnly
	default:
		st.State = UnifyBoth
	}
	if st.ClaudeExists {
		_, st.Blocks = Blocks(st.claude)
	}
	return st, nil
}

func readOptional(file string) (content string, exists bool, hash string, err error) {
	data, err := os.ReadFile(file)
	if errors.Is(err, fs.ErrNotExist) {
		return "", false, "", nil
	}
	if err != nil {
		return "", false, "", writer.Fail("write.unreadable", "path", file, "error", err.Error())
	}
	return string(data), true, model.ContentHash(data), nil
}

// importsFile: в тексте from есть импорт, ведущий на target.
func importsFile(from, text, target string) bool {
	for _, imp := range Imports(text) {
		if adapter.PathKey(ResolveImport(from, imp)) == adapter.PathKey(target) {
			return true
		}
	}
	return false
}

// PlanUnify — план приёма для проекта dir: блоки move из CLAUDE.md уходят в
// конец AGENTS.md (или в новый AGENTS.md), в CLAUDE.md остаётся прочее и
// импорт. Нет CLAUDE.md — он создаётся из одного импорта. Хэши — из
// UnifyState, которую видел пользователь; пустой — файла не было.
func PlanUnify(dir, claudeHash, agentsHash string, move []int) (writer.Plan, error) {
	st, err := ReadUnify(dir)
	if err != nil {
		return writer.Plan{}, err
	}
	if st.ClaudeHash != claudeHash {
		return writer.Plan{}, writer.Fail("write.conflict", "path", st.ClaudeFile, "reason", "changed")
	}
	if st.AgentsHash != agentsHash {
		return writer.Plan{}, writer.Fail("write.conflict", "path", st.AgentsFile, "reason", "changed")
	}
	switch st.State {
	case UnifyDone:
		return writer.Plan{}, writer.Fail("unify.already", "file", st.ClaudeFile)
	case UnifyNone:
		return writer.Plan{}, writer.Fail("unify.nothing")
	}

	moved := map[int]bool{}
	for _, i := range move {
		if i < 0 || i >= len(st.Blocks) {
			return writer.Plan{}, writer.Fail("section.not-found")
		}
		moved[i] = true
	}
	if st.State == UnifyClaudeOnly && len(moved) == 0 {
		// Пустой AGENTS.md ничего не даёт ни одному агенту.
		return writer.Plan{}, writer.Fail("unify.nothing-to-move")
	}

	prefix, blocks := Blocks(st.claude)
	var toAgents, keep []string
	var imports []string
	importAt := 0 // после скольких оставшихся блоков встаёт импорт
	for _, b := range blocks {
		if moved[b.Index] {
			toAgents = append(toAgents, b.Text)
			imports = append(imports, b.Imports...)
			continue
		}
		// Импорт — сразу после заголовка документа, как в CLAUDE.md этого
		// репозитория; без заголовка — в начале файла.
		if b.Intro && len(keep) == 0 {
			importAt = 1
		}
		keep = append(keep, b.Text)
	}
	keep = append(keep[:importAt], append([]string{st.Import}, keep[importAt:]...)...)
	claude := join(append([]string{strings.TrimRight(prefix, "\n")}, keep...)...)

	var changes []writer.Change
	if len(toAgents) > 0 {
		after := join(append([]string{st.agents}, toAgents...)...)
		changes = append(changes, writer.Change{Path: st.AgentsFile, BeforeExists: st.AgentsExists, Before: st.agents,
			AfterExists: true, After: withLineEndings(st.agents, after)})
	}
	changes = append(changes, writer.Change{Path: st.ClaudeFile, BeforeExists: st.ClaudeExists, Before: st.claude,
		AfterExists: true, After: withLineEndings(st.claude, claude)})

	p := writer.NewPlan(writer.Op{Code: "instructions.unify", Params: map[string]string{
		"project": filepath.Base(dir), "moved": strconv.Itoa(len(toAgents)),
	}}, changes...)
	if len(imports) > 0 {
		sort.Strings(imports)
		p.Warnings = append(p.Warnings, writer.Warning{Code: WarnImports, Params: map[string]string{"imports": strings.Join(dedupe(imports), ", ")}})
	}
	if st.Override {
		p.Warnings = append(p.Warnings, writer.Warning{Code: WarnShadowed, Params: map[string]string{"file": AgentsMD, "override": AgentsOverMD}})
	}
	p.Warnings = append(p.Warnings, Warnings(model.Item{Repo: repoRoot(dir)})...)
	return p, nil
}

func dedupe(sorted []string) []string {
	var out []string
	for i, s := range sorted {
		if i == 0 || s != sorted[i-1] {
			out = append(out, s)
		}
	}
	return out
}
