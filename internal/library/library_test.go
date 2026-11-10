package library

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/yarkovaleksei/ai-memory/internal/config"
	"github.com/yarkovaleksei/ai-memory/internal/index"
	"github.com/yarkovaleksei/ai-memory/internal/model"
)

// copyFixtures копирует fixtures/home во временный каталог: тесты этого пакета
// меняют права и содержимое, а исходные фикстуры только читаются.
func copyFixtures(t *testing.T) string {
	t.Helper()
	src, err := filepath.Abs(filepath.Join("..", "..", "fixtures", "home"))
	if err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(t.TempDir(), "home")
	err = filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		in, err := os.Open(p)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.Create(target)
		if err != nil {
			return err
		}
		if _, err := io.Copy(out, in); err != nil {
			out.Close()
			return err
		}
		return out.Close()
	})
	if err != nil {
		t.Fatal(err)
	}
	return dst
}

func configFor(home, dataDir string) config.Config {
	return config.Config{
		DataDir: filepath.ToSlash(dataDir),
		ClaudeProfiles: []string{
			filepath.ToSlash(filepath.Join(home, ".claude")),
			filepath.ToSlash(filepath.Join(home, ".claude-work")),
		},
		CodexHome: filepath.ToSlash(filepath.Join(home, ".codex")),
	}
}

func openLibrary(t *testing.T, cfg config.Config) *Library {
	t.Helper()
	lib, err := New(cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { lib.Close() })
	return lib
}

// snapshot — путь → sha256 и права для каждого файла и каталога дерева.
func snapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		fi, err := d.Info()
		if err != nil {
			return err
		}
		state := fi.Mode().String()
		if !d.IsDir() {
			data, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			sum := sha256.Sum256(data)
			state += " " + hex.EncodeToString(sum[:]) + " " + fi.ModTime().String()
		}
		out[p] = state
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestRebuildOnFixtures(t *testing.T) {
	home := copyFixtures(t)
	lib := openLibrary(t, configFor(home, t.TempDir()))
	ctx := context.Background()

	st, err := lib.Rebuild(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if st.Sources != 3 { // два профиля Claude и Codex v2; v1 выключена по умолчанию
		t.Errorf("sources = %d, want 3", st.Sources)
	}
	if st.Items == 0 || st.Projects == 0 {
		t.Fatalf("пустая пересборка: %+v", st)
	}

	projects, err := lib.Projects(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var demo *model.Project
	for i := range projects {
		if projects[i].Path == "/home/me/projects/demo" {
			if demo != nil {
				t.Fatal("проект demo встречается дважды")
			}
			demo = &projects[i]
		}
	}
	if demo == nil {
		t.Fatalf("проект demo не найден: %+v", projects)
	}
	if !slices.Contains(demo.Agents, model.AgentClaude) || !slices.Contains(demo.Agents, model.AgentCodex) {
		t.Errorf("demo должен объединять оба агента: %v", demo.Agents)
	}

	page, err := lib.Search(ctx, index.Query{ProjectID: demo.ID})
	if err != nil {
		t.Fatal(err)
	}
	agents := map[model.Agent]bool{}
	for _, h := range page.Hits {
		agents[h.Agent] = true
	}
	if !agents[model.AgentClaude] || !agents[model.AgentCodex] {
		t.Errorf("в demo должны быть записи обоих агентов: %v", agents)
	}

	if len(page.Hits) > 0 {
		it, err := lib.Get(ctx, page.Hits[0].ID)
		if err != nil || it.Body == "" {
			t.Errorf("Get вернул запись без тела: %+v %v", it, err)
		}
	}
}

// TestRebuildDoesNotWrite доказывает критерий фазы 1: код фазы не пишет вне каталога
// данных. Каталоги памяти лишены права на запись, и после пересборки дерево
// побайтно совпадает со снимком до неё.
func TestRebuildDoesNotWrite(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("права a-w на каталогах Windows не запрещают создание файлов; проверка хэшей ниже всё равно идёт на Linux и macOS")
	}
	home := copyFixtures(t)
	setWritable := func(writable bool) {
		_ = filepath.WalkDir(home, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			mode := os.FileMode(0o444)
			if d.IsDir() {
				mode = 0o555
			}
			if writable {
				mode |= 0o200
			}
			_ = os.Chmod(p, mode)
			return nil
		})
	}
	setWritable(false)
	t.Cleanup(func() { setWritable(true) }) // иначе t.TempDir не удалится

	before := snapshot(t, home)
	lib := openLibrary(t, configFor(home, t.TempDir()))
	if _, err := lib.Rebuild(context.Background()); err != nil {
		t.Fatal(err)
	}
	after := snapshot(t, home)

	for p, s := range after {
		if before[p] != s {
			t.Errorf("изменилось: %s\n до: %s\n после: %s", p, before[p], s)
		}
	}
	for p := range before {
		if _, ok := after[p]; !ok {
			t.Errorf("пропало: %s", p)
		}
	}
}

