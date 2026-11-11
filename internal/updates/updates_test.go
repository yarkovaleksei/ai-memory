package updates

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestNewer(t *testing.T) {
	for _, tt := range []struct {
		latest, current string
		want            bool
	}{
		{"0.7.0", "0.6.0", true},
		{"0.6.1", "0.6.0", true},
		{"1.0.0", "0.9.9", true},
		{"0.10.0", "0.9.0", true}, // числа, а не строки
		{"0.6.0", "0.6.0", false},
		{"0.5.9", "0.6.0", false},
		{"0.6.0", "0.6.0-dev", true},        // пререлиз младше выпуска
		{"0.6.0", "0.6.0-3-gabc123", false}, // git describe после тега — не старше
		{"0.6.0", "0.6.0-dirty", false},
		{"0.6.1", "0.6.0-3-gabc123-dirty", true},
		{"0.6.0", "непонятно", true},
		{"0.7.0-rc1", "0.6.0", false}, // пререлиз не предлагается
		{"мусор", "0.6.0", false},
	} {
		if got := Newer(tt.latest, tt.current); got != tt.want {
			t.Errorf("Newer(%q, %q) = %v, want %v", tt.latest, tt.current, got, tt.want)
		}
	}
}

func TestParseSums(t *testing.T) {
	sum := strings.Repeat("ab", 32)
	text := "not-a-sum  other\n" + strings.ToUpper(sum) + "  ai-memory_1.0.0_amd64.deb\n" + sum + " *ai-memory-1.0.0.x86_64.rpm\n"
	for _, tt := range []struct {
		name string
		ok   bool
	}{
		{"ai-memory_1.0.0_amd64.deb", true},
		{"ai-memory-1.0.0.x86_64.rpm", true}, // двоичный режим shasum
		{"other", false},
		{"ai-memory", false},
	} {
		got, ok := ParseSums(text, tt.name)
		if ok != tt.ok || (ok && got != sum) {
			t.Errorf("ParseSums(%q) = %q %v", tt.name, got, ok)
		}
	}
}

// Имена вложений — контракт с выпуском: те же строки, что сверяет
// scripts/release-verify.sh и выкладывает CI.
func TestContractMatchesRelease(t *testing.T) {
	verify := readRepo(t, "scripts", "release-verify.sh")
	workflow := readRepo(t, ".github", "workflows", "release.yml")
	taskfile := readRepo(t, "Taskfile.yml")
	for _, f := range []Format{FormatDeb, FormatRPM, FormatArch, FormatDMG, FormatNSIS} {
		c, ok := ContractFor(f, "${ver}")
		if !ok {
			t.Fatalf("нет контракта для %s", f)
		}
		if !strings.Contains(verify, `"`+c.Asset+`"`) {
			t.Errorf("%s: %s не сверяется в release-verify.sh", f, c.Asset)
		}
		if !strings.Contains(verify, c.Sums) {
			t.Errorf("%s: %s нет в release-verify.sh", f, c.Sums)
		}
		where := taskfile
		if f == FormatDMG || f == FormatNSIS {
			where = workflow
			c, _ = ContractFor(f, "${VERSION}")
		} else {
			c, _ = ContractFor(f, "{{.APP_VERSION}}")
		}
		if !strings.Contains(where, c.Asset) || !strings.Contains(where, c.Sums) {
			t.Errorf("%s: %s или %s не выкладывается выпуском", f, c.Asset, c.Sums)
		}
	}
	if _, ok := ContractFor(FormatNone, "1.0.0"); ok {
		t.Error("у сборки без формата не должно быть пакета")
	}
}

