package agentfiles

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/yarkovaleksei/ai-memory/internal/model"
	"github.com/yarkovaleksei/ai-memory/internal/writer"
)

// Коды предупреждений плана (ADR-010, ADR-020).
const (
	// WarnInRepo — файл лежит в git-репозитории проекта: правка появится в его
	// git status, коммитить её — дело владельца репозитория.
	WarnInRepo = "plan.in-repo"
	// WarnSharedSkill — каталог навыка подключён в нескольких местах или
	// симлинком: правка затронет всех, кто на него ссылается.
	WarnSharedSkill = "plan.shared-skill"
)

// PlanUpdate заменяет файл инструкций или SKILL.md навыка целиком. hash —
// ContentHash, который видел пользователь: если файл с тех пор изменился,
// план не строится.
func PlanUpdate(it model.Item, hash, content string) (writer.Plan, error) {
	if it.Write != model.WriteDirect || (it.Kind != model.KindInstructions && it.Kind != model.KindSkill) {
		return writer.Plan{}, writer.Fail("plan.readonly", "title", it.Title)
	}
	current, err := readExpected(it.File, hash)
	if err != nil {
		return writer.Plan{}, err
	}
	if content == current {
		return writer.Plan{}, writer.Fail("write.no-changes")
	}
	op := writer.Op{Code: "instructions.update", Params: map[string]string{"file": it.Title, "agent": string(it.Agent)}}
	if it.Kind == model.KindSkill {
		op = writer.Op{Code: "skill.update", Params: map[string]string{"name": it.Title, "agent": string(it.Agent)}}
	}
	p := writer.NewPlan(op, writer.Change{Path: it.File, BeforeExists: true, Before: current, AfterExists: true, After: content})
	p.Warnings = Warnings(it)
	return p, nil
}

// Warnings — предупреждения для плана, который меняет файлы записи.
func Warnings(it model.Item) []writer.Warning {
	var out []writer.Warning
	if it.Repo != "" {
		out = append(out, writer.Warning{Code: WarnInRepo, Params: map[string]string{"repo": it.Repo}})
	}
	if it.Kind == model.KindSkill && (it.Shared || it.Target != "") {
		dir := it.Target
		if dir == "" {
			dir = filepath.Dir(it.File)
		}
		out = append(out, writer.Warning{Code: WarnSharedSkill, Params: map[string]string{"dir": dir}, Confirm: true})
	}
	return out
}

func readExpected(file, hash string) (string, error) {
	data, err := os.ReadFile(file)
	if errors.Is(err, fs.ErrNotExist) {
		return "", writer.Fail("write.conflict", "path", file, "reason", "deleted")
	}
	if err != nil {
		return "", writer.Fail("write.unreadable", "path", file, "error", err.Error())
	}
	if hash != "" && model.ContentHash(data) != hash {
		return "", writer.Fail("write.conflict", "path", file, "reason", "changed")
	}
	return string(data), nil
}
