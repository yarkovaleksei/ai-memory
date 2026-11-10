package markdown

import (
	"reflect"
	"testing"
)

func TestSplitFrontmatter(t *testing.T) {
	tests := []struct {
		name, in string
		fm, body string
		has      bool
	}{
		{"обычный", "---\nname: a\n---\n\nТекст\n", "name: a", "\nТекст\n", true},
		{"CRLF", "---\r\nname: a\r\n---\r\nТекст", "name: a", "Текст", true},
		{"пустой", "---\n---\nТекст", "", "Текст", true},
		{"без frontmatter", "# Заголовок\n", "", "# Заголовок\n", false},
		{"незакрытый", "---\nname: a\nТекст", "", "---\nname: a\nТекст", false},
		{"закрыт в конце файла", "---\nname: a\n---", "name: a", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fm, body, has := SplitFrontmatter(tt.in)
			if fm != tt.fm || body != tt.body || has != tt.has {
				t.Errorf("got (%q, %q, %v), want (%q, %q, %v)", fm, body, has, tt.fm, tt.body, tt.has)
			}
		})
	}
}

func TestParseFrontmatter(t *testing.T) {
	m, err := ParseFrontmatter("name: a\nmetadata:\n  type: feedback\n")
	if err != nil {
		t.Fatal(err)
	}
	if String(m, "metadata", "type") != "feedback" || String(m, "name") != "a" || String(m, "nope", "x") != "" {
		t.Errorf("String: %v", m)
	}
	if _, err := ParseFrontmatter("description: [незакрытая"); err == nil {
		t.Error("ожидалась ошибка разбора")
	}
}

func TestSections(t *testing.T) {
	text := "v1\n\n## Первый\n\nтекст 1\n\n```\n## не заголовок\n```\n\n## Второй\n### Под\nтекст 2\n"
	got := Sections(text, 2)
	want := []Section{
		{"", "v1"},
		{"Первый", "\nтекст 1\n\n```\n## не заголовок\n```"},
		{"Второй", "### Под\nтекст 2"},
	}
	// Body обрезается от пустых строк по краям.
	want[1].Body = "текст 1\n\n```\n## не заголовок\n```"
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Sections:\n got  %#v\n want %#v", got, want)
	}
}

func TestFirstHeadingAndParagraph(t *testing.T) {
	text := "\n# 2026-01-03 — пакеты\n\n- Пакеты   ставить\n  только через yarn.\n"
	if got := FirstHeading(text); got != "2026-01-03 — пакеты" {
		t.Errorf("FirstHeading = %q", got)
	}
	if got := FirstParagraph(text, 100); got != "- Пакеты ставить только через yarn." {
		t.Errorf("FirstParagraph = %q", got)
	}
	if got := FirstParagraph("абвгдеёжз", 5); got != "абвг…" {
		t.Errorf("FirstParagraph обрезка = %q", got)
	}
}