func readRepo(t *testing.T, parts ...string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(append([]string{"..", ".."}, parts...)...))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestDetectFormat(t *testing.T) {
	marker := func(s string) func(string) ([]byte, error) {
		return func(p string) ([]byte, error) {
			if p != LinuxMarker {
				return nil, os.ErrNotExist
			}
			if s == "" {
				return nil, os.ErrNotExist
			}
			return []byte(s), nil
		}
	}
	for _, tt := range []struct {
		name, built, goos, exe, marker string
		want                           Format
	}{
		{"dmg вшит", "dmg", "darwin", "/Applications/ai-memory.app/Contents/MacOS/ai-memory", "", FormatDMG},
		{"nsis вшит", "nsis", "windows", `C:\x\ai-memory.exe`, "", FormatNSIS},
		{"deb по метке", "", "linux", "/usr/bin/ai-memory", "deb\n", FormatDeb},
		{"rpm по метке", "", "linux", "/usr/bin/ai-memory", "rpm", FormatRPM},
		{"arch по метке", "", "linux", "/usr/bin/ai-memory", "arch\n", FormatArch},
		{"из исходников рядом с пакетом", "", "linux", "/home/me/projects/ai-memory/bin/ai-memory", "deb", FormatNone},
		{"метки нет", "", "linux", "/usr/bin/ai-memory", "", FormatNone},
		{"чужая метка", "", "linux", "/usr/bin/ai-memory", "dmg", FormatNone},
		{"без формата", "", "darwin", "/x/ai-memory", "deb", FormatNone},
		{"неизвестный вшитый", "appimage", "linux", "/x/ai-memory", "", FormatNone},
	} {
		if got := DetectFormat(tt.built, tt.goos, tt.exe, marker(tt.marker)); got != tt.want {
			t.Errorf("%s: %q, want %q", tt.name, got, tt.want)
		}
	}
}

// --- фальшивый GitHub ---

type fakeGitHub struct {
	srv      *httptest.Server
	requests atomic.Int64

	mu       sync.Mutex
	tag      string
	draft    bool
	status   int // ответ API; 0 — 200
	files    map[string][]byte
	sums     map[string]string // имя файла сумм → текст; nil — посчитать по files
	digest   map[string]string // подмена digest; "" — без digest
	size     map[string]int64  // подмена размера во вложении
	redirect string            // куда перенаправлять скачивание
	hold     chan struct{}     // скачивание пакета ждёт, пока канал не закроют
	apiBody  []byte            // подмена ответа API целиком
}

func newFakeGitHub(t *testing.T, tag string, files map[string][]byte) *fakeGitHub {
	t.Helper()
	g := &fakeGitHub{tag: tag, files: files}
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/"+Repo+"/releases/latest", g.latest)
	mux.HandleFunc("/download/", g.download)
	g.srv = httptest.NewTLSServer(mux)
	t.Cleanup(g.srv.Close)
	return g
}

func (g *fakeGitHub) client() *Client {
	u, _ := url.Parse(g.srv.URL)
	return &Client{LatestURL: g.srv.URL + "/repos/" + Repo + "/releases/latest", Hosts: []string{u.Hostname()},
		HTTP: g.srv.Client(), UserAgent: "ai-memory/test"}
}

func (g *fakeGitHub) latest(w http.ResponseWriter, r *http.Request) {
	g.requests.Add(1)
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.status != 0 {
		w.WriteHeader(g.status)
		return
	}
	if g.apiBody != nil {
		_, _ = w.Write(g.apiBody)
		return
	}
	type asset struct {
		Name   string `json:"name"`
		URL    string `json:"browser_download_url"`
		Size   int64  `json:"size"`
		Digest string `json:"digest,omitempty"`
		State  string `json:"state"`
	}
	var assets []asset
	for name, data := range g.allFiles() {
		a := asset{Name: name, URL: g.srv.URL + "/download/" + name, Size: int64(len(data)), State: "uploaded", Digest: "sha256:" + hexSum(data)}
		if d, ok := g.digest[name]; ok {
			a.Digest = d
		}
		if s, ok := g.size[name]; ok {
			a.Size = s
		}
		assets = append(assets, a)
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"tag_name": g.tag, "html_url": "https://github.com/" + Repo + "/releases/tag/" + g.tag,
		"draft": g.draft, "prerelease": false, "assets": assets,
	})
}

