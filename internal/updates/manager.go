package updates

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// State — где сейчас обновление.
type State string

const (
	StateIdle        State = "idle"        // ещё не проверяли
	StateUnsupported State = "unsupported" // сборка без формата пакета: обновляется вручную
	StateChecking    State = "checking"
	StateCurrent     State = "current" // новее выпуска нет
	StateAvailable   State = "available"
	StateDownloading State = "downloading"
	StateReady       State = "ready" // пакет скачан и сверен
	StateFailed      State = "failed"
)

// Failure — ошибка в статусе: код и параметры для словаря фронтенда.
type Failure struct {
	Code   string            `json:"code"`
	Params map[string]string `json:"params,omitempty"`
}

// Status — состояние обновления для интерфейса.
type Status struct {
	State    State    `json:"state"`
	Current  string   `json:"current"`
	Format   Format   `json:"format"`
	Latest   string   `json:"latest,omitempty"`
	NotesURL string   `json:"notesUrl,omitempty"`
	Package  string   `json:"package,omitempty"` // имя файла пакета
	Size     int64    `json:"size,omitempty"`
	Received int64    `json:"received,omitempty"`
	Install  Install  `json:"install,omitempty"`
	Command  string   `json:"command,omitempty"` // готовая команда установки для InstallCommand
	Error    *Failure `json:"error,omitempty"`
}

// Opener — действия системы с готовым пакетом. Реализации — по ОС
// (open_*.go); в тестах подменяются.
type Opener interface {
	Reveal(ctx context.Context, path string) error // открыть папку с файлом
	Open(ctx context.Context, path string) error   // открыть файл программой по умолчанию
	Run(ctx context.Context, path string) error    // запустить, не дожидаясь завершения
}

// Options — из чего собирается Manager.
type Options struct {
	Current  string // версия этой сборки
	Format   Format
	Dir      string // каталог загрузок: <каталог данных>/updates
	Client   *Client
	Opener   Opener
	OnChange func(Status) // вызывается при каждой смене статуса, без блокировки
	Log      *slog.Logger
}

// Manager — проверка, скачивание и установка. Одновременно идёт не больше
// одной операции; статус читается в любой момент.
type Manager struct {
	opt Options

	mu       sync.Mutex
	st       Status
	release  Release
	contract Contract
	sum      string // ожидаемый SHA-256 готового пакета
	file     string // путь готового пакета
	cancel   context.CancelFunc
}

// New готовит Manager. Без формата пакета он в сеть не ходит.
func New(opt Options) *Manager {
	if opt.Log == nil {
		opt.Log = slog.Default()
	}
	st := Status{State: StateIdle, Current: opt.Current, Format: opt.Format}
	if opt.Format == FormatNone {
		st.State = StateUnsupported
	}
	return &Manager{opt: opt, st: st}
}

// Status — текущее состояние.
func (m *Manager) Status() Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.st
}

func (m *Manager) set(f func(*Status)) Status {
	m.mu.Lock()
	f(&m.st)
	st := m.st
	m.mu.Unlock()
	m.notify(st)
	return st
}

func (m *Manager) notify(st Status) {
	if m.opt.OnChange != nil {
		m.opt.OnChange(st)
	}
}

func (m *Manager) failed(err error) Status {
	var ue *Error
	f := &Failure{Code: "update.unknown"}
	switch {
	case errors.As(err, &ue):
		f = &Failure{Code: ue.Code, Params: ue.Params}
	case errors.Is(err, context.Canceled):
		f = &Failure{Code: "update.canceled"}
	}
	m.opt.Log.Warn("обновление", "error", err)
	return m.set(func(s *Status) {
		s.State, s.Error, s.Received = StateFailed, f, 0
	})
}

func (m *Manager) busy() bool {
	return m.st.State == StateChecking || m.st.State == StateDownloading
}

