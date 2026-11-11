package main

import (
	"fmt"
	"io"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/yarkovaleksei/ai-memory/internal/bridge"
	"github.com/yarkovaleksei/ai-memory/internal/model"
)

func TestParse(t *testing.T) {
	tests := []struct {
		args    []string
		want    bridge.Options
		version bool
		err     bool
	}{
		{args: nil, want: bridge.Options{Refresh: bridge.DefaultRefresh}},
		{args: []string{"-project", "/w/demo", "-agent", "codex", "-global", "-all-projects", "-refresh", "1m"},
			want: bridge.Options{Project: "/w/demo", Agent: model.AgentCodex, Global: true, AllProjects: true, Refresh: time.Minute}},
		{args: []string{"--agent=claude"}, want: bridge.Options{Agent: model.AgentClaude, Refresh: bridge.DefaultRefresh}},
		{args: []string{"-version"}, want: bridge.Options{Refresh: bridge.DefaultRefresh}, version: true},
		{args: []string{"-agent", "gemini"}, err: true},
		{args: []string{"extra"}, err: true},
		{args: []string{"-unknown"}, err: true},
	}
	for _, tt := range tests {
		t.Run(strings.Join(tt.args, " "), func(t *testing.T) {
			got, version, err := parse(tt.args, io.Discard)
			if (err != nil) != tt.err {
				t.Fatalf("err = %v", err)
			}
			if tt.err {
				return
			}
			tt.want.Version = got.Version
			if fmt.Sprintf("%+v", got) != fmt.Sprintf("%+v", tt.want) || version != tt.version {
				t.Errorf("parse = %+v %v, want %+v %v", got, version, tt.want, tt.version)
			}
		})
	}
}

// ADR-006 и AGENTS.md: MCP-мост собирается без Wails и GTK — агент запускает
// его там, где окна может не быть.
func TestNoWailsDependency(t *testing.T) {
	if testing.Short() {
		t.Skip("go list -deps")
	}
	out, err := exec.Command("go", "list", "-deps", ".").Output()
	if err != nil {
		t.Skipf("go list: %v", err)
	}
	for _, pkg := range strings.Fields(string(out)) {
		if strings.Contains(pkg, "wailsapp") || strings.Contains(pkg, "/internal/app") {
			t.Errorf("зависимость от %s", pkg)
		}
	}
}
