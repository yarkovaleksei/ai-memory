// Package writer — единственный путь записи в память агентов:
// план → проверки → снимок в бэкап → атомарная запись → коммит бэкапа.
//
// План строят адаптеры (internal/adapter/claude, internal/adapter/codex): это
// список файлов с ожидаемым текущим содержимым и новым. Writer ничего не знает
// о форматах памяти; он отвечает за то, чтобы запись была безопасной и обратимой.
package writer

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/yarkovaleksei/ai-memory/internal/backup"
)

// Change — изменение одного файла. Before — то, что пользователь видел при
// построении плана: если на диске уже другое, запись отклоняется.
type Change struct {
	Path         string `json:"path"` // абсолютный путь
	BeforeExists bool   `json:"beforeExists"`
	Before       string `json:"before"`
	AfterExists  bool   `json:"afterExists"`
	After        string `json:"after"`
}

// Op — что делает план, кодом с параметрами: интерфейс и история собирают
// текст по словарю (ADR-008).
type Op struct {
	Code   string            `json:"code"` // "claude.fact.create", "codex.note.forget", "revert"…
	Params map[string]string `json:"params,omitempty"`
}

// OpRevert — код операции отката (PlanRevert).
const OpRevert = "revert"

// Plan — набор изменений, которые применяются вместе.
type Plan struct {
	ID      string   `json:"id"`
	Op      Op       `json:"op"`
	Changes []Change `json:"changes"`
	// Warnings — о чём предупредить перед записью: файл в чужом git-репозитории,
	// общий навык (ADR-010). Writer их не проверяет, их показывает интерфейс.
	Warnings []Warning `json:"warnings,omitempty"`
}

// Warning — предупреждение плана кодом с параметрами (ADR-008). Confirm —
// записать можно только после отдельного подтверждения пользователя.
type Warning struct {
	Code    string            `json:"code"`
	Params  map[string]string `json:"params,omitempty"`
	Confirm bool              `json:"confirm,omitempty"`
}

// NewPlan даёт плану случайный идентификатор.
func NewPlan(op Op, changes ...Change) Plan {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return Plan{ID: hex.EncodeToString(b[:]), Op: op, Changes: changes}
}

// Combine собирает несколько планов в одну операцию: перенос — это создание
// записи и удаление (или «забыть») исходной, и откатываться они должны вместе.
// Планы, которые меняют один и тот же файл, не сочетаются: каждый строился от
// состояния диска, а не от результата другого.
func Combine(op Op, plans ...Plan) (Plan, error) {
	var changes []Change
	var warnings []Warning
	seen := map[string]bool{}
	for _, p := range plans {
		warnings = append(warnings, p.Warnings...)
		for _, c := range p.Changes {
			key := pathKey(c.Path)
			if seen[key] {
				return Plan{}, Fail("write.path-duplicate", "path", c.Path)
			}
			seen[key] = true
			changes = append(changes, c)
		}
	}
	out := NewPlan(op, changes...)
	out.Warnings = warnings
	return out, nil
}

// Error — отказ writer'а кодом с параметрами.
type Error struct {
	Code   string
	Params map[string]string
}

func (e *Error) Error() string {
	if len(e.Params) == 0 {
		return e.Code
	}
	return fmt.Sprintf("%s %v", e.Code, e.Params)
}

// Fail создаёт Error: Fail("write.conflict", "path", p).
func Fail(code string, kv ...string) *Error {
	e := &Error{Code: code}
	if len(kv) > 1 {
		e.Params = map[string]string{}
		for i := 0; i+1 < len(kv); i += 2 {
			e.Params[kv[i]] = kv[i+1]
		}
	}
	return e
}

// Guard — дополнительная проверка плана, знающая об агенте (например, что Codex
// сейчас не консолидирует память). Acquire держит блокировки агента до вызова
// release, который writer делает после записи.
type Guard interface {
	Acquire(ctx context.Context, p Plan) (release func(), err error)
}

// Writer применяет планы.
type Writer struct {
	Roots  []string // разрешённые корни: профили Claude, CODEX_HOME, ~/.agents, каталоги проектов, цели симлинков навыков
	Backup *backup.Repo
	Guards []Guard
}

// Result — итог применения.
type Result struct {
	Commit string `json:"commit"` // коммит операции в репозитории бэкапов
}

// opMeta — метаданные коммита операции: по ним строятся история и откат.
type opMeta struct {
	Plan  string   `json:"plan"`
	Op    Op       `json:"op"`
	Paths []string `json:"paths"`
}

