package app

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/yarkovaleksei/ai-memory/internal/adapter/agentfiles"
	"github.com/yarkovaleksei/ai-memory/internal/index"
	"github.com/yarkovaleksei/ai-memory/internal/model"
)

// skillOf — навык агента agent с заголовком title, если он есть в индексе.
func (e editEnv) skillOf(t *testing.T, agent model.Agent, title string) (index.Hit, bool) {
	t.Helper()
	for _, h := range e.searchKind(t, model.KindSkill) {
		if h.Agent == agent && h.Title == title {
			return h, true
		}
	}
	return index.Hit{}, false
}

// Критерий фазы 4: навык Claude ставится в Codex копией или симлинком и после
// этого виден в навыках Codex; откат убирает его.
func TestInstallSkill(t *testing.T) {
	e := newEditEnvWith(t, func(home string) { relocateHome(t, home) })
	ctx := context.Background()
	codexHome := filepath.Join(e.home, ".codex")
	tests := []struct {
		name  string
		from  model.Item
		key   string
		link  bool
		agent model.Agent
		dir   string
		files []string // относительно каталога навыка
		warn  string
	}{
		{
			name: "копия в ~/.agents со скриптом", from: e.findIn(t, model.KindSkill, "release-notes", "release-notes"),
			key: "agents", agent: model.AgentCodex, dir: filepath.Join(e.home, ".agents", "skills", "release-notes"),
			files: []string{"SKILL.md", "scripts/last-tag.sh"},
		},
		{
			name: "симлинк в CODEX_HOME/skills", from: e.findIn(t, model.KindSkill, "release-notes", "release-notes"),
			key: "codex-home", link: true, agent: model.AgentCodex, dir: filepath.Join(codexHome, "skills", "release-notes"),
			warn: agentfiles.WarnSkillLink,
		},
		{
			name: "проектный навык Claude с полями, которых нет у Codex", from: e.findIn(t, model.KindSkill, "deploy-preview", "deploy-preview"),
			key: "project-agents", agent: model.AgentCodex, dir: filepath.Join(e.home, "projects", "demo", ".agents", "skills", "deploy-preview"),
			files: []string{"SKILL.md"}, warn: agentfiles.WarnSkillFields,
		},
		{
			name: "навык Codex в профиль Claude", from: e.findIn(t, model.KindSkill, "lint-fix", "lint-fix"),
			key: "profile:" + agentfiles.ClaudeFilesID(filepath.Join(e.home, ".claude")), agent: model.AgentClaude,
			dir: filepath.Join(e.home, ".claude", "skills", "lint-fix"), files: []string{"SKILL.md", "agents/openai.yaml"},
			warn: agentfiles.WarnSkillOpenAI,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			targets, err := e.plans.SkillTargets(ctx, tt.from.ID)
			if err != nil {
				t.Fatal(err)
			}
			var keys []string
			for _, tg := range targets {
				keys = append(keys, tg.Key)
				if tg.Key == tt.key && (tg.Dir != tt.dir || tg.Exists || tg.Agent != tt.agent) {
					t.Errorf("цель %+v", tg)
				}
			}
			if !strings.Contains(strings.Join(keys, ","), tt.key) {
				t.Fatalf("нет цели %s среди %v", tt.key, keys)
			}
			p, err := e.plans.InstallSkill(ctx, tt.from.ID, tt.key, tt.link)
			if err != nil {
				t.Fatal(err)
			}
			if p.Op.Code != "skill.install" || warningCodes(p) != tt.warn {
				t.Errorf("операция %s, предупреждения %q, want %q", p.Op.Code, warningCodes(p), tt.warn)
			}
			res := e.apply(t)(p, nil)
			if tt.link {
				if got, err := os.Readlink(tt.dir); err != nil || got == "" {
					t.Fatalf("симлинк: %q %v", got, err)
				}
			}
			for _, f := range tt.files {
				if _, err := os.Stat(filepath.Join(tt.dir, filepath.FromSlash(f))); err != nil {
					t.Errorf("нет %s: %v", f, err)
				}
			}
			// На Windows бита исполнения нет: os.Stat всегда даёт rw-rw-rw-.
			if runtime.GOOS != "windows" && strings.Contains(strings.Join(tt.files, ","), "last-tag.sh") {
				if fi, _ := os.Stat(filepath.Join(tt.dir, "scripts", "last-tag.sh")); fi == nil || fi.Mode().Perm()&0o100 == 0 {
					t.Error("скрипт навыка потерял права на исполнение")
				}
			}
			if _, ok := e.skillOf(t, tt.agent, tt.from.Title); !ok {
				t.Errorf("навык %s не виден у %s после установки", tt.from.Title, tt.agent)
			}
			// Повторная установка в то же место — отказ.
			if _, err := e.plans.InstallSkill(ctx, tt.from.ID, tt.key, tt.link); errCode(err) != "skill.exists" {
				t.Errorf("повторная установка: %v", err)
			}

			e.apply(t)(e.history.Revert(ctx, res.Commit, ""))
			if agentfiles.Occupied(tt.dir) {
				t.Error("откат не убрал навык")
			}
			if _, ok := e.skillOf(t, tt.agent, tt.from.Title); ok && tt.agent == model.AgentCodex && tt.from.Agent == model.AgentClaude {
				t.Error("после отката навык всё ещё виден")
			}
		})
	}

	// После отката копии остаются пустые каталоги — симлинк встаёт на их
	// место (найдено на приёмке фазы 4).
	notes := e.findIn(t, model.KindSkill, "release-notes", "release-notes")
	res := e.apply(t)(e.plans.InstallSkill(ctx, notes.ID, "agents", false))
	e.apply(t)(e.history.Revert(ctx, res.Commit, ""))
	dir := filepath.Join(e.home, ".agents", "skills", "release-notes")
	if _, err := os.Stat(filepath.Join(dir, "scripts")); err != nil {
		t.Fatalf("ожидались пустые каталоги после отката копии: %v", err)
	}
	res = e.apply(t)(e.plans.InstallSkill(ctx, notes.ID, "agents", true))
	if _, err := os.Readlink(dir); err != nil {
		t.Fatalf("симлинк на месте пустых каталогов: %v", err)
	}
	e.apply(t)(e.history.Revert(ctx, res.Commit, ""))
	if _, err := os.Lstat(dir); !os.IsNotExist(err) {
		t.Errorf("после отката симлинка осталось: %v", err)
	}

	if _, err := e.plans.InstallSkill(ctx, e.findIn(t, model.KindSkill, "release-notes", "release-notes").ID, "bogus", false); errCode(err) != "skill.target-invalid" {
		t.Errorf("чужой ключ: %v", err)
	}
	if _, err := e.plans.SkillTargets(ctx, e.findIn(t, model.KindInstructions, "AGENTS.md", ".codex").ID); errCode(err) != "skill.not-skill" {
		t.Errorf("цели для инструкций: %v", err)
	}
}
