package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yarkovaleksei/ai-memory/internal/adapter/agentfiles"
	"github.com/yarkovaleksei/ai-memory/internal/model"
)

// Правило переносится из CLAUDE.md проекта в его AGENTS.md одной операцией и
// одной же откатывается байт в байт (критерий фазы 4).
func TestMoveSectionBetweenInstructions(t *testing.T) {
	e := newEditEnvWith(t, func(home string) { relocateHome(t, home) })
	ctx := context.Background()
	from := e.findIn(t, model.KindInstructions, "CLAUDE.md", "demo")

	d, err := e.plans.InstructionSections(ctx, from.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Sections) != 2 || d.Sections[1].Heading != "Только для Claude Code" || d.Sections[0].Imports[0] != "AGENTS.md" {
		t.Fatalf("разделы: %+v", d.Sections)
	}

	targets, err := e.plans.SectionTargets(ctx, from.ID)
	if err != nil {
		t.Fatal(err)
	}
	var titles []string
	for _, tg := range targets {
		titles = append(titles, string(tg.Agent)+":"+tg.Title)
		if !tg.Exists {
			t.Errorf("у проекта есть инструкции обоих агентов, а предложено создать %s", tg.Title)
		}
	}
	// Сначала файлы Codex, главный первым; файлы Claude — после.
	if got := strings.Join(titles, ","); !strings.HasPrefix(got, "codex:AGENTS.md,codex:AGENTS.override.md,claude:") {
		t.Fatalf("цели: %s", got)
	}
	to := targets[0]

	fromBefore, toBefore := readFile(t, from.File), readFile(t, to.File)
	p, err := e.plans.CopySection(ctx, from.ID, d.Hash, 1, to.Key, to.Hash, -1, true)
	if err != nil {
		t.Fatal(err)
	}
	if p.Op.Code != "instructions.section-move" || warningCodes(p) != agentfiles.WarnShadowed {
		t.Errorf("операция %s, предупреждения %q", p.Op.Code, warningCodes(p))
	}
	res := e.apply(t)(p, nil)
	if !strings.HasSuffix(readFile(t, to.File), "## Только для Claude Code\n\n- Тесты запускай через `yarn test`.\n") ||
		strings.Contains(readFile(t, from.File), "Только для Claude Code") {
		t.Fatalf("после переноса:\n%s\n---\n%s", readFile(t, to.File), readFile(t, from.File))
	}
	e.apply(t)(e.history.Revert(ctx, res.Commit, ""))
	if readFile(t, from.File) != fromBefore || readFile(t, to.File) != toBefore {
		t.Error("откат не вернул файлы байт в байт")
	}

	// Устаревший хэш, чужой ключ и раздел из записи памяти — отказ.
	if _, err := e.plans.CopySection(ctx, from.ID, "old", 1, to.Key, to.Hash, -1, false); errCode(err) != "write.conflict" {
		t.Errorf("устаревший хэш: %v", err)
	}
	if _, err := e.plans.CopySection(ctx, from.ID, d.Hash, 1, "new:bogus", "", -1, false); errCode(err) != "section.target-invalid" {
		t.Errorf("чужой ключ: %v", err)
	}
	global := e.findIn(t, model.KindInstructions, "AGENTS.md", ".codex")
	if _, err := e.plans.CopySection(ctx, from.ID, d.Hash, 1, global.ID, "", -1, false); errCode(err) != "section.target-invalid" {
		t.Errorf("цель из другого проекта: %v", err)
	}
	fact := e.searchKind(t, model.KindClaudeFact)[0]
	if _, err := e.plans.InstructionSections(ctx, fact.ID); errCode(err) != "section.not-instructions" {
		t.Errorf("разделы факта: %v", err)
	}
}

// Нет файла инструкций агента — он создаётся; откат его удаляет. Новый
// CLAUDE.md рядом с AGENTS.md импортирует его: иначе Claude Code перестал бы
// читать AGENTS.md.
func TestCopySectionCreatesFile(t *testing.T) {
	e := newEditEnvWith(t, func(home string) {
		relocateHome(t, home)
		demo := filepath.Join(home, "projects", "demo")
		for _, p := range []string{
			filepath.Join(demo, "CLAUDE.md"), filepath.Join(demo, "CLAUDE.local.md"),
			filepath.Join(home, ".codex", "AGENTS.md"),
		} {
			if err := os.Remove(p); err != nil {
				t.Fatal(err)
			}
		}
	})
	ctx := context.Background()
	tests := []struct {
		name    string
		from    model.Item
		section int
		key     string
		file    string
		want    string
		warn    string
	}{
		{
			name: "CLAUDE.md проекта из AGENTS.md", from: e.findIn(t, model.KindInstructions, "AGENTS.md", "demo"), section: 1,
			key: "new:" + agentfiles.ClaudeProjects, file: filepath.Join(e.home, "projects", "demo", "CLAUDE.md"),
			want: "@AGENTS.md\n\n## Код\n\n- Используй yarn, а не npm.\n", warn: agentfiles.WarnImportsAgents,
		},
		{
			name: "AGENTS.md в CODEX_HOME из глобального CLAUDE.md", from: e.findIn(t, model.KindInstructions, "CLAUDE.md", ".claude"),
			key: "new:" + agentfiles.CodexFilesID(filepath.Join(e.home, ".codex")), file: filepath.Join(e.home, ".codex", "AGENTS.md"),
			want: "# Глобальные правила (фикстура)\n\n- Отвечай по-русски.\n- Перед коммитом запускай проверки проекта.\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			targets, err := e.plans.SectionTargets(ctx, tt.from.ID)
			if err != nil {
				t.Fatal(err)
			}
			var found bool
			for _, tg := range targets {
				if tg.Key == tt.key {
					found = !tg.Exists && tg.File == tt.file
				}
			}
			if !found {
				t.Fatalf("нет цели %s среди %+v", tt.key, targets)
			}
			d, err := e.plans.InstructionSections(ctx, tt.from.ID)
			if err != nil {
				t.Fatal(err)
			}
			p, err := e.plans.CopySection(ctx, tt.from.ID, d.Hash, tt.section, tt.key, "", -1, false)
			if err != nil {
				t.Fatal(err)
			}
			if warningCodes(p) != tt.warn {
				t.Errorf("предупреждения %q, want %q", warningCodes(p), tt.warn)
			}
			res := e.apply(t)(p, nil)
			if got := readFile(t, tt.file); got != tt.want {
				t.Errorf("новый файл: %q", got)
			}
			e.apply(t)(e.history.Revert(ctx, res.Commit, ""))
			if _, err := os.Stat(tt.file); !os.IsNotExist(err) {
				t.Errorf("откат не удалил созданный файл: %v", err)
			}
		})
	}
}
