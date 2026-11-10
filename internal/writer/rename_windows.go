//go:build windows

package writer

import (
	"errors"
	"os"
	"syscall"
	"time"
)

// Коды ошибок Windows, когда файл открыт другим процессом (агентом).
const (
	errorAccessDenied     syscall.Errno = 5
	errorSharingViolation syscall.Errno = 32
)

// rename на Windows не заменяет файл, пока его держит открытым другой процесс
// (ADR-009): повторяем с паузой, затем сообщаем, что файл занят.
func rename(from, to string) error {
	var err error
	for attempt := 0; attempt < 10; attempt++ {
		if err = os.Rename(from, to); err == nil {
			return nil
		}
		var errno syscall.Errno
		if !errors.As(err, &errno) || (errno != errorAccessDenied && errno != errorSharingViolation) {
			return err
		}
		time.Sleep(100 * time.Millisecond)
	}
	return Fail("write.busy", "path", to, "error", err.Error())
}
