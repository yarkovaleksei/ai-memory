// Package adapter — общий контракт адаптеров памяти и помощники для них.
//
// Адаптер знает формат одного агента: находит его хранилища, читает записи
// и говорит, за какими каталогами следить. Адаптеры только читают диск.
package adapter

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/yarkovaleksei/ai-memory/internal/model"
)

// Adapter — контракт адаптера одного агента.
type Adapter interface {
	// Discover находит хранилища памяти. Отсутствующее хранилище — не ошибка.
	Discover(ctx context.Context) ([]model.Source, error)
	// Scan читает все записи хранилища. Испорченный файл даёт запись с Warnings,
	// а не ошибку: ошибка — только когда хранилище нельзя прочитать вообще.
	Scan(ctx context.Context, src model.Source) ([]model.Item, error)
	// WatchDirs — каталоги, изменения в которых должны пересобрать индекс.
	WatchDirs(src model.Source) []string
}

// RelPath — путь файла относительно корня хранилища в виде со слэшами:
// годится для стабильного ID на любой ОС.
func RelPath(root, file string) string {
	rel, err := filepath.Rel(root, file)
	if err != nil {
		return filepath.ToSlash(file)
	}
	return filepath.ToSlash(rel)
}

// tempRoots — каталоги, проекты внутри которых считаются временными (Q7).
func tempRoots() []string {
	roots := []string{os.TempDir(), "/tmp", "/var/tmp", "/private/tmp", "/private/var/folders"}
	out := roots[:0]
	for _, r := range roots {
		if r != "" {
			out = append(out, filepath.Clean(r))
		}
	}
	return out
}

// IsTempPath сообщает, лежит ли путь во временном каталоге системы.
func IsTempPath(p string) bool {
	if p == "" {
		return false
	}
	p = filepath.Clean(p)
	fold := runtime.GOOS == "windows" || runtime.GOOS == "darwin"
	for _, root := range tempRoots() {
		a, b := p, root
		if fold {
			a, b = strings.ToLower(a), strings.ToLower(b)
		}
		if a == b || strings.HasPrefix(a, strings.TrimSuffix(b, string(filepath.Separator))+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

var wikiLink = regexp.MustCompile(`\[\[([^\[\]\n]+)\]\]`)

// WikiLinks находит ссылки вида [[slug]] без повторов, в порядке появления.
func WikiLinks(text string) []string {
	var out []string
	seen := map[string]bool{}
	for _, m := range wikiLink.FindAllStringSubmatch(text, -1) {
		s := strings.TrimSpace(m[1])
		if s != "" && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}
