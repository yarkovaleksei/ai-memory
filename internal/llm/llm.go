// Package llm спрашивает модель через CLI агентов владельца (ADR-005):
// `claude -p` и `codex exec`. Ответ — только черновик: он ничего не пишет в
// память, путь записи с diff остаётся прежним.
//
// Флаги подобраны так, чтобы вызов не создавал память сам о себе (Q8):
//   - Claude: `--no-session-persistence` (нет файла сессии) и `--tools ""`
//     (модель не может писать файлы, значит, и автопамять);
//   - Codex: `--ephemeral` — эфемерные сессии пропускают конвейер памяти.
//
// Claude Code всё равно создаёт пустой каталог проекта для текущего каталога,
// поэтому вызов идёт всегда из одного постоянного рабочего каталога ai-memory
// (WorkDir), а не из временных.
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/yarkovaleksei/ai-memory/internal/model"
)

// DefaultTimeout — сколько ждём ответа. Codex на сводку группы тратит десятки
// секунд, отчёт по всем группам — дольше.
const DefaultTimeout = 5 * time.Minute

// Коды ошибок: интерфейс собирает текст по словарю.
const (
	ErrNotFound  = "llm.not-found"  // CLI агента нет в PATH
	ErrFailed    = "llm.failed"     // CLI завершился с ошибкой; detail — хвост stderr
	ErrTimeout   = "llm.timeout"    // не уложились в Timeout
	ErrCanceled  = "llm.canceled"   // пользователь закрыл диалог
	ErrBadAnswer = "llm.bad-answer" // ответ не разобрался как JSON по схеме
	ErrAgent     = "llm.agent-invalid"
	ErrTooLarge  = "llm.too-large" // записи не помещаются в один запрос
)

// Error — отказ с кодом и параметрами.
type Error struct {
	Code   string
	Params map[string]string
}

func (e *Error) Error() string {
	if d := e.Params["detail"]; d != "" {
		return e.Code + ": " + d
	}
	return e.Code
}

func fail(code string, kv ...string) *Error {
	p := map[string]string{}
	for i := 0; i+1 < len(kv); i += 2 {
		p[kv[i]] = kv[i+1]
	}
	return &Error{Code: code, Params: p}
}

// Cmd — запуск внешней программы.
type Cmd struct {
	Path  string
	Args  []string
	Dir   string
	Stdin string
}

// Runner запускает программу и возвращает stdout. Ошибка выхода должна
// содержать stderr: по нему пользователь поймёт, что не так (не вошёл,
// кончился лимит). Тесты подставляют свой Runner и ничего не запускают.
type Runner func(ctx context.Context, c Cmd) (stdout []byte, err error)

// Client — доступ к CLI агентов.
type Client struct {
	WorkDir  string // постоянный рабочий каталог вызовов
	Run      Runner // nil — настоящий запуск
	LookPath func(string) (string, error)
	Timeout  time.Duration
}

var binaries = map[model.Agent]string{model.AgentClaude: "claude", model.AgentCodex: "codex"}

func (c *Client) lookPath(agent model.Agent) (string, bool) {
	name, ok := binaries[agent]
	if !ok {
		return "", false
	}
	look := c.LookPath
	if look == nil {
		look = exec.LookPath
	}
	p, err := look(name)
	return p, err == nil
}

// Available — агенты, чьи CLI найдены в PATH, в порядке Claude, Codex.
func (c *Client) Available() []model.Agent {
	out := []model.Agent{}
	for _, a := range []model.Agent{model.AgentClaude, model.AgentCodex} {
		if _, ok := c.lookPath(a); ok {
			out = append(out, a)
		}
	}
	return out
}

// Ask отправляет промпт агенту и разбирает ответ по JSON-схеме в out.
func (c *Client) Ask(ctx context.Context, agent model.Agent, prompt string, schema json.RawMessage, out any) error {
	bin, ok := c.lookPath(agent)
	if !ok {
		if _, known := binaries[agent]; !known {
			return fail(ErrAgent, "agent", string(agent))
		}
		return fail(ErrNotFound, "agent", string(agent))
	}
	if err := os.MkdirAll(c.WorkDir, 0o700); err != nil {
		return fmt.Errorf("рабочий каталог LLM: %w", err)
	}
	timeout := c.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var answer []byte
	var err error
	switch agent {
	case model.AgentClaude:
		answer, err = c.askClaude(ctx, bin, prompt, schema)
	case model.AgentCodex:
		answer, err = c.askCodex(ctx, bin, prompt, schema)
	}
	if err != nil {
		switch {
		case errors.Is(ctx.Err(), context.DeadlineExceeded):
			return fail(ErrTimeout, "agent", string(agent))
		case ctx.Err() != nil:
			return fail(ErrCanceled)
		}
		var le *Error
		if errors.As(err, &le) {
			return le
		}
		return fail(ErrFailed, "agent", string(agent), "detail", tail(err.Error()))
	}
	if err := decode(answer, out); err != nil {
		return fail(ErrBadAnswer, "agent", string(agent), "detail", tail(err.Error()))
	}
	return nil
}

