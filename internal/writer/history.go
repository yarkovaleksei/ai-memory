package writer

import (
	"context"
	"encoding/json"
	"time"
)

// Entry — операция в истории.
type Entry struct {
	Commit string    `json:"commit"`
	Time   time.Time `json:"time"`
	Op     Op        `json:"op"`
	Paths  []string  `json:"paths"`
}

// History возвращает последние операции, новые первыми.
func (w *Writer) History(ctx context.Context, limit int) ([]Entry, error) {
	commits, err := w.Backup.Log(ctx, limit)
	if err != nil {
		return nil, err
	}
	out := make([]Entry, 0, len(commits))
	for _, c := range commits {
		var m opMeta
		if json.Unmarshal(c.Meta, &m) != nil {
			continue
		}
		out = append(out, Entry{Commit: c.Hash, Time: c.Time, Op: m.Op, Paths: m.Paths})
	}
	return out, nil
}

// PlanRevert строит план отката операции: вернуть файлам состояние до неё.
// Это обычный план — с diff, проверкой, что файлы с тех пор не менялись, и
// записью в историю; откат тоже можно откатить.
func (w *Writer) PlanRevert(ctx context.Context, commit string) (Plan, error) {
	raw, err := w.Backup.Meta(ctx, commit)
	if err != nil {
		return Plan{}, Fail("history.not-found", "commit", commit)
	}
	var m opMeta
	if err := json.Unmarshal(raw, &m); err != nil {
		return Plan{}, Fail("history.not-found", "commit", commit)
	}
	var changes []Change
	for _, path := range m.Paths {
		after, err := w.Backup.FileAt(ctx, commit, path)
		if err != nil {
			return Plan{}, Fail("backup.failed", "error", err.Error())
		}
		before, err := w.Backup.FileAt(ctx, commit+"^", path)
		if err != nil {
			return Plan{}, Fail("backup.failed", "error", err.Error())
		}
		changes = append(changes, Change{
			Path:         path,
			BeforeExists: after.Exists, Before: string(after.Content),
			AfterExists: before.Exists, After: string(before.Content),
		})
	}
	return NewPlan(Op{Code: "revert", Params: map[string]string{"commit": commit, "op": m.Op.Code}}, changes...), nil
}
