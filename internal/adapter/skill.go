package adapter

import (
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"unicode/utf8"

	"github.com/yarkovaleksei/ai-memory/internal/markdown"
	"github.com/yarkovaleksei/ai-memory/internal/model"
)

// SkillFile — файл навыка: frontmatter с name и description, дальше Markdown.
const SkillFile = "SKILL.md"

// codexMaxSkillName — предел длины name у Codex: длиннее он навык не грузит
// (storage-formats.md, «Навыки Codex»).
const codexMaxSkillName = 64

// SkillDirs — подкаталоги root, в которых есть SKILL.md, в порядке имён.
// Скрытые каталоги пропускаются: у Codex там системные навыки, у Claude —
// служебные файлы; их читают отдельно.
func SkillDirs(root string) []string {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		dir := filepath.Join(root, e.Name())
		// Stat, а не тип записи каталога: навык часто подключён симлинком.
		if fi, err := os.Stat(filepath.Join(dir, SkillFile)); err == nil && fi.Mode().IsRegular() {
			out = append(out, dir)
		}
	}
	return out
}

// ReadSkill читает навык из каталога dir. ok=false — в каталоге нет SKILL.md.
// Испорченный frontmatter даёт запись с предупреждением, а не ошибку.
func ReadSkill(src model.Source, dir, role string, write model.WriteMode, ref model.ProjectRef) (model.Item, bool) {
	file := filepath.Join(dir, SkillFile)
	data, err := os.ReadFile(file)
	if err != nil {
		return model.Item{}, false
	}
	fi, err := os.Stat(file)
	if err != nil {
		return model.Item{}, false
	}
	content := markdown.Normalize(string(data))
	it := model.Item{
		ID:       model.ItemID(src.ID, RelPath(src.Root, file), ""),
		SourceID: src.ID,
		Agent:    src.Agent,
		Kind:     model.KindSkill,
		Role:     role,
		Title:    filepath.Base(dir),
		File:     file,
		ModTime:  fi.ModTime(),
		Hash:     model.ContentHash(data),
		Write:    write,
		Project:  ref,
	}
	if real, err := filepath.EvalSymlinks(dir); err == nil {
		it.RealDir = real
		// Симлинком подключён сам каталог навыка, а не кто-то из его предков:
		// временный каталог macOS лежит под /var → /private/var, а на Windows
		// путь бывает записан коротким именем (RUNNER~1). Поэтому настоящий
		// путь сравнивается с настоящим путём родителя плюс имя каталога.
		if parent, err := filepath.EvalSymlinks(filepath.Dir(dir)); err == nil &&
			!samePath(real, filepath.Join(parent, filepath.Base(dir))) {
			it.Target = real
		}
	}

	fmText, body, has := markdown.SplitFrontmatter(content)
	it.Body = body
	if !has {
		it.Warnings = append(it.Warnings, model.NewWarning("skill.frontmatter.missing"))
	} else if fm, err := markdown.ParseFrontmatter(fmText); err != nil {
		it.Warnings = append(it.Warnings, model.NewWarning("skill.frontmatter.invalid", "error", err.Error()))
	} else {
		it.Frontmatter = fm
		if n := markdown.String(fm, "name"); n != "" {
			it.Title = n
		}
		it.Description = markdown.String(fm, "description")
	}

	if src.Agent == model.AgentCodex && write != model.WriteNone {
		it.Warnings = append(it.Warnings, codexSkillWarnings(it, file)...)
	}
	return it, true
}

// codexSkillWarnings — то, из-за чего Codex навык не увидит или увидит не так:
// без description и с длинным name он навык не грузит, симлинк на сам SKILL.md
// пропускает при обходе.
func codexSkillWarnings(it model.Item, file string) []model.Warning {
	var out []model.Warning
	if it.Frontmatter != nil && strings.TrimSpace(it.Description) == "" {
		out = append(out, model.NewWarning("codex.skill.no-description"))
	}
	if n := markdown.String(it.Frontmatter, "name"); utf8.RuneCountInString(n) > codexMaxSkillName {
		out = append(out, model.NewWarning("codex.skill.name-too-long", "max", "64"))
	}
	if fi, err := os.Lstat(file); err == nil && fi.Mode()&fs.ModeSymlink != 0 {
		out = append(out, model.NewWarning("codex.skill.file-symlink"))
	}
	return out
}

// samePath сравнивает пути с учётом регистра файловой системы ОС.
func samePath(a, b string) bool {
	a, b = filepath.Clean(a), filepath.Clean(b)
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

// PathKey — путь для сравнения: на macOS и Windows файловые системы по
// умолчанию нечувствительны к регистру.
func PathKey(p string) string {
	p = filepath.Clean(p)
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		return strings.ToLower(p)
	}
	return p
}

// IsDir сообщает, что путь существует и это каталог (симлинки раскрываются).
func IsDir(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}

// IsFile сообщает, что путь существует и это обычный файл (симлинки раскрываются).
func IsFile(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.Mode().IsRegular()
}
