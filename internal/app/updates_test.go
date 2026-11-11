package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/yarkovaleksei/ai-memory/internal/updates"
)

// Ошибки обновления уходят во фронтенд кодом с параметрами (ADR-008).
func TestUpdateError(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	for _, tt := range []struct {
		err    error
		code   string
		params map[string]string
	}{
		{&updates.Error{Code: "update.mismatch", Params: map[string]string{"package": "x.deb"}}, "update.mismatch", map[string]string{"package": "x.deb"}},
		{fmt.Errorf("обёртка: %w", &updates.Error{Code: "update.network"}), "update.network", nil},
		{errors.New("что-то"), "update.unknown", map[string]string{"detail": "что-то"}},
	} {
		var e *Error
		if !errors.As(updateError(tt.err, log), &e) || e.Code != tt.code || fmt.Sprint(e.Params) != fmt.Sprint(tt.params) {
			t.Errorf("%v → %+v, want %s %v", tt.err, e, tt.code, tt.params)
		}
	}
}

// Сборка без формата пакета: статус «вручную», установка отказывает кодом.
func TestUpdatesServiceUnsupported(t *testing.T) {
	s := NewUpdatesService("0.0.1-dev", updates.FormatNone, filepath.Join(t.TempDir(), "updates"), nil)
	t.Cleanup(func() { _ = s.ServiceShutdown() })
	if st := s.Check(context.Background()); st.State != updates.StateUnsupported || st.Current != "0.0.1-dev" {
		t.Errorf("Check: %+v", st)
	}
	var e *Error
	if err := s.Install(context.Background()); !errors.As(err, &e) || e.Code != "update.not-ready" {
		t.Errorf("Install: %v", err)
	}
}
