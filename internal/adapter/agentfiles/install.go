package agentfiles

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/yarkovaleksei/ai-memory/internal/adapter"
	"github.com/yarkovaleksei/ai-memory/internal/markdown"
	"github.com/yarkovaleksei/ai-memory/internal/model"
	"github.com/yarkovaleksei/ai-memory/internal/writer"
)

// Установка навыка одного агента в другого (ADR-023): копией каталога или
// симлинком на него. Поля frontmatter, которых второй агент не понимает, план
// перечисляет, но не трогает: SKILL.md копируется байт в байт.

// Коды предупреждений плана установки навыка.
const (
	WarnSkillFields      = "plan.skill-fields-ignored" // поля, которые агент молча проигнорирует
	WarnSkillNoDesc      = "plan.skill-no-description" // Codex не загрузит навык без description
	WarnSkillNameLong    = "plan.skill-name-long"      // Codex обрежет или не примет имя длиннее 64
	WarnSkillOpenAI      = "plan.skill-openai-yaml"    // agents/openai.yaml Claude не читает
	WarnSkillLink        = "plan.skill-link"           // навык станет общим для обоих агентов
	WarnSkillLinksInside = "plan.skill-symlinks-skipped"
)

// Пределы копии: навык — это инструкции и несколько скриптов, а не проект.
const (
	maxSkillFiles = 200
	maxSkillBytes = 4 << 20
)

// Поля frontmatter SKILL.md, которые агент понимает (storage-formats.md).
var skillFields = map[model.Agent]map[string]bool{
	model.AgentClaude: set("name", "description", "when_to_use", "argument-hint", "arguments",
		"disable-model-invocation", "user-invocable", "allowed-tools", "disallowed-tools", "model", "effort",
		"context", "agent", "background", "hooks", "paths", "shell", "metadata", "license", "compatibility"),
	model.AgentCodex: set("name", "description", "metadata"),
}

func set(keys ...string) map[string]bool {
	m := make(map[string]bool, len(keys))
	for _, k := range keys {
		m[k] = true
	}
	return m
}

// SkillInstall — какой навык и куда ставится.
type SkillInstall struct {
	From  model.Item  // навык-источник
	Dir   string      // каталог навыка в цели: <корень навыков>/<имя>
	Agent model.Agent // агент цели
	Link  bool        // симлинк вместо копии
}

