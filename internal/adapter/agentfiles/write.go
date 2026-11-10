package agentfiles

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/yarkovaleksei/ai-memory/internal/adapter"
	"github.com/yarkovaleksei/ai-memory/internal/markdown"
	"github.com/yarkovaleksei/ai-memory/internal/model"
	"github.com/yarkovaleksei/ai-memory/internal/writer"
)

// Коды предупреждений плана (ADR-010, ADR-020).
const (
	// WarnInRepo — файл лежит в git-репозитории проекта: правка появится в его
	// git status, коммитить её — дело владельца репозитория.
	WarnInRepo = "plan.in-repo"
	// WarnSharedSkill — каталог навыка подключён в нескольких местах или
	// симлинком: правка затронет всех, кто на него ссылается.
	WarnSharedSkill = "plan.shared-skill"
)

// PlanUpdate заменяет файл инструкций или SKILL.md навыка целиком. hash —
// ContentHash, который видел пользователь: если файл с тех пор изменился,
// план не строится.
func PlanUpdate(it model.Item, hash, content string) (writer.Plan, error) {
	if it.Write != model.WriteDirect || (it.Kind != model.KindInstructions && it.Kind != model.KindSkill) {
		return writer.Plan{}, writer.Fail("plan.readonly", "title", it.Title)
	}
	current, err := readExpected(it.File, hash)
	if err != nil {
		return writer.Plan{}, err
	}
	if content == current {
		return writer.Plan{}, writer.Fail("write.no-changes")
	}
	op := writer.Op{Code: "instructions.update", Params: map[string]string{"file": it.Title, "agent": string(it.Agent)}}
	if it.Kind == model.KindSkill {
		op = writer.Op{Code: "skill.update", Params: map[string]string{"name": it.Title, "agent": string(it.Agent)}}
	}
	p := writer.NewPlan(op, writer.Change{Path: it.File, BeforeExists: true, Before: current, AfterExists: true, After: content})
	p.Warnings = Warnings(it)
	return p, nil
}

// Warnings — предупреждения для плана, который меняет файлы записи.
func Warnings(it model.Item) []writer.Warning {
	var out []writer.Warning
	if it.Repo != "" {
		out = append(out, writer.Warning{Code: WarnInRepo, Params: map[string]string{"repo": it.Repo}})
	}
	if it.Kind == model.KindSkill && (it.Shared || it.Target != "") {
		dir := it.Target
		if dir == "" {
			dir = filepath.Dir(it.File)
		}
		out = append(out, writer.Warning{Code: WarnSharedSkill, Params: map[string]string{"dir": dir}, Confirm: true})
	}
	return out
}

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

// Коды предупреждений плана переноса раздела (ADR-021).
const (
	// WarnImports — в разделе есть импорты «@путь», а Codex их не понимает.
	WarnImports = "plan.section-imports"
	// WarnImported — файл Claude уже импортирует исходный файл через «@»:
	// после вставки Claude увидит правило дважды.
	WarnImported = "plan.section-imported"
	// WarnHeadingExists — в целевом файле уже есть раздел с тем же заголовком.
	WarnHeadingExists = "plan.section-exists"
	// WarnShadowed — Codex не читает целевой AGENTS.md: рядом AGENTS.override.md.
	WarnShadowed = "plan.target-shadowed"
	// WarnConditional — целевое правило Claude грузится только для файлов из paths.
	WarnConditional = "plan.target-conditional"
	// WarnCodexSize — инструкции Codex длиннее, чем он прочитает.
	WarnCodexSize = "plan.codex-size"
	// WarnImportsAgents — новый CLAUDE.md выключил бы чтение AGENTS.md
	// Claude Code, поэтому начинается с импорта «@AGENTS.md».
	WarnImportsAgents = "plan.claude-imports-agents"
)

// CodexInstructionsLimit — сколько байт инструкций Codex читает по умолчанию
// (project_doc_max_bytes, storage-formats.md).
const CodexInstructionsLimit = 32 * 1024

// SectionCopy — что и куда переносится.
type SectionCopy struct {
	From     model.Item // исходный файл инструкций
	FromHash string     // хэш, при котором пользователь выбрал раздел
	Section  int        // Section.Index в исходном файле
	To       model.Item // целевой файл; пустой File — создать NewFile
	ToHash   string
	NewFile  string      // путь нового файла, если To.File пуст
	NewAgent model.Agent // чей это новый файл
	After    int         // Section.Index в целевом файле; < 0 — в конец
	Move     bool        // убрать раздел из исходного файла
}

