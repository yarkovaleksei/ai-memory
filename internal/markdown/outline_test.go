package markdown

import (
	"reflect"
	"testing"
)

func TestOutline(t *testing.T) {
	type part struct {
		Level   int
		Heading string
		Text    string
	}
	tests := []struct {
		name string
		text string
		want []part
	}{
		{
			name: "вложенные разделы",
			text: "# A\n\nвступление\n\n## B\n\nb\n\n### C\n\nc\n\n## D\n\nd\n",
			want: []part{
				{1, "A", "# A\n\nвступление\n\n## B\n\nb\n\n### C\n\nc\n\n## D\n\nd\n"},
				{2, "B", "## B\n\nb\n\n### C\n\nc\n\n"},
				{3, "C", "### C\n\nc\n\n"},
				{2, "D", "## D\n\nd\n"},
			},
		},
		{
			name: "текст до первого заголовка",
			text: "@AGENTS.md\n\n## Только для Claude\n\n- x\n",
			want: []part{
				{0, "", "@AGENTS.md\n\n"},
				{2, "Только для Claude", "## Только для Claude\n\n- x\n"},
			},
		},
		{
			name: "заголовки в коде и не-заголовки не режут",
			text: "## A\n\n```md\n## не заголовок\n```\n\n~~~~\n# тоже\n~~~~\n\n#хештег\n    # отступ\n## B ##\n",
			want: []part{
				{2, "A", "## A\n\n```md\n## не заголовок\n```\n\n~~~~\n# тоже\n~~~~\n\n#хештег\n    # отступ\n"},
				{2, "B", "## B ##\n"},
			},
		},
		{
			name: "пустой текст",
			text: "\n\n",
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got []part
			for _, p := range Outline(tt.text) {
				got = append(got, part{p.Level, p.Heading, tt.text[p.Start:p.End]})
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Outline:\n got %+v\nwant %+v", got, tt.want)
			}
		})
	}
}