// PlanInstallSkill строит план установки навыка.
func PlanInstallSkill(c SkillInstall) (writer.Plan, error) {
	if c.From.Kind != model.KindSkill {
		return writer.Plan{}, writer.Fail("skill.not-skill", "title", c.From.Title)
	}
	src, err := filepath.EvalSymlinks(filepath.Dir(c.From.File))
	if err != nil {
		return writer.Plan{}, writer.Fail("write.unreadable", "path", filepath.Dir(c.From.File), "error", err.Error())
	}
	if Occupied(c.Dir) {
		return writer.Plan{}, writer.Fail("skill.exists", "dir", c.Dir)
	}
	if adapter.PathKey(src) == adapter.PathKey(c.Dir) {
		return writer.Plan{}, writer.Fail("skill.exists", "dir", c.Dir)
	}
	skillMD, err := os.ReadFile(filepath.Join(src, adapter.SkillFile))
	if err != nil {
		return writer.Plan{}, writer.Fail("write.unreadable", "path", filepath.Join(src, adapter.SkillFile), "error", err.Error())
	}

	var changes []writer.Change
	var skipped []string
	if c.Link {
		changes = append(changes, writer.Change{Path: c.Dir, AfterExists: true, Link: src})
	} else {
		total := 0
		err := filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			rel, _ := filepath.Rel(src, p)
			switch {
			case d.IsDir():
				// Служебные каталоги (.git и прочие) навыку не нужны.
				if p != src && strings.HasPrefix(d.Name(), ".") {
					return filepath.SkipDir
				}
				return nil
			case d.Type()&fs.ModeSymlink != 0 || !d.Type().IsRegular():
				// Симлинк внутри навыка ведёт неизвестно куда: копия его не повторяет.
				skipped = append(skipped, filepath.ToSlash(rel))
				return nil
			}
			fi, err := d.Info()
			if err != nil {
				return err
			}
			total += int(fi.Size())
			if len(changes) >= maxSkillFiles || total > maxSkillBytes {
				return writer.Fail("skill.too-large", "files", strconv.Itoa(maxSkillFiles), "bytes", strconv.Itoa(maxSkillBytes))
			}
			data, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			changes = append(changes, writer.Change{Path: filepath.Join(c.Dir, rel), AfterExists: true,
				After: string(data), Exec: fi.Mode().Perm()&0o111 != 0})
			return nil
		})
		if err != nil {
			if we, ok := err.(*writer.Error); ok {
				return writer.Plan{}, we
			}
			return writer.Plan{}, writer.Fail("write.unreadable", "path", src, "error", err.Error())
		}
		// SKILL.md — первым: его diff пользователь смотрит в первую очередь.
		sort.SliceStable(changes, func(i, j int) bool {
			return filepath.Base(changes[i].Path) == adapter.SkillFile && filepath.Dir(changes[i].Path) == c.Dir
		})
	}

	mode := "copy"
	if c.Link {
		mode = "link"
	}
	name := filepath.Base(c.Dir)
	p := writer.NewPlan(writer.Op{Code: "skill.install", Params: map[string]string{
		"name": name, "agent": string(c.Agent), "mode": mode,
	}}, changes...)
	p.Warnings = installWarnings(c, src, string(skillMD), skipped)
	return p, nil
}

// Occupied: на месте каталога навыка уже что-то есть — симлинк, файл или
// каталог с файлами. Пустые каталоги не в счёт: их оставляет откат копии,
// который удаляет файлы, но не каталоги.
func Occupied(dir string) bool {
	fi, err := os.Lstat(dir)
	if err != nil {
		return false
	}
	if !fi.IsDir() {
		return true
	}
	found := false
	_ = filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			found = true
			return filepath.SkipAll
		}
		return nil
	})
	return found
}

func installWarnings(c SkillInstall, src, skillMD string, skipped []string) []writer.Warning {
	var out []writer.Warning
	add := func(code string, kv ...string) {
		w := writer.Warning{Code: code, Params: map[string]string{"agent": string(c.Agent)}}
		for i := 0; i+1 < len(kv); i += 2 {
			w.Params[kv[i]] = kv[i+1]
		}
		out = append(out, w)
	}
	fm := map[string]any{}
	if text, _, has := markdown.SplitFrontmatter(skillMD); has {
		if m, err := markdown.ParseFrontmatter(text); err == nil {
			fm = m
		}
	}
	var ignored []string
	for k := range fm {
		if !skillFields[c.Agent][k] {
			ignored = append(ignored, k)
		}
	}
	sort.Strings(ignored)
	if len(ignored) > 0 {
		add(WarnSkillFields, "fields", strings.Join(ignored, ", "))
	}
	if c.Agent == model.AgentCodex {
		if strings.TrimSpace(markdown.String(fm, "description")) == "" {
			add(WarnSkillNoDesc)
		}
		if n := markdown.String(fm, "name"); len([]rune(n)) > 64 {
			add(WarnSkillNameLong, "name", n)
		}
	}
	if c.Agent == model.AgentClaude && adapter.IsFile(filepath.Join(src, "agents", "openai.yaml")) {
		add(WarnSkillOpenAI)
	}
	if len(skipped) > 0 {
		add(WarnSkillLinksInside, "files", strings.Join(skipped, ", "))
	}
	if c.Link {
		add(WarnSkillLink, "target", src)
	}
	return append(out, Warnings(model.Item{Repo: repoRoot(filepath.Dir(c.Dir))})...)
}