// allFiles — пакеты и файлы сумм, как их выкладывает выпуск.
func (g *fakeGitHub) allFiles() map[string][]byte {
	out := map[string][]byte{}
	sums := map[string]*strings.Builder{}
	for name, data := range g.files {
		out[name] = data
		for _, f := range []Format{FormatDeb, FormatRPM, FormatArch, FormatDMG, FormatNSIS} {
			if c, _ := ContractFor(f, strings.TrimPrefix(g.tag, "v")); c.Asset == name {
				if sums[c.Sums] == nil {
					sums[c.Sums] = &strings.Builder{}
				}
				fmt.Fprintf(sums[c.Sums], "%s  %s\n", hexSum(data), name)
			}
		}
	}
	for name, b := range sums {
		out[name] = []byte(b.String())
	}
	if g.sums != nil {
		for name, text := range g.sums {
			if text == "" {
				delete(out, name)
			} else {
				out[name] = []byte(text)
			}
		}
	}
	return out
}

func (g *fakeGitHub) download(w http.ResponseWriter, r *http.Request) {
	g.requests.Add(1)
	name := strings.TrimPrefix(r.URL.Path, "/download/")
	g.mu.Lock()
	data, ok := g.allFiles()[name]
	redirect, hold := g.redirect, g.hold
	g.mu.Unlock()
	if redirect != "" && !strings.HasPrefix(name, "SHA256SUMS") {
		http.Redirect(w, r, redirect, http.StatusFound)
		return
	}
	if !ok {
		http.NotFound(w, r)
		return
	}
	if hold != nil && !strings.HasPrefix(name, "SHA256SUMS") {
		_, _ = w.Write(data[:1])
		w.(http.Flusher).Flush()
		select {
		case <-hold:
		case <-r.Context().Done():
			return
		}
		data = data[1:]
	}
	_, _ = w.Write(data)
}

func hexSum(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

type fakeOpener struct {
	mu    sync.Mutex
	calls []string
}

func (o *fakeOpener) record(kind, path string) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.calls = append(o.calls, kind+" "+filepath.Base(path))
	return nil
}
func (o *fakeOpener) Reveal(_ context.Context, p string) error { return o.record("reveal", p) }
func (o *fakeOpener) Open(_ context.Context, p string) error   { return o.record("open", p) }
func (o *fakeOpener) Run(_ context.Context, p string) error    { return o.record("run", p) }

func newManager(t *testing.T, g *fakeGitHub, format Format, current string) (*Manager, *fakeOpener, string) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "updates")
	op := &fakeOpener{}
	m := New(Options{Current: current, Format: format, Dir: dir, Client: g.client(), Opener: op})
	return m, op, dir
}

func errCode(st Status) string {
	if st.Error == nil {
		return ""
	}
	return st.Error.Code
}

// Полный путь для каждого формата: найти, скачать, сверить, поставить.
func TestUpdateFlow(t *testing.T) {
	for _, tt := range []struct {
		format  Format
		install Install
		call    string
		command string
	}{
		{FormatDeb, InstallCommand, "reveal ai-memory_0.7.0_amd64.deb", "sudo apt install "},
		{FormatRPM, InstallCommand, "reveal ai-memory-0.7.0.x86_64.rpm", "sudo dnf install "},
		{FormatArch, InstallCommand, "reveal ai-memory-0.7.0-x86_64.pkg.tar.zst", "sudo pacman -U "},
		{FormatDMG, InstallOpen, "open ai-memory-0.7.0-macos-universal.dmg", ""},
		{FormatNSIS, InstallRun, "run ai-memory-0.7.0-windows-amd64-setup.exe", ""},
	} {
		t.Run(string(tt.format), func(t *testing.T) {
			ctx := context.Background()
			c, _ := ContractFor(tt.format, "0.7.0")
			body := []byte("пакет " + string(tt.format))
			g := newFakeGitHub(t, "v0.7.0", map[string][]byte{c.Asset: body, "ai-memory_0.7.0_arm64.deb": []byte("чужой")})
			m, op, dir := newManager(t, g, tt.format, "0.6.0")

			st := m.Check(ctx)
			if st.State != StateAvailable || st.Latest != "0.7.0" || st.Package != c.Asset || st.Install != tt.install || st.NotesURL == "" {
				t.Fatalf("Check: %+v", st)
			}
			st = m.Download(ctx)
			if st.State != StateReady || st.Received != int64(len(body)) {
				t.Fatalf("Download: %+v", st)
			}
			file := filepath.Join(dir, "0.7.0", c.Asset)
			if got, err := os.ReadFile(file); err != nil || string(got) != string(body) {
				t.Fatalf("файл пакета: %q %v", got, err)
			}
			if tt.command != "" && st.Command != tt.command+shellQuote(file) {
				t.Errorf("команда: %q", st.Command)
			}
			if tt.command == "" && st.Command != "" {
				t.Errorf("команда не нужна: %q", st.Command)
			}
			how, err := m.Install(ctx)
			if err != nil || how != tt.install {
				t.Fatalf("Install: %v %v", how, err)
			}
			if len(op.calls) != 1 || op.calls[0] != tt.call {
				t.Errorf("действие системы: %v", op.calls)
			}
			if parts, _ := filepath.Glob(filepath.Join(dir, "*", "*.part")); len(parts) != 0 {
				t.Errorf("остались недокачанные файлы: %v", parts)
			}
		})
	}
}

