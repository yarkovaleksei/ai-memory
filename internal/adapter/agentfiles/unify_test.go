package agentfiles

import (
	"errors"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/yarkovaleksei/ai-memory/internal/writer"
)

func TestBlocks(t *testing.T) {
	tests := []struct {
		name    string
		content string
		prefix  string
		want    []string // заголовки блоков; «*» — вступление
	}{
		{"один заголовок документа", "# Проект\n\nвступление\n\n## Код\n\n### Тесты\n\n## Git\n", "", []string{"*Проект", "Код", "Git"}},
		{"разделы верхнего уровня", "текст\n\n## A\n\n## B\n", "", []string{"", "A", "B"}},
		{"без подразделов", "# Только заголовок\n\n- x\n", "", []string{"Только заголовок"}},
		{"frontmatter остаётся", "---\nk: v\n---\n\n## A\n", "---\nk: v\n---\n\n", []string{"A"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prefix, blocks := Blocks(tt.content)
			var got []string
			text := prefix
			for _, b := range blocks {
				h := b.Heading
				if b.Intro {
					h = "*" + h
				}
				got = append(got, h)
				text += tt.content[b.start:b.end]
			}
			if prefix != tt.prefix || !reflect.DeepEqual(got, tt.want) {
				t.Errorf("prefix %q, блоки %v", prefix, got)
			}
			if text != tt.content {
				t.Errorf("блоки не покрывают файл: %q", text)
			}
		})
	}
}

func TestPlanUnify(t *testing.T) {
	claude := "# Проект\n\nСм. @docs/a.md.\n\n## Код\n\n- yarn\n\n## Только для Claude Code\n\n- /review\n"
	tests := []struct {
		name   string
		files  map[string]string
		move   []int
		state  string
		claude string // новый CLAUDE.md (относительно проекта: CLAUDE.md или .claude/CLAUDE.md)
		agents string // новый AGENTS.md; пусто — не меняется
		warn   string
		fail   string
	}{
		{
			name:   "оба файла: общий раздел уходит в AGENTS.md",
			files:  map[string]string{"CLAUDE.md": claude, "AGENTS.md": "# Правила\n\n- Коммить по просьбе.\n"},
			move:   []int{1},
			state:  UnifyBoth,
			claude: "# Проект\n\nСм. @docs/a.md.\n\n@AGENTS.md\n\n## Только для Claude Code\n\n- /review\n",
			agents: "# Правила\n\n- Коммить по просьбе.\n\n## Код\n\n- yarn\n",
		},
		{
			name:   "только CLAUDE.md: AGENTS.md создаётся, импорты предупреждают",
			files:  map[string]string{"CLAUDE.md": claude, "AGENTS.override.md": "x\n"},
			move:   []int{0, 1},
			state:  UnifyClaudeOnly,
			claude: "@AGENTS.md\n\n## Только для Claude Code\n\n- /review\n",
			agents: "# Проект\n\nСм. @docs/a.md.\n\n## Код\n\n- yarn\n",
			warn:   WarnImports + "," + WarnShadowed,
		},
		{
			name:   "только AGENTS.md: CLAUDE.md из одного импорта",
			files:  map[string]string{"AGENTS.md": "# Правила\n"},
			state:  UnifyAgentsOnly,
			claude: "@AGENTS.md\n",
		},
		{
			name:   ".claude/CLAUDE.md импортирует ../AGENTS.md",
			files:  map[string]string{".claude/CLAUDE.md": "## Код\n\n- yarn\n", "AGENTS.md": "# Правила\n"},
			state:  UnifyBoth,
			claude: "@../AGENTS.md\n\n## Код\n\n- yarn\n",
		},
		{
			name:  "уже импортирует",
			files: map[string]string{"CLAUDE.md": "@AGENTS.md\n", "AGENTS.md": "x\n"},
			state: UnifyDone,
			fail:  "unify.already",
		},
		{
			name:  "нечего переносить в новый AGENTS.md",
			files: map[string]string{"CLAUDE.md": claude},
			state: UnifyClaudeOnly,
			fail:  "unify.nothing-to-move",
		},
		{
			name:  "инструкций нет",
			files: map[string]string{},
			state: UnifyNone,
			fail:  "unify.nothing",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			for name, content := range tt.files {
				write(t, filepath.Join(dir, filepath.FromSlash(name)), content)
			}
			st, err := ReadUnify(dir)
			if err != nil {
				t.Fatal(err)
			}
			if st.State != tt.state {
				t.Fatalf("состояние %s, want %s", st.State, tt.state)
			}
			p, err := PlanUnify(dir, st.ClaudeHash, st.AgentsHash, tt.move)
			if tt.fail != "" {
				if failCode(err) != tt.fail {
					t.Fatalf("ошибка %v, want %s", err, tt.fail)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			got := map[string]string{}
			for _, c := range p.Changes {
				got[c.Path] = c.After
			}
			if got[st.ClaudeFile] != tt.claude {
				t.Errorf("CLAUDE.md: %q", got[st.ClaudeFile])
			}
			if a, ok := got[st.AgentsFile]; ok != (tt.agents != "") || a != tt.agents {
				t.Errorf("AGENTS.md: %q", a)
			}
			if w := warnCodes(p.Warnings); w != tt.warn {
				t.Errorf("предупреждения %q, want %q", w, tt.warn)
			}
		})
	}

	// Файл изменился после того, как пользователь открыл предложение.
	dir := t.TempDir()
	write(t, filepath.Join(dir, "CLAUDE.md"), claude)
	if _, err := PlanUnify(dir, "", "", []int{1}); failCode(err) != "write.conflict" {
		t.Errorf("устаревший хэш: %v", err)
	}
}

func failCode(err error) string {
	var we *writer.Error
	if errors.As(err, &we) {
		return we.Code
	}
	return ""
}
