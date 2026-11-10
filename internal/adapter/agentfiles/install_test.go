package agentfiles

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yarkovaleksei/ai-memory/internal/model"
)

func TestPlanInstallSkill(t *testing.T) {
	home := fixtureHome(t)
	skill := func(dir string) model.Item {
		return model.Item{Kind: model.KindSkill, Title: filepath.Base(dir), File: filepath.Join(dir, "SKILL.md")}
	}
	noDesc := skill(filepath.Join(home, ".codex", "skills", "no-description"))
	deploy := skill(filepath.Join(home, "projects", "demo", ".claude", "skills", "deploy-preview"))
	lint := skill(filepath.Join(home, ".codex", "skills", "lint-fix"))

	// Навык с симлинком внутри и длинным именем — во временном каталоге.
	tmp := t.TempDir()
	long := filepath.Join(tmp, "long")
	write(t, filepath.Join(long, "SKILL.md"), "---\nname: "+strings.Repeat("x", 65)+"\ndescription: d\n---\n")
	write(t, filepath.Join(long, ".git", "HEAD"), "ref")
	if err := os.Symlink(filepath.Join(long, "SKILL.md"), filepath.Join(long, "alias.md")); err != nil {
		t.Skipf("симлинки недоступны: %v", err)
	}

	tests := []struct {
		name  string
		c     SkillInstall
		paths int
		warn  string
		fail  string
	}{
		{"Claude → Codex: поля, которых Codex не знает", SkillInstall{From: deploy, Dir: filepath.Join(t.TempDir(), "deploy-preview"), Agent: model.AgentCodex}, 1, WarnSkillFields, ""},
		{"без description Codex навык не загрузит", SkillInstall{From: noDesc, Dir: filepath.Join(t.TempDir(), "x"), Agent: model.AgentCodex}, 1, WarnSkillNoDesc, ""},
		{"Codex → Claude: openai.yaml", SkillInstall{From: lint, Dir: filepath.Join(t.TempDir(), "lint-fix"), Agent: model.AgentClaude}, 2, WarnSkillOpenAI, ""},
		{"симлинк вместо копии", SkillInstall{From: lint, Dir: filepath.Join(t.TempDir(), "lint-fix"), Agent: model.AgentClaude, Link: true}, 1, WarnSkillOpenAI + "," + WarnSkillLink, ""},
		{"длинное имя, симлинк внутри, .git не копируется", SkillInstall{From: skill(long), Dir: filepath.Join(t.TempDir(), "long"), Agent: model.AgentCodex}, 1, WarnSkillNameLong + "," + WarnSkillLinksInside, ""},
		{"цель занята", SkillInstall{From: lint, Dir: filepath.Join(home, ".codex", "skills", "lint-fix"), Agent: model.AgentClaude}, 0, "", "skill.exists"},
		{"не навык", SkillInstall{From: model.Item{Kind: model.KindInstructions}}, 0, "", "skill.not-skill"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := PlanInstallSkill(tt.c)
			if tt.fail != "" {
				if failCode(err) != tt.fail {
					t.Fatalf("ошибка %v, want %s", err, tt.fail)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(p.Changes) != tt.paths || warnCodes(p.Warnings) != tt.warn {
				t.Errorf("файлов %d, предупреждения %q; want %d, %q", len(p.Changes), warnCodes(p.Warnings), tt.paths, tt.warn)
			}
			if !tt.c.Link && filepath.Base(p.Changes[0].Path) != "SKILL.md" {
				t.Errorf("первым должен идти SKILL.md: %s", p.Changes[0].Path)
			}
			if tt.c.Link && p.Changes[0].Link == "" {
				t.Errorf("не симлинк: %+v", p.Changes[0])
			}
		})
	}
}