func TestCheckOutcomes(t *testing.T) {
	deb := "ai-memory_0.7.0_amd64.deb"
	for _, tt := range []struct {
		name    string
		current string
		setup   func(g *fakeGitHub)
		state   State
		code    string
	}{
		{"версия актуальна", "0.7.0", nil, StateCurrent, ""},
		{"своя версия новее", "0.8.0", nil, StateCurrent, ""},
		{"выпуска нет", "0.6.0", func(g *fakeGitHub) { g.status = http.StatusNotFound }, StateFailed, "update.no-release"},
		{"лимит API", "0.6.0", func(g *fakeGitHub) { g.status = http.StatusForbidden }, StateFailed, "update.rate-limited"},
		{"ошибка сервера", "0.6.0", func(g *fakeGitHub) { g.status = http.StatusBadGateway }, StateFailed, "update.http"},
		{"черновик", "0.6.0", func(g *fakeGitHub) { g.draft = true }, StateFailed, "update.no-release"},
		{"тег не vX.Y.Z", "0.6.0", func(g *fakeGitHub) { g.tag = "v0.7.0-rc1" }, StateFailed, "update.no-release"},
		{"нет пакета формата", "0.6.0", func(g *fakeGitHub) { delete(g.files, deb) }, StateFailed, "update.no-package"},
		{"ответ не JSON", "0.6.0", func(g *fakeGitHub) { g.apiBody = []byte("<html>") }, StateFailed, "update.bad-release"},
		{"ответ слишком большой", "0.6.0", func(g *fakeGitHub) { g.apiBody = make([]byte, maxAPIBody+10) }, StateFailed, "update.bad-release"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			g := newFakeGitHub(t, "v0.7.0", map[string][]byte{deb: []byte("пакет")})
			if tt.setup != nil {
				tt.setup(g)
			}
			m, _, _ := newManager(t, g, FormatDeb, tt.current)
			st := m.Check(context.Background())
			if st.State != tt.state || errCode(st) != tt.code {
				t.Errorf("статус %s, код %q; want %s, %q", st.State, errCode(st), tt.state, tt.code)
			}
		})
	}
}