func TestWatchPicksUpNewFact(t *testing.T) {
	home := copyFixtures(t)
	lib := openLibrary(t, configFor(home, t.TempDir()))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if _, err := lib.Rebuild(ctx); err != nil {
		t.Fatal(err)
	}
	updates := make(chan Stats, 4)
	done := make(chan error, 1)
	go func() { done <- lib.Watch(ctx, func(s Stats) { updates <- s }) }()
	time.Sleep(200 * time.Millisecond) // дать наблюдателю подписаться на каталоги

	// Это копия фикстур во временном каталоге, не настоящая память.
	fact := filepath.Join(home, ".claude", "projects", "-home-me-projects-demo", "memory", "watch-probe.md")
	body := "---\nname: watch-probe\ndescription: проверка наблюдения\nmetadata:\n  type: project\n---\n\nуникальноеслово\n"
	if err := os.WriteFile(fact, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	select {
	case <-updates:
	case <-time.After(5 * time.Second):
		t.Fatal("наблюдатель не пересобрал индекс после нового файла")
	}
	page, err := lib.Search(ctx, index.Query{Text: "уникальноеслово"})
	if err != nil || page.Total != 1 {
		t.Fatalf("новый факт не найден: %+v %v", page, err)
	}
	if !strings.Contains(page.Hits[0].File, "watch-probe.md") {
		t.Errorf("найден не тот файл: %s", page.Hits[0].File)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Error(err)
		}
	case <-time.After(2 * time.Second):
		t.Error("Watch не завершился после отмены контекста")
	}
}

// ai-memory работает с одним агентом или без обоих: отсутствующие каталоги
// Claude и Codex — пустая память, а не ошибка.
func TestRebuildWithoutAgents(t *testing.T) {
	home := t.TempDir()
	only := func(dir string) string { return filepath.Join(home, dir) }
	if err := os.CopyFS(only(".claude"), os.DirFS(filepath.Join(copyFixtures(t), ".claude"))); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name    string
		claude  []string
		codex   string
		sources int
	}{
		{"нет ни Claude, ни Codex", []string{only(".claude-нет")}, only(".codex-нет"), 0},
		{"только Claude", []string{only(".claude")}, only(".codex-нет"), 1},
		{"только Codex", []string{only(".claude-нет")}, filepath.Join(copyFixtures(t), ".codex"), 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lib := openLibrary(t, config.Config{
				DataDir:        filepath.ToSlash(t.TempDir()),
				ClaudeProfiles: toSlash(tt.claude),
				CodexHome:      filepath.ToSlash(tt.codex),
			})
			st, err := lib.Rebuild(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if st.Sources != tt.sources {
				t.Errorf("sources = %d, want %d", st.Sources, tt.sources)
			}
		})
	}
}

func toSlash(paths []string) []string {
	out := make([]string, len(paths))
	for i, p := range paths {
		out[i] = filepath.ToSlash(p)
	}
	return out
}
