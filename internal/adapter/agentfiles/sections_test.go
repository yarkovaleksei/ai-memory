package agentfiles

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/yarkovaleksei/ai-memory/internal/model"
	"github.com/yarkovaleksei/ai-memory/internal/writer"
)

func TestSections(t *testing.T) {
	content := "---\npaths:\n  - \"**/*.go\"\n---\n\n# Правило\n\nСм. @docs/style.md и `@не-импорт.md`, пиши me@example.com.\n\n## Подраздел\n\n@~/.claude/extra.md\n"
	secs := Sections(content)
	if len(secs) != 2 {
		t.Fatalf("разделов %d: %+v", len(secs), secs)
	}
	if secs[0].Heading != "Правило" || secs[0].Level != 1 || strings.Contains(secs[0].Text, "paths") {
		t.Errorf("первый раздел: %+v", secs[0])
	}
	if !reflect.DeepEqual(secs[0].Imports, []string{"docs/style.md", "~/.claude/extra.md"}) {
		t.Errorf("импорты раздела с подразделом: %v", secs[0].Imports)
	}
	if secs[1].Text != "## Подраздел\n\n@~/.claude/extra.md" || secs[1].Index != 1 {
		t.Errorf("подраздел: %+v", secs[1])
	}
}

func TestImports(t *testing.T) {
	tests := []struct {
		text string
		want []string
	}{
		{"@AGENTS.md", []string{"AGENTS.md"}},
		{"- (@./a.md), @../b/c.md.", []string{"./a.md", "../b/c.md"}},
		{"```\n@code.md\n```\n@real.md @real.md", []string{"real.md"}},
		{"пиши @me, почта a@b.ru", []string{}},
	}
	for _, tt := range tests {
		if got := Imports(tt.text); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("Imports(%q) = %v, want %v", tt.text, got, tt.want)
		}
	}
}

func TestInsertRemoveSection(t *testing.T) {
	doc := "# Правила\n\n## Код\n\n- yarn\n\n## Git\n\n- коммит\n"
	tests := []struct {
		name  string
		after int
		want  string
	}{
		{"в конец", -1, doc + "\n## Тесты\n\n- vitest\n"},
		{"в конец раздела «Код»", 1, "# Правила\n\n## Код\n\n- yarn\n\n## Тесты\n\n- vitest\n\n## Git\n\n- коммит\n"},
		{"в конец раздела верхнего уровня", 0, doc + "\n## Тесты\n\n- vitest\n"},
	}
	for _, tt := range tests {
		got, ok := InsertSection(doc, "\n## Тесты\n\n- vitest\n\n", tt.after)
		if !ok || got != tt.want {
			t.Errorf("%s: %q", tt.name, got)
		}
	}
	if _, ok := InsertSection(doc, "x", 9); ok {
		t.Error("вставка после несуществующего раздела")
	}
	if got, _ := InsertSection("", "## A\n", -1); got != "## A\n" {
		t.Errorf("в пустой файл: %q", got)
	}
	if got, _ := RemoveSection(doc, 1); got != "# Правила\n\n## Git\n\n- коммит\n" {
		t.Errorf("удаление: %q", got)
	}
	if got, _ := RemoveSection(doc, 0); got != "" {
		t.Errorf("удаление всего: %q", got)
	}
}

func demoItems(t *testing.T, home string) map[string]model.Item {
	t.Helper()
	demo := filepath.Join(home, "projects", "demo")
	_, items := scanAll(t, Adapter{
		ClaudeProfiles: []string{filepath.Join(home, ".claude")},
		CodexHome:      filepath.Join(home, ".codex"),
		Projects:       []string{demo},
	})
	out := map[string]model.Item{}
	for _, it := range items {
		if it.Kind == model.KindInstructions {
			scope := "p:"
			if it.Project.Global {
				scope = "g:"
			}
			out[scope+string(it.Agent)+":"+it.Title] = it
		}
	}
	return out
}

func warnCodes(ws []writer.Warning) string {
	var out []string
	for _, w := range ws {
		out = append(out, w.Code)
	}
	return strings.Join(out, ",")
}

