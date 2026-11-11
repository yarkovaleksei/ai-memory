package app

import (
	"context"
	"errors"
	"log/slog"
	"os/exec"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/yarkovaleksei/ai-memory/internal/updates"
)

// EventUpdateStatus — смена состояния обновления; данные — updates.Status.
// Скачивание идёт в фоне, и прогресс приходит этим событием.
const EventUpdateStatus = "update:status"

func init() {
	application.RegisterEvent[updates.Status](EventUpdateStatus)
}

// UpdatesService — обновление из приложения (ADR-031): проверка по кнопке,
// скачивание со сверкой и установка средствами системы.
type UpdatesService struct {
	m   *updates.Manager
	log *slog.Logger
	// ctx живёт, пока живёт приложение: скачивание не должно обрываться,
	// когда возвращается вызов Download.
	ctx    context.Context
	cancel context.CancelFunc
	// quit закрывает приложение после запуска установщика; подменяется в тестах.
	quit func()
}

// NewUpdatesService: dir — каталог загрузок (<каталог данных>/updates).
func NewUpdatesService(current string, format updates.Format, dir string, log *slog.Logger) *UpdatesService {
	if log == nil {
		log = slog.Default()
	}
	ctx, cancel := context.WithCancel(context.Background())
	s := &UpdatesService{log: log, ctx: ctx, cancel: cancel, quit: quitApp}
	s.m = updates.New(updates.Options{
		Current:  current,
		Format:   format,
		Dir:      dir,
		Client:   updates.NewClient("ai-memory/" + current),
		Opener:   systemOpener{},
		OnChange: func(st updates.Status) { emitEvent(EventUpdateStatus, st) },
		Log:      log,
	})
	return s
}

// ServiceShutdown прерывает скачивание при выходе.
func (s *UpdatesService) ServiceShutdown() error {
	s.cancel()
	return nil
}

// Status — текущее состояние, без сети.
func (s *UpdatesService) Status() updates.Status { return s.m.Status() }

// Check спрашивает GitHub о последнем выпуске. Ошибка — в поле error статуса.
func (s *UpdatesService) Check(ctx context.Context) updates.Status {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	return s.m.Check(ctx)
}

// Download начинает скачивание в фоне. Статус не возвращается: он приходит
// только событием update:status, иначе ответ вызова мог бы прийти позже
// события «готово» и вернуть в интерфейс устаревшее состояние.
func (s *UpdatesService) Download() {
	go func() {
		ctx, cancel := context.WithTimeout(s.ctx, 30*time.Minute)
		defer cancel()
		s.m.Download(ctx)
	}()
}

// Cancel прерывает скачивание.
func (s *UpdatesService) Cancel() { s.m.Cancel() }

// Install ставит скачанный пакет. Для установщика Windows окно закрывается:
// установщик заменяет запущенный бинарь.
func (s *UpdatesService) Install(ctx context.Context) error {
	how, err := s.m.Install(ctx)
	if err != nil {
		return updateError(err, s.log)
	}
	if how == updates.InstallRun {
		// Пауза — чтобы ответ успел дойти до фронтенда.
		time.AfterFunc(300*time.Millisecond, s.quit)
	}
	return nil
}

func updateError(err error, log *slog.Logger) error {
	var ue *updates.Error
	if errors.As(err, &ue) {
		log.Warn("обновление", "error", err)
		return &Error{Code: ue.Code, Params: ue.Params}
	}
	log.Error("обновление", "error", err)
	return &Error{Code: "update.unknown", Params: map[string]string{"detail": err.Error()}}
}

func quitApp() {
	if a := application.Get(); a != nil {
		a.Quit()
	}
}

// systemOpener — действия системы через Wails и запуск установщика.
type systemOpener struct{}

func (systemOpener) Reveal(_ context.Context, path string) error {
	a := application.Get()
	if a == nil {
		return errors.New("приложение не запущено")
	}
	return a.Env.OpenFileManager(path, true)
}

func (systemOpener) Open(_ context.Context, path string) error {
	a := application.Get()
	if a == nil {
		return errors.New("приложение не запущено")
	}
	return a.Browser.OpenFile(path)
}

// Run запускает установщик отдельным процессом. Контекст вызова сюда не
// передаётся: установщик должен пережить и вызов, и само приложение.
func (systemOpener) Run(_ context.Context, path string) error {
	cmd := exec.Command(path) //nolint:gosec // путь — сверенный пакет из каталога загрузок
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}
