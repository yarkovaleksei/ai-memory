//go:build !windows

package codex

import (
	"errors"
	"io/fs"
	"os"
	"syscall"
)

// tryLock берёт неблокирующий эксклюзивный flock, как Rust File::try_lock в
// Codex. Файла нет — блокировать нечего (его создаёт сам Codex; мы не создаём
// ничего под CODEX_HOME, кроме заметок).
func tryLock(path string) (unlock func(), busy bool, err error) {
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if errors.Is(err, fs.ErrNotExist) {
		return func() {}, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, true, nil
		}
		return nil, false, err
	}
	return func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		f.Close()
	}, false, nil
}
