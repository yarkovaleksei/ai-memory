// Package backup ведёт репозиторий бэкапов ai-memory: отдельный git-репозиторий
// в каталоге данных, куда перед каждой записью кладётся снимок затронутых
// файлов, а после — их новое содержимое. Откат операции строится по двум
// соседним состояниям этого репозитория.
//
// Используется системный git: он есть у всех, кто работает с Claude Code и
// Codex, а история бэкапов остаётся читаемой обычными инструментами.
package backup

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Dir — имя репозитория в каталоге данных.
const Dir = "backups"

// filesDir — корень зеркала файлов агентов внутри репозитория.
const filesDir = "files"

// metaPrefix открывает строку метаданных в теле коммита операции.
const metaPrefix = "ai-memory-meta: "

// ErrNoGit — системный git не найден.
var ErrNoGit = errors.New("backup.git-missing")

// Repo — репозиторий бэкапов.
type Repo struct {
	dir string
	git string
}

// File — состояние одного файла агента: содержимое или отсутствие.
type File struct {
	Path    string // абсолютный путь файла агента
	Exists  bool
	Content []byte
}

// Commit — запись истории.
type Commit struct {
	Hash    string
	Time    time.Time
	Subject string
	Meta    json.RawMessage // nil у снимков и служебных коммитов
}

// Open открывает репозиторий в каталоге данных, при необходимости создаёт его.
func Open(ctx context.Context, dataDir string) (*Repo, error) {
	git, err := exec.LookPath("git")
	if err != nil {
		return nil, ErrNoGit
	}
	r := &Repo{dir: filepath.Join(filepath.FromSlash(dataDir), Dir), git: git}
	if _, err := os.Stat(filepath.Join(r.dir, ".git")); err == nil {
		return r, nil
	}
	if err := os.MkdirAll(r.dir, 0o700); err != nil {
		return nil, fmt.Errorf("backup: каталог: %w", err)
	}
	if _, err := r.run(ctx, nil, "init", "-q"); err != nil {
		return nil, err
	}
	// Файлы памяти храним байт в байт: никаких преобразований концов строк.
	if err := os.WriteFile(filepath.Join(r.dir, ".gitattributes"), []byte("* -text\n"), 0o600); err != nil {
		return nil, fmt.Errorf("backup: .gitattributes: %w", err)
	}
	if _, err := r.run(ctx, nil, "add", ".gitattributes"); err != nil {
		return nil, err
	}
	if _, err := r.run(ctx, nil, "commit", "-q", "-m", "Создать репозиторий бэкапов ai-memory"); err != nil {
		return nil, err
	}
	return r, nil
}

// Dir возвращает путь репозитория.
func (r *Repo) Dir() string { return r.dir }

// MapPath — путь копии файла агента внутри репозитория. Обратное отображение не
// нужно: исходные пути хранятся в метаданных коммита операции.
func MapPath(abs string) string {
	p := filepath.ToSlash(filepath.Clean(abs))
	switch {
	case strings.HasPrefix(p, "//"): // UNC-путь Windows
		p = "unc/" + strings.TrimPrefix(p, "//")
	case len(p) >= 2 && p[1] == ':': // C:/Users/… → C/Users/…
		p = p[:1] + p[2:]
	}
	return filesDir + "/" + strings.TrimLeft(p, "/")
}

// Record записывает состояние файлов и коммитит его. meta == nil — снимок, иначе
// коммит операции с метаданными. Если состояние не изменилось, коммит снимка
// не создаётся, а коммит операции создаётся всегда (пустой), чтобы операция
// была в истории.
func (r *Repo) Record(ctx context.Context, files []File, subject string, meta any) (string, error) {
	for _, f := range files {
		rel := MapPath(f.Path)
		dst := filepath.Join(r.dir, filepath.FromSlash(rel))
		if f.Exists {
			// Путь в зеркале — либо файл, либо каталог: симлинк навыка хранится
			// файлом на месте прежнего каталога копии, и наоборот (ADR-023).
			// На диске агента так быть не может, значит, остатки устарели.
			if fi, err := os.Lstat(dst); err == nil && fi.IsDir() {
				if err := os.RemoveAll(dst); err != nil {
					return "", fmt.Errorf("backup: каталог на месте копии %s: %w", f.Path, err)
				}
			}
			if err := clearFileParents(r.dir, filepath.Dir(dst)); err != nil {
				return "", fmt.Errorf("backup: каталог копии: %w", err)
			}
			if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
				return "", fmt.Errorf("backup: каталог копии: %w", err)
			}
			if err := os.WriteFile(dst, f.Content, 0o600); err != nil {
				return "", fmt.Errorf("backup: копия %s: %w", f.Path, err)
			}
		} else if fi, err := os.Lstat(dst); err == nil && fi.IsDir() {
			// Каталог в зеркале — не копия файла: файла по этому пути нет и так.
			continue
		} else if err := os.Remove(dst); err != nil && !errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("backup: удаление копии %s: %w", f.Path, err)
		}
	}
	// Зеркало целиком: git add по отдельным путям падает на файлах, которых в
	// репозитории ещё не было, а кроме наших записей в files/ ничего не меняется.
	if _, err := os.Stat(filepath.Join(r.dir, filesDir)); err == nil {
		if _, err := r.run(ctx, nil, "add", "-A", "--", filesDir); err != nil {
			return "", err
		}
	}
	msg := subject
	args := []string{"commit", "-q", "-F", "-"}
	if meta != nil {
		data, err := json.Marshal(meta)
		if err != nil {
			return "", fmt.Errorf("backup: метаданные: %w", err)
		}
		msg += "\n\n" + metaPrefix + string(data) + "\n"
		args = append(args, "--allow-empty")
	} else {
		staged, err := r.run(ctx, nil, "diff", "--cached", "--name-only")
		if err != nil {
			return "", err
		}
		if strings.TrimSpace(staged) == "" {
			return r.head(ctx)
		}
	}
	if _, err := r.run(ctx, strings.NewReader(msg), args...); err != nil {
		return "", err
	}
	return r.head(ctx)
}