func TestPlanSection(t *testing.T) {
	home := fixtureHome(t)
	items := demoItems(t, home)
	claudeMD, agentsMD := items["p:claude:CLAUDE.md"], items["p:codex:AGENTS.md"]
	override := items["p:codex:AGENTS.override.md"]
	globalClaude, globalCodex := items["g:claude:CLAUDE.md"], items["g:codex:AGENTS.md"]
	if claudeMD.File == "" || agentsMD.File == "" || override.File == "" || globalClaude.File == "" || globalCodex.File == "" {
		t.Fatalf("фикстуры: %v", items)
	}
	newFile := filepath.Join(t.TempDir(), "AGENTS.md")
	tests := []struct {
		name     string
		c        SectionCopy
		wantOp   string
		wantPath []string
		want     []string // куски нового содержимого первой цели
		warn     string
		fail     string
	}{
		{
			name:     "из CLAUDE.md в AGENTS.md: импорт и затенение",
			c:        SectionCopy{From: claudeMD, Section: 0, To: agentsMD, After: -1},
			wantOp:   "instructions.section-copy",
			wantPath: []string{agentsMD.File},
			want:     []string{"## Git\n\n- Коммить только по просьбе.\n\n# Проект demo (фикстура)\n\n@AGENTS.md\n"},
			warn:     WarnImports + "," + WarnShadowed + "," + WarnInRepo, // фикстуры лежат в git-репозитории ai-memory
		},
		{
			name:     "из AGENTS.md в CLAUDE.md, который его импортирует",
			c:        SectionCopy{From: agentsMD, Section: 1, To: claudeMD, After: 1},
			wantOp:   "instructions.section-copy",
			wantPath: []string{claudeMD.File},
			want:     []string{"- Тесты запускай через `yarn test`.\n\n## Код\n\n- Используй yarn, а не npm.\n"},
			warn:     WarnImported + "," + WarnInRepo,
		},
		{
			name:     "перенос: убрать из исходного",
			c:        SectionCopy{From: agentsMD, Section: 2, To: override, After: -1, Move: true},
			wantOp:   "instructions.section-move",
			wantPath: []string{override.File, agentsMD.File},
			want:     []string{"legacy/`.\n\n## Git\n"},
			warn:     WarnInRepo,
		},
		{
			name:     "новый AGENTS.md",
			c:        SectionCopy{From: globalClaude, Section: 0, NewFile: newFile, NewAgent: model.AgentCodex, After: -1},
			wantOp:   "instructions.section-copy",
			wantPath: []string{newFile},
			want:     []string{"# Глобальные правила (фикстура)\n\n- Отвечай по-русски.\n- Перед коммитом запускай проверки проекта.\n"},
		},
		{
			name: "хэш исходного файла устарел",
			c:    SectionCopy{From: claudeMD, FromHash: "old", Section: 0, To: agentsMD},
			fail: "write.conflict",
		},
		{
			name: "в тот же файл",
			c:    SectionCopy{From: agentsMD, Section: 1, To: agentsMD},
			fail: "section.same-file",
		},
		{
			name: "нет такого раздела",
			c:    SectionCopy{From: agentsMD, Section: 7, To: claudeMD},
			fail: "section.not-found",
		},
		{
			name: "новый файл уже есть",
			c:    SectionCopy{From: agentsMD, Section: 1, NewFile: globalCodex.File, NewAgent: model.AgentCodex},
			fail: "write.conflict",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := PlanSection(tt.c)
			if tt.fail != "" {
				var we *writer.Error
				if err == nil || !errors.As(err, &we) || we.Code != tt.fail {
					t.Fatalf("ошибка %v, want %s", err, tt.fail)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if p.Op.Code != tt.wantOp {
				t.Errorf("операция %s", p.Op.Code)
			}
			var paths []string
			for _, c := range p.Changes {
				paths = append(paths, c.Path)
			}
			if !reflect.DeepEqual(paths, tt.wantPath) {
				t.Errorf("пути %v", paths)
			}
			for _, w := range tt.want {
				if !strings.Contains(p.Changes[0].After, w) {
					t.Errorf("в новом содержимом нет %q:\n%s", w, p.Changes[0].After)
				}
			}
			if got := warnCodes(p.Warnings); got != tt.warn {
				t.Errorf("предупреждения %q, want %q", got, tt.warn)
			}
		})
	}

	// Перенос убирает раздел из исходного файла, остальное не трогает.
	p, err := PlanSection(SectionCopy{From: agentsMD, Section: 2, To: override, After: -1, Move: true})
	if err != nil {
		t.Fatal(err)
	}
	if got := p.Changes[1].After; got != "# Правила проекта demo (фикстура)\n\n## Код\n\n- Используй yarn, а не npm.\n" {
		t.Errorf("исходный после переноса: %q", got)
	}
}

// Файл с CRLF остаётся с CRLF: иначе diff и git status покажут весь файл.
func TestPlanSectionKeepsCRLF(t *testing.T) {
	dir := t.TempDir()
	from, to := filepath.Join(dir, "CLAUDE.md"), filepath.Join(dir, "AGENTS.md")
	write(t, from, "## A\n\n- a\n")
	write(t, to, "# T\r\n\r\n- t\r\n")
	p, err := PlanSection(SectionCopy{
		From:    model.Item{Kind: model.KindInstructions, Agent: model.AgentClaude, File: from, Title: "CLAUDE.md"},
		Section: 0,
		To:      model.Item{Kind: model.KindInstructions, Agent: model.AgentCodex, File: to, Title: "AGENTS.md", Write: model.WriteDirect},
		After:   -1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := p.Changes[0].After; got != "# T\r\n\r\n- t\r\n\r\n## A\r\n\r\n- a\r\n" {
		t.Errorf("CRLF: %q", got)
	}
	if _, err := os.Stat(to); err != nil {
		t.Fatal(err)
	}
}
