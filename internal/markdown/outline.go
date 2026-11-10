package markdown

import "strings"

// Part — раздел документа по ATX-заголовку вместе с подразделами: от строки
// заголовка до следующего заголовка того же или более высокого уровня.
// Start и End — смещения в байтах в тексте, который получил Outline.
type Part struct {
	Level   int    // 1–6; 0 — текст до первого заголовка
	Heading string // текст заголовка без «#» и закрывающих «#»
	Start   int
	End     int
}

// Outline — разделы текста в порядке заголовков. Разделы вложены: раздел
// уровня 2 входит в раздел уровня 1 над ним. Заголовки внутри блоков кода
// (``` и ~~~) не считаются. Текст до первого заголовка — раздел уровня 0,
// если в нём есть что-то кроме пустых строк.
//
// Текст должен быть нормализован (Normalize): смещения считаются по \n.
func Outline(text string) []Part {
	type head struct {
		level   int
		heading string
		start   int
	}
	var heads []head
	fence := ""
	for pos := 0; pos < len(text); {
		end := strings.IndexByte(text[pos:], '\n')
		next := len(text)
		if end >= 0 {
			next = pos + end + 1
		}
		line := strings.TrimRight(text[pos:next], "\n")
		trimmed := strings.TrimLeft(line, " ")
		indent := len(line) - len(trimmed)
		switch {
		case fence != "":
			if indent < 4 && strings.HasPrefix(trimmed, fence) && strings.Trim(trimmed, fence[:1]) == "" {
				fence = ""
			}
		case indent < 4 && (strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~")):
			fence = trimmed[:3]
			for len(fence) < len(trimmed) && trimmed[len(fence)] == fence[0] {
				fence += fence[:1]
			}
		case indent < 4:
			if level, heading, ok := atxHeading(trimmed); ok {
				heads = append(heads, head{level, heading, pos})
			}
		}
		pos = next
	}

	var out []Part
	first := len(text)
	if len(heads) > 0 {
		first = heads[0].start
	}
	if strings.TrimSpace(text[:first]) != "" {
		out = append(out, Part{Start: 0, End: first})
	}
	for i, h := range heads {
		end := len(text)
		for _, n := range heads[i+1:] {
			if n.level <= h.level {
				end = n.start
				break
			}
		}
		out = append(out, Part{Level: h.level, Heading: h.heading, Start: h.start, End: end})
	}
	return out
}

// atxHeading разбирает строку «## Заголовок ##».
func atxHeading(line string) (int, string, bool) {
	level := 0
	for level < len(line) && line[level] == '#' {
		level++
	}
	if level == 0 || level > 6 {
		return 0, "", false
	}
	rest := line[level:]
	if rest != "" && rest[0] != ' ' && rest[0] != '\t' {
		return 0, "", false
	}
	rest = strings.TrimSpace(rest)
	// Закрывающие «#» отбрасываются, только если перед ними пробел.
	if t := strings.TrimRight(rest, "#"); t != rest && (t == "" || strings.HasSuffix(t, " ")) {
		rest = strings.TrimSpace(t)
	}
	return level, rest, true
}