// clearFileParents убирает файлы зеркала, которые стоят на месте нужных
// каталогов (между root и dir).
func clearFileParents(root, dir string) error {
	for d := dir; len(d) > len(root); d = filepath.Dir(d) {
		if fi, err := os.Lstat(d); err == nil && !fi.IsDir() {
			if err := os.Remove(d); err != nil {
				return err
			}
		}
	}
	return nil
}

// Log возвращает коммиты операций, новые первыми.
func (r *Repo) Log(ctx context.Context, limit int) ([]Commit, error) {
	args := []string{"log", "--format=%H%x1f%aI%x1f%s%x1f%b%x1e"}
	if limit > 0 {
		args = append(args, fmt.Sprintf("-n%d", limit*2+1)) // снимки перемежаются с операциями
	}
	out, err := r.run(ctx, nil, args...)
	if err != nil {
		return nil, err
	}
	var commits []Commit
	for _, rec := range strings.Split(out, "\x1e") {
		parts := strings.SplitN(strings.TrimLeft(rec, "\n"), "\x1f", 4)
		if len(parts) < 4 {
			continue
		}
		c := Commit{Hash: parts[0], Subject: parts[2]}
		c.Time, _ = time.Parse(time.RFC3339, parts[1])
		for _, line := range strings.Split(parts[3], "\n") {
			if strings.HasPrefix(line, metaPrefix) {
				c.Meta = json.RawMessage(strings.TrimPrefix(line, metaPrefix))
			}
		}
		if c.Meta == nil {
			continue
		}
		commits = append(commits, c)
		if limit > 0 && len(commits) == limit {
			break
		}
	}
	return commits, nil
}

// Meta возвращает метаданные коммита операции.
func (r *Repo) Meta(ctx context.Context, hash string) (json.RawMessage, error) {
	if err := validRev(hash); err != nil {
		return nil, err
	}
	out, err := r.run(ctx, nil, "log", "-1", "--format=%b", hash, "--")
	if err != nil {
		return nil, err
	}
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, metaPrefix) {
			return json.RawMessage(strings.TrimPrefix(line, metaPrefix)), nil
		}
	}
	return nil, fmt.Errorf("backup: у коммита %s нет метаданных операции", hash)
}

// FileAt — состояние файла агента в коммите rev («hash» или «hash^»).
func (r *Repo) FileAt(ctx context.Context, rev, abs string) (File, error) {
	if err := validRev(strings.TrimSuffix(rev, "^")); err != nil {
		return File{}, err
	}
	spec := rev + ":" + MapPath(abs)
	if _, err := r.run(ctx, nil, "cat-file", "-e", spec); err != nil {
		return File{Path: abs}, nil // файла в этом состоянии не было
	}
	out, err := r.runBytes(ctx, nil, "cat-file", "blob", spec)
	if err != nil {
		return File{}, err
	}
	return File{Path: abs, Exists: true, Content: out}, nil
}

func (r *Repo) head(ctx context.Context) (string, error) {
	out, err := r.run(ctx, nil, "rev-parse", "HEAD")
	return strings.TrimSpace(out), err
}

// validRev пропускает только полный или сокращённый hex-хэш: строка приходит из
// интерфейса и не должна стать опцией git.
func validRev(rev string) error {
	if len(rev) < 7 || len(rev) > 64 {
		return fmt.Errorf("backup: неверный коммит %q", rev)
	}
	for _, c := range rev {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return fmt.Errorf("backup: неверный коммит %q", rev)
		}
	}
	return nil
}

func (r *Repo) run(ctx context.Context, stdin io.Reader, args ...string) (string, error) {
	out, err := r.runBytes(ctx, stdin, args...)
	return string(out), err
}

func (r *Repo) runBytes(ctx context.Context, stdin io.Reader, args ...string) ([]byte, error) {
	full := append([]string{
		"-C", r.dir,
		"-c", "commit.gpgsign=false",
		"-c", "core.autocrlf=false",
		"-c", "core.quotepath=false",
		"-c", "init.defaultBranch=main",
	}, args...)
	cmd := exec.CommandContext(ctx, r.git, full...)
	// Репозиторий бэкапов не зависит от настроек git пользователя: ни хуков,
	// ни подписи, ни чужого автора.
	cmd.Env = append(os.Environ(),
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_GLOBAL="+os.DevNull,
		"GIT_AUTHOR_NAME=ai-memory", "GIT_AUTHOR_EMAIL=ai-memory@localhost",
		"GIT_COMMITTER_NAME=ai-memory", "GIT_COMMITTER_EMAIL=ai-memory@localhost",
		"GIT_TERMINAL_PROMPT=0",
	)
	if stdin != nil {
		cmd.Stdin = stdin
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("backup: git %s: %w: %s", args[0], err, strings.TrimSpace(stderr.String()))
	}
	return stdout.Bytes(), nil
}