func (c *Client) run(ctx context.Context, cmd Cmd) ([]byte, error) {
	if c.Run != nil {
		return c.Run(ctx, cmd)
	}
	return Exec(ctx, cmd)
}

// askClaude: промпт через stdin, ответ — JSON-объект результата, где
// structured_output — ответ по схеме. MCP-серверы не поднимаются: инструментов
// у модели всё равно нет, а запуск серверов только тянет время.
func (c *Client) askClaude(ctx context.Context, bin, prompt string, schema json.RawMessage) ([]byte, error) {
	out, err := c.run(ctx, Cmd{Path: bin, Dir: c.WorkDir, Stdin: prompt, Args: []string{
		"-p",
		"--no-session-persistence",
		"--tools", "",
		"--strict-mcp-config",
		"--disable-slash-commands",
		"--output-format", "json",
		"--json-schema", string(schema),
	}})
	if err != nil {
		return nil, err
	}
	var res struct {
		IsError          bool            `json:"is_error"`
		Result           string          `json:"result"`
		StructuredOutput json.RawMessage `json:"structured_output"`
	}
	if err := json.Unmarshal(out, &res); err != nil {
		return nil, fail(ErrBadAnswer, "agent", string(model.AgentClaude), "detail", tail(err.Error()))
	}
	if res.IsError {
		return nil, fail(ErrFailed, "agent", string(model.AgentClaude), "detail", tail(res.Result))
	}
	if len(res.StructuredOutput) > 0 && string(res.StructuredOutput) != "null" {
		return res.StructuredOutput, nil
	}
	return []byte(res.Result), nil
}

// askCodex: схема и ответ — во временных файлах внутри рабочего каталога;
// песочница только на чтение, сессия эфемерная.
func (c *Client) askCodex(ctx context.Context, bin, prompt string, schema json.RawMessage) ([]byte, error) {
	tmp, err := os.MkdirTemp(c.WorkDir, "codex-")
	if err != nil {
		return nil, fmt.Errorf("каталог ответа Codex: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmp) }()
	schemaFile := filepath.Join(tmp, "schema.json")
	answerFile := filepath.Join(tmp, "answer.json")
	if err := os.WriteFile(schemaFile, schema, 0o600); err != nil {
		return nil, fmt.Errorf("схема ответа Codex: %w", err)
	}
	if _, err := c.run(ctx, Cmd{Path: bin, Dir: c.WorkDir, Stdin: prompt, Args: []string{
		"exec",
		"--ephemeral",
		"--skip-git-repo-check",
		"-s", "read-only",
		"-C", c.WorkDir,
		"--color", "never",
		"--output-schema", schemaFile,
		"-o", answerFile,
		"-",
	}}); err != nil {
		return nil, err
	}
	out, err := os.ReadFile(answerFile)
	if err != nil {
		return nil, fail(ErrBadAnswer, "agent", string(model.AgentCodex), "detail", "нет ответа")
	}
	return out, nil
}

// decode терпим к обёртке ответа в блок кода Markdown.
func decode(answer []byte, out any) error {
	s := strings.TrimSpace(string(answer))
	if strings.HasPrefix(s, "```") {
		s = strings.TrimPrefix(s, "```json")
		s = strings.TrimPrefix(s, "```")
		s = strings.TrimSuffix(strings.TrimSpace(s), "```")
	}
	return json.Unmarshal([]byte(s), out)
}

// Exec — настоящий запуск: stdout отдельно, stderr — в текст ошибки.
func Exec(ctx context.Context, c Cmd) ([]byte, error) {
	cmd := exec.CommandContext(ctx, c.Path, c.Args...)
	cmd.Dir = c.Dir
	cmd.Stdin = strings.NewReader(c.Stdin)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = strings.TrimSpace(stdout.String())
		}
		return nil, fmt.Errorf("%s: %w: %s", filepath.Base(c.Path), err, msg)
	}
	return stdout.Bytes(), nil
}

// tail — последние строки сообщения: начало stderr у CLI обычно баннер.
func tail(s string) string {
	s = strings.TrimSpace(s)
	const max = 600
	if r := []rune(s); len(r) > max {
		return "…" + string(r[len(r)-max:])
	}
	return s
}