// PlanSection — вставка раздела инструкций в другой файл и, при переносе,
// удаление его из исходного. Одна операция: откат возвращает оба файла.
func PlanSection(c SectionCopy) (writer.Plan, error) {
	if c.From.Kind != model.KindInstructions {
		return writer.Plan{}, writer.Fail("plan.readonly", "title", c.From.Title)
	}
	raw, err := readExpected(c.From.File, c.FromHash)
	if err != nil {
		return writer.Plan{}, err
	}
	secs := Sections(raw)
	if c.Section < 0 || c.Section >= len(secs) {
		return writer.Plan{}, writer.Fail("section.not-found")
	}
	sec := secs[c.Section]

	to, agent := c.To, c.To.Agent
	var change writer.Change
	before := "" // текущее содержимое цели
	if to.File == "" {
		if c.NewFile == "" {
			return writer.Plan{}, writer.Fail("section.target-invalid")
		}
		if _, err := os.Lstat(c.NewFile); err == nil {
			return writer.Plan{}, writer.Fail("write.conflict", "path", c.NewFile, "reason", "created")
		}
		agent = c.NewAgent
		to = model.Item{Kind: model.KindInstructions, Agent: agent, File: c.NewFile,
			Title: filepath.Base(c.NewFile), Repo: repoRoot(filepath.Dir(c.NewFile))}
		start := ""
		if keepsAgentsMD(c.NewFile) {
			start = "@" + AgentsMD + "\n"
		}
		after, _ := InsertSection(start, sec.Text, -1)
		change = writer.Change{Path: c.NewFile, AfterExists: true, After: after}
	} else {
		if to.Kind != model.KindInstructions || to.Write != model.WriteDirect {
			return writer.Plan{}, writer.Fail("plan.readonly", "title", to.Title)
		}
		if adapter.PathKey(to.File) == adapter.PathKey(c.From.File) {
			return writer.Plan{}, writer.Fail("section.same-file")
		}
		before, err = readExpected(to.File, c.ToHash)
		if err != nil {
			return writer.Plan{}, err
		}
		after, ok := InsertSection(before, sec.Text, c.After)
		if !ok {
			return writer.Plan{}, writer.Fail("section.not-found")
		}
		change = writer.Change{Path: to.File, BeforeExists: true, Before: before, AfterExists: true,
			After: withLineEndings(before, after)}
	}

	code := "instructions.section-copy"
	if c.Move {
		code = "instructions.section-move"
	}
	heading := sec.Heading
	if heading == "" {
		heading = markdown.FirstParagraph(sec.Text, 60)
	}
	op := writer.Op{Code: code, Params: map[string]string{
		"heading": heading, "from": c.From.Title, "to": to.Title, "agent": string(agent),
	}}
	changes := []writer.Change{change}
	if c.Move {
		rest, _ := RemoveSection(raw, c.Section)
		changes = append(changes, writer.Change{Path: c.From.File, BeforeExists: true, Before: raw,
			AfterExists: true, After: withLineEndings(raw, rest)})
	}
	p := writer.NewPlan(op, changes...)
	p.Warnings = sectionWarnings(c, sec, to, agent, before, change.After)
	return p, nil
}

// keepsAgentsMD: Claude Code сам читает AGENTS.md проекта, только пока рядом
// нет CLAUDE.md и CLAUDE.local.md (storage-formats.md). Новый CLAUDE.md
// выключил бы это чтение — чтобы правила AGENTS.md не пропали, он начинается
// с импорта.
func keepsAgentsMD(newFile string) bool {
	if filepath.Base(newFile) != ClaudeMD {
		return false
	}
	dir := filepath.Dir(newFile)
	if !adapter.IsFile(filepath.Join(dir, AgentsMD)) {
		return false
	}
	for _, n := range []string{ClaudeLocalMD, filepath.Join(".claude", ClaudeMD)} {
		if _, err := os.Lstat(filepath.Join(dir, n)); err == nil {
			return false
		}
	}
	return true
}

func sectionWarnings(c SectionCopy, sec Section, to model.Item, agent model.Agent, before, after string) []writer.Warning {
	var out []writer.Warning
	add := func(code string, kv ...string) {
		w := writer.Warning{Code: code, Params: map[string]string{}}
		for i := 0; i+1 < len(kv); i += 2 {
			w.Params[kv[i]] = kv[i+1]
		}
		out = append(out, w)
	}
	if agent == model.AgentCodex && len(sec.Imports) > 0 {
		add(WarnImports, "imports", strings.Join(sec.Imports, ", "))
	}
	if agent == model.AgentClaude {
		for _, imp := range Imports(before) {
			if adapter.PathKey(ResolveImport(to.File, imp)) == adapter.PathKey(c.From.File) {
				add(WarnImported, "file", to.Title, "import", imp)
				break
			}
		}
	}
	if sec.Heading != "" {
		for _, s := range Sections(before) {
			if strings.EqualFold(s.Heading, sec.Heading) {
				add(WarnHeadingExists, "heading", sec.Heading)
				break
			}
		}
	}
	for _, w := range to.Warnings {
		if w.Code == "codex.instructions.shadowed" {
			add(WarnShadowed, "file", to.Title, "override", w.Params["file"])
		}
	}
	if to.Role == RoleRule && to.Frontmatter != nil && to.Frontmatter["paths"] != nil {
		add(WarnConditional, "file", to.Title)
	}
	if to.File == c.NewFile && keepsAgentsMD(c.NewFile) {
		add(WarnImportsAgents, "file", to.Title)
	}
	// Лимит Codex — на проектные файлы вместе; один файл больше лимита —
	// точно не прочитается целиком.
	if agent == model.AgentCodex && !c.From.Project.Global && len(after) > CodexInstructionsLimit {
		add(WarnCodexSize, "size", strconv.Itoa(len(after)), "limit", strconv.Itoa(CodexInstructionsLimit))
	}
	out = append(out, Warnings(to)...)
	if c.Move && c.From.Repo != "" && c.From.Repo != to.Repo {
		out = append(out, Warnings(model.Item{Repo: c.From.Repo})...)
	}
	return out
}