// Check спрашивает GitHub о последнем выпуске.
func (m *Manager) Check(ctx context.Context) Status {
	// Занятость проверяется и снимается под одним замком: два нажатия подряд
	// не должны запустить две проверки.
	m.mu.Lock()
	if m.opt.Format == FormatNone || m.busy() {
		st := m.st
		m.mu.Unlock()
		return st
	}
	// Прежний выпуск забывается целиком: если проверка не удастся, скачать
	// нечего, и статус не должен предлагать пакет прошлой проверки.
	m.st = Status{State: StateChecking, Current: m.st.Current, Format: m.st.Format}
	m.release, m.contract, m.sum, m.file = Release{}, Contract{}, "", ""
	st := m.st
	m.mu.Unlock()
	m.notify(st)

	r, err := m.opt.Client.Latest(ctx)
	if err != nil {
		return m.failed(err)
	}
	if !Newer(r.Version, m.opt.Current) {
		return m.set(func(s *Status) {
			*s = Status{State: StateCurrent, Current: s.Current, Format: s.Format, Latest: r.Version, NotesURL: r.NotesURL}
		})
	}
	c, _ := ContractFor(m.opt.Format, r.Version)
	a, ok := r.asset(c.Asset)
	if !ok {
		return m.failed(fail(nil, "update.no-package", "version", r.Version, "package", c.Asset))
	}
	m.mu.Lock()
	m.release, m.contract = r, c
	m.mu.Unlock()
	return m.set(func(s *Status) {
		*s = Status{State: StateAvailable, Current: s.Current, Format: s.Format, Latest: r.Version, NotesURL: r.NotesURL,
			Package: a.Name, Size: a.Size, Install: c.Install}
	})
}

// Download скачивает и сверяет пакет найденного выпуска. Блокирует до конца;
// прервать — Cancel.
func (m *Manager) Download(ctx context.Context) Status {
	m.mu.Lock()
	if m.st.State != StateAvailable && !(m.st.State == StateFailed && m.release.Version != "") {
		st := m.st
		m.mu.Unlock()
		return st
	}
	ctx, cancel := context.WithCancel(ctx)
	m.cancel = cancel
	r, c := m.release, m.contract
	a, _ := r.asset(c.Asset)
	m.st.State, m.st.Error, m.st.Received, m.st.Package, m.st.Size, m.st.Install = StateDownloading, nil, 0, a.Name, a.Size, c.Install
	st := m.st
	m.mu.Unlock()
	m.notify(st)
	defer func() {
		cancel()
		m.mu.Lock()
		m.cancel = nil
		m.mu.Unlock()
	}()

	file, sum, err := m.download(ctx, r, c, a)
	if err != nil {
		return m.failed(err)
	}
	m.mu.Lock()
	m.file, m.sum = file, sum
	m.mu.Unlock()
	return m.set(func(s *Status) {
		s.State, s.Received = StateReady, a.Size
		if c.Install == InstallCommand {
			s.Command = c.Command + " " + shellQuote(file)
		}
	})
}