// Пакет, который не совпал со сверкой, не остаётся на диске и не ставится.
func TestDownloadRejects(t *testing.T) {
	deb := "ai-memory_0.7.0_amd64.deb"
	other := strings.Repeat("0", 64)
	for _, tt := range []struct {
		name  string
		setup func(g *fakeGitHub)
		code  string
	}{
		{"сумма в SHA256SUMS не та", func(g *fakeGitHub) {
			g.sums = map[string]string{"SHA256SUMS-linux.txt": other + "  " + deb + "\n"}
			g.digest = map[string]string{deb: ""}
		}, "update.mismatch"},
		{"digest API расходится с SHA256SUMS", func(g *fakeGitHub) { g.digest = map[string]string{deb: "sha256:" + other} }, "update.mismatch"},
		{"нет файла сумм", func(g *fakeGitHub) { g.sums = map[string]string{"SHA256SUMS-linux.txt": ""} }, "update.no-sums"},
		{"пакета нет в файле сумм", func(g *fakeGitHub) {
			g.sums = map[string]string{"SHA256SUMS-linux.txt": other + "  другой.deb\n"}
		}, "update.no-sums"},
		{"файл короче заявленного", func(g *fakeGitHub) { g.size = map[string]int64{deb: 100} }, "update.size"},
		{"файл длиннее заявленного", func(g *fakeGitHub) { g.size = map[string]int64{deb: 2} }, "update.size"},
		{"перенаправление на чужой хост", func(g *fakeGitHub) { g.redirect = "https://example.com/x.deb" }, "update.host"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			g := newFakeGitHub(t, "v0.7.0", map[string][]byte{deb: []byte("настоящий пакет")})
			tt.setup(g)
			m, op, dir := newManager(t, g, FormatDeb, "0.6.0")
			if st := m.Check(ctx); st.State != StateAvailable {
				t.Fatalf("Check: %+v", st)
			}
			st := m.Download(ctx)
			if st.State != StateFailed || errCode(st) != tt.code {
				t.Fatalf("статус %s, код %q; want %q", st.State, errCode(st), tt.code)
			}
			if files, _ := filepath.Glob(filepath.Join(dir, "*", "*")); len(files) != 0 {
				t.Errorf("на диске остались файлы: %v", files)
			}
			if _, err := m.Install(ctx); err == nil || len(op.calls) != 0 {
				t.Errorf("непроверенный пакет не должен ставиться: %v %v", err, op.calls)
			}
		})
	}
}

