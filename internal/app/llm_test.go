package app

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yarkovaleksei/ai-memory/internal/config"
	"github.com/yarkovaleksei/ai-memory/internal/llm"
	"github.com/yarkovaleksei/ai-memory/internal/model"
)

// llmFixture — сервис на фикстурах с подменённым запуском CLI: настоящие
// claude и codex в тестах не запускаются.
func llmFixture(t *testing.T, answer string, runErr error) (*LLMService, *[]llm.Cmd) {
	t.Helper()
	ls, _ := fixtureService(t)
	data := t.TempDir()
	s := NewLLMService(ls.lib, NewRuntime(config.Config{DataDir: filepath.ToSlash(data)}), nil)
	calls := &[]llm.Cmd{}
	s.newClient = func(dir string) *llm.Client {
		if dir != filepath.Join(data, "llm") {
			t.Errorf("рабочий каталог %q не в каталоге данных", dir)
		}
		return &llm.Client{
			WorkDir:  dir,
			LookPath: func(name string) (string, error) { return "/opt/bin/" + name, nil },
			Run: func(_ context.Context, c llm.Cmd) ([]byte, error) {
				*calls = append(*calls, c)
				return []byte(answer), runErr
			},
		}
	}
	return s, calls
}

func TestLLMMergeDraft(t *testing.T) {
	ctx := context.Background()
	s, calls := llmFixture(t, `{"is_error":false,"structured_output":{"title":"Менеджер пакетов",`+
		`"description":"Чем ставить пакеты","body":"Только yarn 4.","contradictions":[`+
		`{"records":[1,2],"summary":"разные версии"}]}}`, nil)
	groups, err := similarGroups(ctx, s.lib, SimilarQuery{})
	if err != nil || len(groups) == 0 {
		t.Fatalf("группы: %v", err)
	}
	ids := []string{groups[0].Items[0].ID, groups[0].Items[1].ID}

	d, err := s.MergeDraft(ctx, ids, model.AgentClaude)
	if err != nil {
		t.Fatal(err)
	}
	if d.Title != "Менеджер пакетов" || d.Body != "Только yarn 4.\n" || len(d.Contradictions) != 1 ||
		d.Contradictions[0].ItemIDs[0] != ids[0] || d.Contradictions[0].ItemIDs[1] != ids[1] {
		t.Errorf("черновик %+v", d)
	}
	if len(*calls) != 1 || !strings.Contains((*calls)[0].Stdin, groups[0].Items[1].Title) {
		t.Errorf("записи группы не попали в промпт")
	}

	if _, err := s.MergeDraft(ctx, ids[:1], model.AgentClaude); code(err) != "merge.too-few" {
		t.Errorf("одна запись: %v", err)
	}
	if _, err := s.MergeDraft(ctx, []string{ids[0], "нет-такой"}, model.AgentClaude); code(err) != "item.not-found" {
		t.Errorf("нет записи: %v", err)
	}
}

func TestLLMErrors(t *testing.T) {
	ctx := context.Background()
	s, _ := llmFixture(t, "", errors.New("claude: exit status 1: Not logged in"))
	groups, _ := similarGroups(ctx, s.lib, SimilarQuery{})
	ids := []string{groups[0].Items[0].ID, groups[0].Items[1].ID}
	_, err := s.MergeDraft(ctx, ids, model.AgentClaude)
	var ae *Error
	if !errors.As(err, &ae) || ae.Code != llm.ErrFailed || !strings.Contains(ae.Params["detail"], "Not logged in") {
		t.Errorf("ошибка CLI: %+v", err)
	}
}

func TestLLMContradictions(t *testing.T) {
	ctx := context.Background()
	s, calls := llmFixture(t, `{"is_error":false,"structured_output":{"contradictions":[`+
		`{"a":1,"b":2,"summary":"разные ветки"}]}}`, nil)
	rep, err := s.Contradictions(ctx, SimilarQuery{}, model.AgentClaude)
	if err != nil {
		t.Fatal(err)
	}
	groups, _ := similarGroups(ctx, s.lib, SimilarQuery{})
	if rep.Groups != len(groups) || rep.Checked != len(groups) || len(*calls) != 1 {
		t.Fatalf("отчёт %+v, вызовов %d", rep, len(*calls))
	}
	if len(rep.Contradictions) != 1 || len(rep.Contradictions[0].Items) != 2 ||
		rep.Contradictions[0].Items[0].ID != groups[0].Items[0].ID || rep.Contradictions[0].Items[1].Body == "" {
		t.Errorf("противоречия %+v", rep.Contradictions)
	}

	// Без групп CLI не запускается.
	rep, err = s.Contradictions(ctx, SimilarQuery{MinScore: 0.99}, model.AgentClaude)
	if err != nil || rep.Groups != 0 || rep.Contradictions == nil || len(*calls) != 1 {
		t.Errorf("пусто: %+v %v, вызовов %d", rep, err, len(*calls))
	}
}

func code(err error) string {
	var ae *Error
	if errors.As(err, &ae) {
		return ae.Code
	}
	return ""
}
