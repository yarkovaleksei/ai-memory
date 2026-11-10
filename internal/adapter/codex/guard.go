package codex

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/yarkovaleksei/ai-memory/internal/writer"
)

// Guard — защита памяти Codex для writer'а (ADR-002, ADR-020):
//   - под CODEX_HOME меняются только *.md в extensions/ad_hoc/notes/ активного
//     поколения, а из того, что не память, — AGENTS.md, AGENTS.override.md и
//     пользовательские навыки в skills/ (кроме системных skills/.system),
//     в том числе симлинк каталога навыка;
//     сгенерированные файлы Codex — никогда;
//   - файл заметки не удаляется никогда; файл инструкций или навыка удаляет
//     только откат операции, которая его создала (ADR-021);
//   - запись не идёт во время консолидации: на время записи берём тот же
//     файловый lock, что Codex 0.167 держит при консолидации, и проверяем
//     строку задачи в базе (0.159 файлового lock не берёт).
type Guard struct {
	Home       string
	Generation Generation
	now        func() time.Time
}

var _ writer.Guard = Guard{}

// Acquire проверяет план и держит блокировку Codex до записи.
func (g Guard) Acquire(ctx context.Context, p writer.Plan) (func(), error) {
	home := filepath.Clean(filepath.FromSlash(g.Home))
	notes := filepath.Join(home, g.Generation.Dir(), notesDir)
	touches := false
	for _, c := range p.Changes {
		if !within(home, c.Path) {
			continue
		}
		if instructionsOrSkill(home, c.Path) || (c.Link != "" || c.BeforeLink != "") && skillDir(home, c.Path) {
			// Не память: консолидация их не трогает, блокировка не нужна.
			// Удаляет их только откат: он возвращает состояние из снимка, то
			// есть убирает файл, который создала сама ai-memory (ADR-021).
			if !c.AfterExists && p.Op.Code != writer.OpRevert {
				return nil, writer.Fail("codex.files.delete-forbidden", "path", c.Path)
			}
			continue
		}
		touches = true
		if filepath.Dir(c.Path) != notes || !strings.EqualFold(filepath.Ext(c.Path), ".md") {
			return nil, writer.Fail("codex.generated-readonly", "path", c.Path)
		}
		if !c.AfterExists {
			return nil, writer.Fail("codex.note.delete-forbidden", "path", c.Path)
		}
	}
	if !touches {
		return nil, nil
	}

	unlock, busy, err := tryLock(filepath.Join(home, g.Generation.LockFile()))
	if err != nil {
		return nil, writer.Fail("codex.lock-failed", "error", err.Error())
	}
	if busy {
		return nil, writer.Fail("codex.consolidation-running")
	}
	if g.jobRunning(ctx, filepath.Join(SQLiteHome(home), g.Generation.DB())) {
		unlock()
		return nil, writer.Fail("codex.consolidation-running")
	}
	return unlock, nil
}

// jobRunning: задача консолидации в статусе running с непросроченной арендой.
// Упавшая задача держит running до конца аренды (до часа) — это тоже ответ
// «подождите», а не ошибка.
func (g Guard) jobRunning(ctx context.Context, dbPath string) bool {
	if _, err := os.Stat(dbPath); err != nil {
		return false
	}
	db, err := openRO(dbPath)
	if err != nil {
		return false
	}
	defer db.Close()
	var status string
	var lease sql.NullInt64
	err = db.QueryRowContext(ctx,
		`SELECT status, lease_until FROM jobs WHERE kind = 'memory_consolidate_global' AND job_key = 'global'`).
		Scan(&status, &lease)
	if errors.Is(err, sql.ErrNoRows) || err != nil {
		return false
	}
	now := time.Now
	if g.now != nil {
		now = g.now
	}
	return status == "running" && lease.Valid && lease.Int64 > now().Unix()
}

// skillDir — каталог пользовательского навыка CODEX_HOME/skills/<навык>:
// туда ставится симлинк навыка другого агента (ADR-023).
func skillDir(home, p string) bool {
	base := filepath.Base(p)
	return filepath.Dir(p) == filepath.Join(home, "skills") && !strings.HasPrefix(base, ".")
}

// IsSkillDir — то же для сервиса отката.
func IsSkillDir(home, p string) bool { return skillDir(filepath.Clean(home), filepath.Clean(p)) }

// IsInstructionsOrSkill — файл инструкций или пользовательского навыка под
// CODEX_HOME, а не память: его удаление при откате не подменяется заметкой.
func IsInstructionsOrSkill(home, p string) bool {
	return instructionsOrSkill(filepath.Clean(home), filepath.Clean(p))
}

// instructionsOrSkill — файл инструкций в корне CODEX_HOME или файл
// пользовательского навыка в CODEX_HOME/skills/<навык>/. Системные навыки
// Codex перезаписывает сам — их не трогаем.
func instructionsOrSkill(home, p string) bool {
	if filepath.Dir(p) == home {
		base := filepath.Base(p)
		return strings.EqualFold(base, "AGENTS.md") || strings.EqualFold(base, "AGENTS.override.md")
	}
	skills := filepath.Join(home, "skills")
	rel, err := filepath.Rel(skills, p)
	if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
		return false
	}
	parts := strings.Split(rel, string(filepath.Separator))
	return len(parts) >= 2 && !strings.HasPrefix(parts[0], ".")
}

func within(root, p string) bool {
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		root, p = strings.ToLower(root), strings.ToLower(p)
	}
	rel, err := filepath.Rel(root, p)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
