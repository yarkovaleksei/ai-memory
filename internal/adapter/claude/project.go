package claude

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"unicode"

	"github.com/yarkovaleksei/ai-memory/internal/adapter"
	"github.com/yarkovaleksei/ai-memory/internal/model"
)

// EncodeProjectPath повторяет кодирование Claude Code: каждый символ, кроме
// ASCII-букв и цифр, становится «-» (проверено на 76 каталогах, storage-formats.md).
func EncodeProjectPath(p string) string {
	var b strings.Builder
	for _, r := range p {
		if r < unicode.MaxASCII && (unicode.IsLetter(r) || unicode.IsDigit(r)) {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}
	return b.String()
}

// ResolveProject определяет путь проекта по каталогу Claude: сначала по cwd из
// файлов сессий, затем подбором по диску. Не вышло — остаётся закодированное имя.
func ResolveProject(projDir, encoded string) model.ProjectRef {
	if cwd := cwdFromSessions(projDir); cwd != "" {
		p := filepath.Clean(cwd)
		return model.ProjectRef{Path: p, Temp: adapter.IsTempPath(p)}
	}
	if p, ok := DecodeByDisk(encoded, runtime.GOOS); ok {
		return model.ProjectRef{Path: p, Temp: adapter.IsTempPath(p)}
	}
	return model.ProjectRef{Encoded: encoded, Temp: looksTemp(encoded)}
}

func looksTemp(encoded string) bool {
	for _, prefix := range []string{"-tmp-", "-var-tmp-", "-private-tmp-", "-private-var-folders-"} {
		if strings.HasPrefix(encoded, prefix) {
			return true
		}
	}
	return false
}

// Сколько читать из начала файла сессии: cwd записан в первых строках,
// а сами сессии бывают по сотне мегабайт.
const sessionHeadBytes = 256 << 10

var cwdField = regexp.MustCompile(`"cwd"\s*:\s*("(?:[^"\\]|\\.)*")`)

// cwdFromSessions достаёт только поле cwd из первых строк файлов сессий.
// Остальное содержимое сессий не разбирается и никуда не попадает.
func cwdFromSessions(projDir string) string {
	for _, f := range sortedJSONL(projDir) {
		fh, err := os.Open(f)
		if err != nil {
			continue
		}
		head, _ := io.ReadAll(io.LimitReader(fh, sessionHeadBytes))
		fh.Close()
		if m := cwdField.FindSubmatch(head); m != nil {
			var cwd string
			if json.Unmarshal(m[1], &cwd) == nil && cwd != "" {
				return cwd
			}
		}
	}
	return ""
}

var windowsEncodedRoot = regexp.MustCompile(`^([A-Za-z])--(.*)$`)

// DecodeByDisk восстанавливает путь по закодированному имени, сверяясь с диском:
// на каждом уровне ищется запись, чьё закодированное имя совпадает с началом
// остатка. Кодирование неоднозначно («-» — это и «/», и «.», и «_»), поэтому
// перебор с откатом; глубина ограничена.
func DecodeByDisk(encoded, goos string) (string, bool) {
	var root, rest string
	if goos == "windows" {
		m := windowsEncodedRoot.FindStringSubmatch(encoded)
		if m == nil {
			return "", false
		}
		root, rest = m[1]+`:\`, m[2]
	} else {
		if !strings.HasPrefix(encoded, "-") {
			return "", false
		}
		root, rest = "/", encoded[1:]
	}
	if rest == "" {
		return root, true
	}
	return walkDecode(root, rest, 0)
}

const maxDecodeDepth = 48

func walkDecode(dir, rest string, depth int) (string, bool) {
	if depth > maxDecodeDepth {
		return "", false
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", false
	}
	for _, e := range entries {
		enc := EncodeProjectPath(e.Name())
		if enc == "" {
			continue
		}
		full := filepath.Join(dir, e.Name())
		if enc == rest {
			if isDir(full) {
				return full, true
			}
			continue
		}
		if strings.HasPrefix(rest, enc+"-") && isDir(full) {
			if p, ok := walkDecode(full, rest[len(enc)+1:], depth+1); ok {
				return p, true
			}
		}
	}
	return "", false
}

func isDir(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}

// SessionProjects — пути проектов из каталогов проектов профилей, в том числе
// тех, где у Claude нет ни одного факта памяти: по ним ищутся инструкции и
// навыки проекта (ADR-025). Из файлов сессий читается только поле cwd, как и
// для памяти. Временные проекты не отбрасываются, как и в памяти: каталоги,
// которых уже нет, отсеет адаптер инструкций.
func SessionProjects(ctx context.Context, profiles []string) []string {
	var out []string
	for _, profile := range profiles {
		projects := filepath.Join(filepath.FromSlash(profile), "projects")
		entries, err := os.ReadDir(projects)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if ctx.Err() != nil {
				return out
			}
			if !e.IsDir() {
				continue
			}
			ref := ResolveProject(filepath.Join(projects, e.Name()), e.Name())
			if ref.Path != "" {
				out = append(out, ref.Path)
			}
		}
	}
	return out
}
