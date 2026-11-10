package library

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"
)

// Debounce — пауза после последнего изменения перед пересборкой: агенты пишут
// память пачками (факт + строка в индексе), пересобирать на каждый файл незачем.
const Debounce = 400 * time.Millisecond

// Watch следит за каталогами памяти и после изменений пересобирает индекс,
// сообщая итог через onUpdate. fsnotify не рекурсивен, поэтому после каждой
// пересборки набор каталогов обновляется: новые проекты подхватываются сами.
// Возвращается, когда ctx отменён.
func (l *Library) Watch(ctx context.Context, onUpdate func(Stats)) error {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("library: наблюдение: %w", err)
	}
	defer w.Close()

	current := map[string]bool{}
	var filters map[string]map[string]bool
	resync := func() {
		want := l.watchDirs()
		filters = want
		for d := range want {
			if !current[d] {
				if err := w.Add(d); err != nil {
					l.log.Debug("каталог не добавлен в наблюдение", "dir", d, "error", err)
					continue
				}
				current[d] = true
			}
		}
		for d := range current {
			if _, ok := want[d]; !ok {
				_ = w.Remove(d)
				delete(current, d)
			}
		}
	}
	resync()

	var (
		timer   *time.Timer
		timerCh <-chan time.Time
	)
	for {
		select {
		case <-ctx.Done():
			return nil
		case ev, ok := <-w.Events:
			if !ok {
				return nil
			}
			if ev.Op == fsnotify.Chmod || !relevant(filters, ev.Name) {
				continue
			}
			if timer == nil {
				timer = time.NewTimer(Debounce)
			} else {
				timer.Reset(Debounce)
			}
			timerCh = timer.C
		case err, ok := <-w.Errors:
			if !ok {
				return nil
			}
			l.log.Warn("ошибка наблюдения за файлами", "error", err)
		case <-timerCh:
			timerCh = nil
			st, err := l.Rebuild(ctx)
			if err != nil {
				l.log.Warn("пересборка индекса не удалась", "error", err)
				continue
			}
			resync()
			if onUpdate != nil {
				onUpdate(st)
			}
		}
	}
}

// relevant — изменение важно для индекса: каталог без фильтра или имя из фильтра.
func relevant(filters map[string]map[string]bool, name string) bool {
	names, ok := filters[filepath.Dir(name)]
	if !ok || names == nil {
		return true
	}
	return names[filepath.Base(name)]
}