// Пакет подменили между скачиванием и установкой — установки нет.
func TestInstallRechecks(t *testing.T) {
	ctx := context.Background()
	deb := "ai-memory_0.7.0_amd64.deb"
	g := newFakeGitHub(t, "v0.7.0", map[string][]byte{deb: []byte("пакет")})
	m, op, dir := newManager(t, g, FormatDeb, "0.6.0")
	m.Check(ctx)
	if st := m.Download(ctx); st.State != StateReady {
		t.Fatalf("Download: %+v", st)
	}
	if err := os.WriteFile(filepath.Join(dir, "0.7.0", deb), []byte("подмена"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := m.Install(ctx)
	var ue *Error
	if !errors.As(err, &ue) || ue.Code != "update.mismatch" || len(op.calls) != 0 {
		t.Fatalf("Install: %v %v", err, op.calls)
	}
	if st := m.Status(); st.State != StateAvailable || st.Command != "" {
		t.Errorf("после отказа пакет нужно скачать заново: %+v", st)
	}
}

// Сборка без формата пакета не ходит в сеть.
func TestUnsupportedBuildStaysOffline(t *testing.T) {
	g := newFakeGitHub(t, "v0.7.0", map[string][]byte{"ai-memory_0.7.0_amd64.deb": []byte("пакет")})
	m, _, _ := newManager(t, g, FormatNone, "0.0.1-dev")
	if st := m.Check(context.Background()); st.State != StateUnsupported {
		t.Errorf("статус %s", st.State)
	}
	if st := m.Download(context.Background()); st.State != StateUnsupported {
		t.Errorf("статус %s", st.State)
	}
	if n := g.requests.Load(); n != 0 {
		t.Errorf("запросов к GitHub: %d", n)
	}
}

// Новая загрузка убирает пакеты прежних версий: каталог загрузок — только
// ai-memory.
func TestDownloadPrunesOldVersions(t *testing.T) {
	ctx := context.Background()
	deb := "ai-memory_0.7.0_amd64.deb"
	g := newFakeGitHub(t, "v0.7.0", map[string][]byte{deb: []byte("пакет")})
	m, _, dir := newManager(t, g, FormatDeb, "0.6.0")
	old := filepath.Join(dir, "0.6.5", "ai-memory_0.6.5_amd64.deb")
	if err := os.MkdirAll(filepath.Dir(old), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(old, []byte("старый"), 0o600); err != nil {
		t.Fatal(err)
	}
	m.Check(ctx)
	if st := m.Download(ctx); st.State != StateReady {
		t.Fatalf("Download: %+v", st)
	}
	if _, err := os.Stat(filepath.Dir(old)); !os.IsNotExist(err) {
		t.Errorf("прежняя версия осталась: %v", err)
	}
}

// Отмена прерывает скачивание, а повторная попытка проходит.
func TestCancelAndRetry(t *testing.T) {
	ctx := context.Background()
	deb := "ai-memory_0.7.0_amd64.deb"
	g := newFakeGitHub(t, "v0.7.0", map[string][]byte{deb: []byte("пакет целиком")})
	hold := make(chan struct{})
	g.hold = hold
	var seen sync.Map
	dir := filepath.Join(t.TempDir(), "updates")
	m := New(Options{Current: "0.6.0", Format: FormatDeb, Dir: dir, Client: g.client(), Opener: &fakeOpener{},
		OnChange: func(st Status) { seen.Store(st.State, true) }})
	m.Check(ctx)

	done := make(chan Status)
	go func() { done <- m.Download(ctx) }()
	deadline := time.Now().Add(5 * time.Second)
	for m.Status().State != StateDownloading {
		if time.Now().After(deadline) {
			t.Fatal("скачивание не началось")
		}
		time.Sleep(10 * time.Millisecond)
	}
	// Вторая загрузка, пока идёт первая, ничего не запускает.
	if st := m.Download(ctx); st.State != StateDownloading {
		t.Errorf("вторая загрузка: %s", st.State)
	}
	m.Cancel()
	st := <-done
	if st.State != StateFailed || errCode(st) != "update.canceled" {
		t.Fatalf("после отмены: %+v", st)
	}
	if _, ok := seen.Load(StateDownloading); !ok {
		t.Error("OnChange не сообщил о скачивании")
	}

	close(hold)
	g.mu.Lock()
	g.hold = nil
	g.mu.Unlock()
	if st := m.Download(ctx); st.State != StateReady {
		t.Errorf("повторная попытка: %+v", st)
	}
}

// Неудачная повторная проверка не оставляет в статусе пакет прошлой: иначе
// интерфейс предложил бы «Скачать снова», а скачивать нечего.
func TestFailedRecheckForgetsPackage(t *testing.T) {
	ctx := context.Background()
	g := newFakeGitHub(t, "v0.7.0", map[string][]byte{"ai-memory_0.7.0_amd64.deb": []byte("пакет")})
	m, _, _ := newManager(t, g, FormatDeb, "0.6.0")
	if st := m.Check(ctx); st.State != StateAvailable {
		t.Fatalf("Check: %+v", st)
	}
	g.mu.Lock()
	g.status = http.StatusBadGateway
	g.mu.Unlock()
	st := m.Check(ctx)
	if st.State != StateFailed || st.Package != "" || st.Latest != "" {
		t.Errorf("после неудачной проверки: %+v", st)
	}
	if st := m.Download(ctx); st.State != StateFailed || g.requests.Load() != 2 {
		t.Errorf("скачивание без выпуска: %+v, запросов %d", st, g.requests.Load())
	}
}

func TestShellQuote(t *testing.T) {
	for in, want := range map[string]string{
		"/home/me/.local/share/ai-memory/updates/0.7.0/ai-memory_0.7.0_amd64.deb": "/home/me/.local/share/ai-memory/updates/0.7.0/ai-memory_0.7.0_amd64.deb",
		"/home/me/my data/x.deb": "'/home/me/my data/x.deb'",
		"/home/o'brien/x.deb":    `'/home/o'\''brien/x.deb'`,
	} {
		if got := shellQuote(in); got != want {
			t.Errorf("shellQuote(%q) = %q, want %q", in, got, want)
		}
	}
}
