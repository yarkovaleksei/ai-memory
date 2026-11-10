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
	recs := fixtureItems(t, s)
	ids := []string{recs[0].ID, recs[1].ID}

	d, err := s.MergeDraft(ctx, ids, model.AgentClaude)
	if err != nil {
		t.Fatal(err)
	}
	if d.Title != "Менеджер пакетов" || d.Body != "Только yarn 4.\n" || len(d.Contradictions) != 1 ||
		d.Contradictions[0].ItemIDs[0] != ids[0] || d.Contradictions[0].ItemIDs[1] != ids[1] {
		t.Errorf("черновик %+v", d)
	}
	if len(*calls) != 1 || !strings.Contains((*calls)[0].Stdin, recs[1].Title) {
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
	recs := fixtureItems(t, s)
	ids := []string{recs[0].ID, recs[1].ID}
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
	recs := fixtureItems(t, s)
	got, err := s.Contradictions(ctx, []string{recs[0].ID, recs[1].ID}, model.AgentClaude)
	if err != nil {
		t.Fatal(err)
	}
	// Уходят только отмеченные записи.
	if len(*calls) != 1 || strings.Count((*calls)[0].Stdin, "<record ") != 2 {
		t.Fatalf("вызовов %d", len(*calls))
	}
	if len(got) != 1 || len(got[0].Items) != 2 || got[0].Items[0].ID != recs[0].ID || got[0].Items[1].Body == "" {
		t.Errorf("противоречия %+v", got)
	}

	// Одна запись — CLI не запускается.
	if _, err := s.Contradictions(ctx, []string{recs[0].ID}, model.AgentClaude); code(err) != "merge.too-few" ||
		len(*calls) != 1 {
		t.Errorf("одна запись: %v, вызовов %d", err, len(*calls))
	}
}

// fixtureItems — факты Claude из фикстур: у них есть тело и заголовок.
func fixtureItems(t *testing.T, s *LLMService) []model.Item {
	t.Helper()
	all, err := s.lib.All(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var out []model.Item
	for _, it := range all {
		if it.Kind == model.KindClaudeFact && it.Body != "" {
			out = append(out, it)
		}
	}
	if len(out) < 2 {
		t.Fatalf("в фикстурах меньше двух фактов: %d", len(out))
	}
	return out
}

func code(err error) string {
	var ae *Error
	if errors.As(err, &ae) {
		return ae.Code
	}
	return ""
}

// Инструкции отмечаются для проверки противоречий и дублей с памятью, но не
// для объединения: оно не умеет их убрать (ADR-024).
func TestLLMInstructionsOnlyForContradictions(t *testing.T) {
	ctx := context.Background()
	s, calls := llmFixture(t, `{"is_error":false,"structured_output":{"contradictions":[`+
		`{"a":1,"b":2,"kind":"duplicate","summary":"правило уже в CLAUDE.md"}]}}`, nil)
	all, err := s.lib.All(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var instr model.Item
	for _, it := range all {
		if it.Kind == model.KindInstructions {
			instr = it
			break
		}
	}
	if instr.ID == "" {
		t.Fatal("в фикстурах нет инструкций")
	}
	fact := fixtureItems(t, s)[0]
	got, err := s.Contradictions(ctx, []string{instr.ID, fact.ID}, model.AgentClaude)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Kind != llm.KindDuplicate || got[0].Items[0].ID != instr.ID {
		t.Errorf("находки %+v", got)
	}
	if _, err := s.MergeDraft(ctx, []string{instr.ID, fact.ID}, model.AgentClaude); code(err) != "plan.not-memory" || len(*calls) != 1 {
		t.Errorf("объединение с инструкциями: %v, вызовов %d", err, len(*calls))
	}
}
