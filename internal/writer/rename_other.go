//go:build !windows

package writer

import "os"

// rename на Linux и macOS атомарно заменяет файл, даже если его держит открытым
// другой процесс.
func rename(from, to string) error { return os.Rename(from, to) }
