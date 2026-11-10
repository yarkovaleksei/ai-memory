package app

import (
	"testing"

	"github.com/yarkovaleksei/ai-memory/internal/config"
)

func TestInfoReturnsIndependentCopy(t *testing.T) {
	cfg := config.Config{ClaudeProfiles: []string{"/fx/.claude"}}
	svc := NewInfoService("1.2.3", cfg)

	// Изменение исходного Config после создания сервиса не должно протекать внутрь.
	cfg.ClaudeProfiles[0] = "/changed"
	first := svc.Info()
	first.ClaudeProfiles[0] = "/changed-by-caller"

	got := svc.Info()
	if got.Version != "1.2.3" {
		t.Errorf("Version = %q, want 1.2.3", got.Version)
	}
	if got.ClaudeProfiles[0] != "/fx/.claude" {
		t.Errorf("ClaudeProfiles[0] = %q, want /fx/.claude", got.ClaudeProfiles[0])
	}
}