// Apply применяет план. Либо все файлы записаны и операция в истории, либо на
// диске ничего не изменилось и возвращается ошибка.
func (w *Writer) Apply(ctx context.Context, p Plan) (Result, error) {
	if len(p.Changes) == 0 {
		return Result{}, Fail("write.empty-plan")
	}
	if err := w.validatePaths(p); err != nil {
		return Result{}, err
	}
	var releases []func()
	defer func() {
		for i := len(releases) - 1; i >= 0; i-- {
			releases[i]()
		}
	}()
	for _, g := range w.Guards {
		release, err := g.Acquire(ctx, p)
		if err != nil {
			return Result{}, err
		}
		if release != nil {
			releases = append(releases, release)
		}
	}
	// Состояние проверяется уже под блокировками агентов.
	modes := make([]fs.FileMode, len(p.Changes))
	for i, c := range p.Changes {
		mode, err := checkState(c)
		if err != nil {
			return Result{}, err
		}
		modes[i] = mode
	}
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}

	before := make([]backup.File, len(p.Changes))
	after := make([]backup.File, len(p.Changes))
	paths := make([]string, len(p.Changes))
	for i, c := range p.Changes {
		before[i] = backup.File{Path: c.Path, Exists: c.BeforeExists, Content: []byte(c.Before)}
		after[i] = backup.File{Path: c.Path, Exists: c.AfterExists, Content: []byte(c.After)}
		paths[i] = c.Path
	}
	if _, err := w.Backup.Record(ctx, before, "Снимок перед "+p.Op.Code, nil); err != nil {
		return Result{}, Fail("backup.failed", "error", err.Error())
	}

	for i, c := range p.Changes {
		if err := apply(c, modes[i]); err != nil {
			// Возвращаем уже записанные файлы: план применяется целиком или никак.
			for j := i - 1; j >= 0; j-- {
				_ = apply(invert(p.Changes[j]), modes[j])
			}
			return Result{}, err
		}
	}

	commit, err := w.Backup.Record(ctx, after, p.Op.Code, opMeta{Plan: p.ID, Op: p.Op, Paths: paths})
	if err != nil {
		// Файлы уже записаны, снимок «до» в бэкапе есть; нет только записи в истории.
		return Result{}, Fail("backup.record-failed", "error", err.Error())
	}
	return Result{Commit: commit}, nil
}

func invert(c Change) Change {
	return Change{Path: c.Path, BeforeExists: c.AfterExists, Before: c.After, AfterExists: c.BeforeExists, After: c.Before}
}

// validatePaths: абсолютный путь, без повторов, внутри разрешённых корней, сам
// файл и путь к нему не уходят симлинками наружу.
func (w *Writer) validatePaths(p Plan) error {
	seen := map[string]bool{}
	for _, c := range p.Changes {
		if !filepath.IsAbs(c.Path) || filepath.Clean(c.Path) != c.Path {
			return Fail("write.path-invalid", "path", c.Path)
		}
		key := pathKey(c.Path)
		if seen[key] {
			return Fail("write.path-duplicate", "path", c.Path)
		}
		seen[key] = true
		if fi, err := os.Lstat(c.Path); err == nil && fi.Mode()&fs.ModeSymlink != 0 {
			return Fail("write.symlink", "path", c.Path)
		} else if err == nil && !fi.Mode().IsRegular() {
			return Fail("write.not-regular", "path", c.Path)
		}
		if !w.insideRoots(c.Path) {
			return Fail("write.outside-roots", "path", c.Path)
		}
	}
	return nil
}

func (w *Writer) insideRoots(path string) bool {
	real := resolveExisting(path)
	for _, root := range w.Roots {
		r := resolveExisting(filepath.Clean(filepath.FromSlash(root)))
		if within(pathKey(r), pathKey(real)) {
			return true
		}
	}
	return false
}

// resolveExisting раскрывает симлинки у самой глубокой существующей части пути и
// приклеивает остаток: новый файл в существующем каталоге проверяется по
// настоящему месту каталога.
func resolveExisting(p string) string {
	rest := ""
	cur := p
	for {
		if r, err := filepath.EvalSymlinks(cur); err == nil {
			return filepath.Join(r, rest)
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return p
		}
		rest = filepath.Join(filepath.Base(cur), rest)
		cur = parent
	}
}

func within(root, p string) bool {
	rel, err := filepath.Rel(root, p)
	return err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func pathKey(p string) string {
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		return strings.ToLower(p)
	}
	return p
}

// checkState сверяет диск с тем, что видел пользователь, и возвращает права файла.
func checkState(c Change) (fs.FileMode, error) {
	data, err := os.ReadFile(c.Path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		if c.BeforeExists {
			return 0, Fail("write.conflict", "path", c.Path, "reason", "deleted")
		}
		return 0o644, nil
	case err != nil:
		return 0, Fail("write.unreadable", "path", c.Path, "error", err.Error())
	}
	if !c.BeforeExists {
		return 0, Fail("write.conflict", "path", c.Path, "reason", "created")
	}
	if !bytes.Equal(data, []byte(c.Before)) {
		return 0, Fail("write.conflict", "path", c.Path, "reason", "changed")
	}
	fi, err := os.Stat(c.Path)
	if err != nil {
		return 0, Fail("write.unreadable", "path", c.Path, "error", err.Error())
	}
	return fi.Mode().Perm(), nil
}
