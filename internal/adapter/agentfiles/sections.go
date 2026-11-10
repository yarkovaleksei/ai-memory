package agentfiles

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/yarkovaleksei/ai-memory/internal/markdown"
)

// Section — раздел файла инструкций: то, что переносится как одно правило
// (ADR-021). Index — номер в Sections того же содержимого: интерфейс
// возвращает его вместе с хэшем файла, поэтому номер не уплывает.
type Section struct {
	Index   int    `json:"index"`
	Level   int    `json:"level"`   // 1–6; 0 — текст до первого заголовка
	Heading string `json:"heading"` // пусто для текста до первого заголовка
	Text    string `json:"text"`    // раздел целиком, с подразделами
	// Imports — импорты Claude «@путь» в тексте раздела. Codex их не понимает:
	// в AGENTS.md они останутся строкой текста.
	Imports []string `json:"imports"`

	start, end int // смещения в нормализованном содержимом файла
}

// Sections режет файл инструкций на разделы по заголовкам. Frontmatter
// правила в разделы не входит.
func Sections(content string) []Section {
	content = markdown.Normalize(content)
	base := 0
	if _, body, has := markdown.SplitFrontmatter(content); has {
		base = len(content) - len(body)
	}
	parts := markdown.Outline(content[base:])
	out := make([]Section, 0, len(parts))
	for i, p := range parts {
		text := strings.Trim(content[base+p.Start:base+p.End], "\n")
		out = append(out, Section{
			Index:   i,
			Level:   p.Level,
			Heading: p.Heading,
			Text:    text,
			Imports: Imports(text),
			start:   base + p.Start,
			end:     base + p.End,
		})
	}
	return out
}

var (
	fencedCode = regexp.MustCompile("(?ms)^ {0,3}(```|~~~).*?^ {0,3}(```|~~~)[^\n]*$")
	inlineCode = regexp.MustCompile("`[^`\n]*`")
	// Импорт Claude — «@» в начале слова и путь с точкой или слэшем: так не
	// ловятся упоминания вроде «@me» и адреса почты.
	importRe = regexp.MustCompile(`(?:^|[\s(\[])@((?:~/|\.{0,2}/)?[^\s()\[\]@]*[./][^\s()\[\]@]*)`)
)

// Imports — импорты «@путь» в тексте инструкций Claude вне блоков кода и
// строк кода, без повторов, в порядке появления.
func Imports(text string) []string {
	text = fencedCode.ReplaceAllString(text, "")
	text = inlineCode.ReplaceAllString(text, "")
	out := []string{}
	seen := map[string]bool{}
	for _, m := range importRe.FindAllStringSubmatch(text, -1) {
		p := strings.TrimRight(m[1], ".,;:!?")
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	return out
}

// ResolveImport — файл, на который ведёт импорт из файла from: относительно
// его каталога, «~/» — от домашнего каталога.
func ResolveImport(from, imp string) string {
	switch {
	case strings.HasPrefix(imp, "~/"):
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		return filepath.Join(home, filepath.FromSlash(imp[2:]))
	case filepath.IsAbs(filepath.FromSlash(imp)):
		return filepath.Clean(filepath.FromSlash(imp))
	}
	return filepath.Join(filepath.Dir(from), filepath.FromSlash(imp))
}

// InsertSection вставляет текст раздела в содержимое файла: в конец файла
// (after < 0) или в конец раздела after — последним его подразделом. Между
// разделами — одна пустая строка, файл кончается переводом строки.
func InsertSection(content, text string, after int) (string, bool) {
	content = markdown.Normalize(content)
	text = strings.Trim(markdown.Normalize(text), "\n")
	at := len(content)
	if after >= 0 {
		secs := Sections(content)
		if after >= len(secs) {
			return "", false
		}
		at = secs[after].end
	}
	return join(content[:at], text, content[at:]), true
}

// RemoveSection убирает раздел index вместе с подразделами.
func RemoveSection(content string, index int) (string, bool) {
	content = markdown.Normalize(content)
	secs := Sections(content)
	if index < 0 || index >= len(secs) {
		return "", false
	}
	s := secs[index]
	return join(content[:s.start], "", content[s.end:]), true
}

// join склеивает куски через пустую строку, пропуская пустые.
func join(parts ...string) string {
	var keep []string
	for _, p := range parts {
		if p = strings.Trim(p, "\n"); strings.TrimSpace(p) != "" {
			keep = append(keep, p)
		}
	}
	if len(keep) == 0 {
		return ""
	}
	return strings.Join(keep, "\n\n") + "\n"
}

// withLineEndings возвращает тексту переводы строк исходного файла: файл с
// CRLF не должен целиком смениться в diff и в git status проекта.
func withLineEndings(original, text string) string {
	if strings.Contains(original, "\r\n") {
		return strings.ReplaceAll(text, "\n", "\r\n")
	}
	return text
}
