package writer

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// apply записывает одно изменение: новое содержимое — атомарно через временный
// файл в том же каталоге, удаление — os.Remove, симлинк — os.Symlink. Здесь единственные вызовы
// записи и удаления по путям памяти агентов (AGENTS.md).
func apply(c Change, mode fs.FileMode) error {
	if c.Link != "" {
		if err := os.MkdirAll(filepath.Dir(c.Path), 0o755); err != nil {
			return Fail("write.failed", "path", c.Path, "error", err.Error())
		}
		if c.BeforeLink != "" {
			if err := os.Remove(c.Path); err != nil {
				return Fail("write.failed", "path", c.Path, "error", err.Error())
			}
		}
		if err := os.Symlink(c.Link, c.Path); err != nil {
			return Fail("write.failed", "path", c.Path, "error", err.Error())
		}
		return nil
	}
	if !c.AfterExists {
		if err := os.Remove(c.Path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return Fail("write.failed", "path", c.Path, "error", err.Error())
		}
		return nil
	}
	if err := writeAtomic(c.Path, []byte(c.After), mode); err != nil {
		var we *Error
		if errors.As(err, &we) {
			return we // уже с кодом, например write.busy
		}
		return Fail("write.failed", "path", c.Path, "error", err.Error())
	}
	return nil
}

func writeAtomic(path string, data []byte, mode fs.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	// Временный файл в том же каталоге: rename в пределах одной файловой системы
	// атомарен, и агент видит либо старый файл, либо новый целиком.
	tmp, err := os.CreateTemp(dir, ".ai-memory-*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	ok := false
	defer func() {
		if !ok {
			_ = os.Remove(name)
		}
	}()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(name, mode); err != nil {
		return err
	}
	if err := rename(name, path); err != nil {
		return err
	}
	ok = true
	return nil
}
