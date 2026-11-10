package markdown

import (
	"strings"
	"testing"
)

func TestFrontmatterRoundTrip(t *testing.T) {
	src := "name: prefer-yarn\n" +
		"description: Зависимости фронтенда ставятся только через yarn 4, очень длинное описание, которое не должно переноситься на несколько строк при сохранении\n" +
		"metadata:\n" +
		"  type: feedback\n" +
		"  originSessionId: 00000000-0000-0000-0000-000000000001\n" +
		"  modified: 2026-01-02T10:00:00.000Z\n" +
		"unknown: [1, 2]\n" +
		"# комментарий\n" +
		"quoted: '007'\n"

	fm, err := ParseFrontmatterNode(src)
	if err != nil {
		t.Fatal(err)
	}
	out, err := fm.String()
	if err != nil {
		t.Fatal(err)
	}
	if out != src {
		t.Errorf("без правок текст должен сохраниться байт в байт:\n%s\n---\n%s", out, src)
	}

	fm.Set("Новое описание", "description")
	fm.Set("reference", "metadata", "type")
	fm.Set("2026-02-03T04:05:06.000Z", "metadata", "modified")
	fm.Set("42", "extra", "count")
	out, _ = fm.String()
	want := []string{
		"name: prefer-yarn\ndescription: Новое описание\nmetadata:\n  type: reference\n",
		"  modified: 2026-02-03T04:05:06.000Z\n",
		"unknown: [1, 2]\n",
		"quoted: '007'\n",
		"extra:\n  count: \"42\"\n",
	}
	for _, w := range want {
		if !strings.Contains(out, w) {
			t.Errorf("нет %q в\n%s", w, out)
		}
	}
	if strings.Index(out, "name:") > strings.Index(out, "description:") {
		t.Error("порядок ключей нарушен")
	}
	if v, ok := fm.Get("metadata", "type"); !ok || v != "reference" {
		t.Errorf("Get = %q %v", v, ok)
	}
}

func TestFrontmatterRejectsNonMapping(t *testing.T) {
	if _, err := ParseFrontmatterNode("- a\n- b\n"); err == nil {
		t.Error("список вместо словаря должен быть ошибкой")
	}
}

func TestJoin(t *testing.T) {
	fm := NewFrontmatter()
	fm.Set("x", "name")
	got, err := Join(fm, "\n\nТекст")
	if err != nil || got != "---\nname: x\n---\n\nТекст\n" {
		t.Errorf("%q %v", got, err)
	}
}