func (m *Manager) download(ctx context.Context, r Release, c Contract, a Asset) (string, string, error) {
	// Сумма — из SHA256SUMS того же выпуска, а не только из API: файл сумм
	// выпуск сверяет перед публикацией (ADR-030).
	sa, ok := r.asset(c.Sums)
	if !ok {
		return "", "", fail(nil, "update.no-sums", "file", c.Sums)
	}
	var sums strings.Builder
	if _, err := m.opt.Client.fetch(ctx, sa.URL, &sums, maxSumsBody, nil); err != nil {
		return "", "", err
	}
	want, ok := ParseSums(sums.String(), a.Name)
	if !ok {
		return "", "", fail(nil, "update.no-sums", "file", c.Sums)
	}
	if a.Digest != "" && a.Digest != want {
		return "", "", fail(nil, "update.mismatch", "package", a.Name)
	}

	dir := filepath.Join(m.opt.Dir, r.Version)
	if err := m.prune(r.Version); err != nil {
		return "", "", err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", "", fail(err, "update.write")
	}
	tmp, err := os.CreateTemp(dir, a.Name+".*.part")
	if err != nil {
		return "", "", fail(err, "update.write")
	}
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name()) // после успешного переименования файла уже нет
	}()
	h := sha256.New()
	limit := int64(maxPackage)
	if a.Size > 0 {
		limit = a.Size
	}
	n, err := m.opt.Client.fetch(ctx, a.URL, io.MultiWriter(tmp, h), limit, func(n int64) {
		m.set(func(s *Status) { s.Received = n })
	})
	if err != nil {
		return "", "", err
	}
	if a.Size > 0 && n != a.Size {
		return "", "", fail(nil, "update.size", "reason", "short")
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != want {
		return "", "", fail(nil, "update.mismatch", "package", a.Name)
	}
	if err := tmp.Close(); err != nil {
		return "", "", fail(err, "update.write")
	}
	final := filepath.Join(dir, a.Name)
	if err := os.Rename(tmp.Name(), final); err != nil {
		return "", "", fail(err, "update.write")
	}
	return final, want, nil
}

// prune удаляет загрузки других версий. Каталог загрузок принадлежит
// ai-memory целиком, памяти агентов в нём нет.
func (m *Manager) prune(keep string) error {
	entries, err := os.ReadDir(m.opt.Dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fail(err, "update.write")
	}
	for _, e := range entries {
		if e.Name() == keep {
			continue
		}
		if err := os.RemoveAll(filepath.Join(m.opt.Dir, e.Name())); err != nil {
			return fail(err, "update.write")
		}
	}
	return nil
}

// Cancel прерывает скачивание.
func (m *Manager) Cancel() {
	m.mu.Lock()
	cancel := m.cancel
	m.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// Install ставит готовый пакет средствами системы. Перед этим файл сверяется
// ещё раз: между скачиванием и установкой его могли заменить. Возвращает
// способ установки: при InstallRun окно нужно закрыть, чтобы установщик
// заменил запущенный бинарь.
func (m *Manager) Install(ctx context.Context) (Install, error) {
	m.mu.Lock()
	if m.st.State != StateReady {
		m.mu.Unlock()
		return "", fail(nil, "update.not-ready")
	}
	file, sum, c := m.file, m.sum, m.contract
	m.mu.Unlock()

	if err := verify(file, sum); err != nil {
		m.mu.Lock()
		m.file, m.sum = "", ""
		m.mu.Unlock()
		m.set(func(s *Status) {
			s.State, s.Command, s.Received = StateAvailable, "", 0
			s.Error = &Failure{Code: "update.mismatch", Params: map[string]string{"package": filepath.Base(file)}}
		})
		return "", err
	}
	var err error
	switch c.Install {
	case InstallCommand:
		err = m.opt.Opener.Reveal(ctx, file)
	case InstallOpen:
		err = m.opt.Opener.Open(ctx, file)
	case InstallRun:
		err = m.opt.Opener.Run(ctx, file)
	default:
		err = fmt.Errorf("неизвестный способ установки %q", c.Install)
	}
	if err != nil {
		return "", fail(err, "update.launch")
	}
	return c.Install, nil
}

func verify(file, want string) error {
	f, err := os.Open(file)
	if err != nil {
		return fail(err, "update.mismatch", "package", filepath.Base(file))
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return fail(err, "update.mismatch", "package", filepath.Base(file))
	}
	if hex.EncodeToString(h.Sum(nil)) != want {
		return fail(nil, "update.mismatch", "package", filepath.Base(file))
	}
	return nil
}

// shellQuote — путь для команды в POSIX-оболочке: команды установки бывают
// только у пакетов Linux.
func shellQuote(p string) string {
	if p != "" && strings.Trim(p, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789/._-+") == "" {
		return p
	}
	return "'" + strings.ReplaceAll(p, "'", `'\''`) + "'"
}
