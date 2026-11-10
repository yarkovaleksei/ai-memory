//go:build windows

package codex

import (
	"errors"
	"io/fs"
	"os"

	"golang.org/x/sys/windows"
)

// tryLock берёт неблокирующую эксклюзивную блокировку LockFileEx, как Rust
// File::try_lock в Codex на Windows.
func tryLock(path string) (unlock func(), busy bool, err error) {
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if errors.Is(err, fs.ErrNotExist) {
		return func() {}, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	h := windows.Handle(f.Fd())
	var ol windows.Overlapped
	err = windows.LockFileEx(h, windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, &ol)
	if err != nil {
		f.Close()
		if errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
			return nil, true, nil
		}
		return nil, false, err
	}
	return func() {
		_ = windows.UnlockFileEx(h, 0, 1, 0, &ol)
		f.Close()
	}, false, nil
}
