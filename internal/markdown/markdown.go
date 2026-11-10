// Package markdown — разбор того немногого в Markdown, что нужно адаптерам:
// frontmatter, разделы по заголовкам, первая строка-заголовок.
package markdown

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// Normalize приводит переводы строк к \n: файлы памяти могут прийти с Windows.
func Normalize(s string) string {
	return strings.ReplaceAll(s, "\r\n", "\n")
}

// SplitFrontmatter отделяет YAML-frontmatter между строками «---» от тела.
// has=false, если файл не начинается с «---»; тогда body — весь текст.
// Незакрытый frontmatter тоже считается отсутствующим: лучше показать файл
// целиком, чем потерять его тело.
func SplitFrontmatter(content string) (fm, body string, has bool) {
	content = Normalize(content)
	if !strings.HasPrefix(content, "---\n") {
		return "", content, false
	}
	rest := content[len("---\n"):]
	if strings.HasPrefix(rest, "---\n") || rest == "---" {
		return "", strings.TrimPrefix(strings.TrimPrefix(rest, "---"), "\n"), true
	}
	end := strings.Index(rest, "\n---\n")
	if end < 0 {
		if strings.HasSuffix(rest, "\n---") {
			return rest[:len(rest)-len("\n---")], "", true
		}
		return "", content, false
	}
	return rest[:end], rest[end+len("\n---\n"):], true
}

// ParseFrontmatter разбирает YAML в карту. Порядок ключей здесь не важен:
// writer фазы 2 перечитает файл через yaml.Node, чтобы сохранить его.
func ParseFrontmatter(fm string) (map[string]any, error) {
	out := map[string]any{}
	if strings.TrimSpace(fm) == "" {
		return out, nil
	}
	if err := yaml.Unmarshal([]byte(fm), &out); err != nil {
		return nil, fmt.Errorf("frontmatter: %w", err)
	}
	return out, nil
}

// String достаёт строку из вложенной карты по пути ключей: String(m, "metadata", "type").
func String(m map[string]any, path ...string) string {
	var cur any = m
	for _, k := range path {
		mm, ok := cur.(map[string]any)
		if !ok {
			return ""
		}
		cur = mm[k]
	}
	s, _ := cur.(string)
	return s
}

// Section — раздел документа: заголовок заданного уровня и текст до следующего такого же.
type Section struct {
	Heading string // текст заголовка без «#»; пусто для текста до первого заголовка
	Body    string // текст раздела без строки заголовка
}

// Sections режет текст по заголовкам уровня level (2 — «## »). Заголовки внутри
// блоков кода ``` не считаются.
func Sections(text string, level int) []Section {
	prefix := strings.Repeat("#", level) + " "
	var (
		out    []Section
		cur    Section
		buf    strings.Builder
		inCode bool
	)
	flush := func() {
		cur.Body = strings.Trim(buf.String(), "\n")
		if cur.Heading != "" || strings.TrimSpace(cur.Body) != "" {
			out = append(out, cur)
		}
		buf.Reset()
	}
	for _, line := range strings.Split(Normalize(text), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			inCode = !inCode
		}
		if !inCode && strings.HasPrefix(line, prefix) {
			flush()
			cur = Section{Heading: strings.TrimSpace(line[len(prefix):])}
			continue
		}
		buf.WriteString(line)
		buf.WriteByte('\n')
	}
	flush()
	return out
}

// FirstHeading возвращает текст первого заголовка любого уровня или пустую строку.
func FirstHeading(text string) string {
	for _, line := range strings.Split(Normalize(text), "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "#") {
			return strings.TrimSpace(strings.TrimLeft(t, "#"))
		}
	}
	return ""
}

// FirstParagraph возвращает первый непустой абзац без разметки заголовков,
// обрезанный до max рун — для краткого описания записи.
func FirstParagraph(text string, max int) string {
	for _, para := range strings.Split(Normalize(text), "\n\n") {
		p := strings.TrimSpace(para)
		if p == "" || strings.HasPrefix(p, "#") {
			continue
		}
		p = strings.Join(strings.Fields(p), " ")
		r := []rune(p)
		if len(r) > max {
			return string(r[:max-1]) + "…"
		}
		return p
	}
	return ""
}
