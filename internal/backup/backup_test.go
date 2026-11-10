package backup

import (
	"context"
	"strings"
	"testing"
)

func TestMapPath(t *testing.T) {
	tests := []struct{ in, want string }{
		{"/home/me/.claude/projects/p/memory/a.md", "files/home/me/.claude/projects/p/memory/a.md"},
		{"/home/me/x/../y.md", "files/home/me/y.md"},
	}
	for _, tt := range tests {
		if got := MapPath(tt.in); got != tt.want {
			t.Errorf("MapPath(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestValidRev(t *testing.T) {
	for _, bad := range []string{"--help", "HEAD", "abc", "zzzzzzz", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdefff"} {
		if validRev(bad) == nil {
			t.Errorf("%q должен отклоняться", bad)
		}
	}
	if validRev("0123abc") != nil {
		t.Error("короткий хэш должен проходить")
	}
}

func TestRecordLogAndFileAt(t *testing.T) {
	ctx := context.Background()
	r, err := Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	path := "/home/me/.claude/projects/p/memory/a.md"

	snap1, err := r.Record(ctx, []File{{Path: path}}, "снимок", nil)
	if err != nil {
		t.Fatal(err)
	}
	snap2, _ := r.Record(ctx, []File{{Path: path}}, "снимок", nil)
	if snap1 != snap2 {
		t.Error("снимок без изменений не должен создавать коммит")
	}
	op, err := r.Record(ctx, []File{{Path: path, Exists: true, Content: []byte("v1\n")}}, "claude.fact.create", map[string]string{"k": "v"})
	if err != nil {
		t.Fatal(err)
	}

	log, err := r.Log(ctx, 10)
	if err != nil || len(log) != 1 || log[0].Hash != op || !strings.Contains(string(log[0].Meta), `"k":"v"`) {
		t.Fatalf("Log: %+v %v", log, err)
	}
	if m, err := r.Meta(ctx, op); err != nil || !strings.Contains(string(m), `"k"`) {
		t.Errorf("Meta: %s %v", m, err)
	}
	after, _ := r.FileAt(ctx, op, path)
	before, _ := r.FileAt(ctx, op+"^", path)
	if !after.Exists || string(after.Content) != "v1\n" || before.Exists {
		t.Errorf("FileAt: after=%+v before=%+v", after, before)
	}

	// Повторное открытие не пересоздаёт репозиторий.
	r2, err := Open(ctx, strings.TrimSuffix(r.Dir(), "/"+Dir))
	if err != nil {
		t.Fatal(err)
	}
	if log2, _ := r2.Log(ctx, 10); len(log2) != 1 {
		t.Errorf("после повторного открытия: %d операций", len(log2))
	}
}
