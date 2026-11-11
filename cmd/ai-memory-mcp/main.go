// ai-memory-mcp — MCP-мост ai-memory (фаза 5, ADR-026): stdio-сервер, через
// который Claude Code и Codex во время работы читают память друг друга.
//
// Зависит только от ядра из internal/, не от Wails и GTK: агент запускает его
// без окна ai-memory. Ничего не пишет на диск — ни в память агентов, ни в
// каталог данных ai-memory: индекс строится в памяти процесса.
//
// Подключение — в README.md, раздел «MCP-мост».
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/yarkovaleksei/ai-memory/internal/bridge"
	"github.com/yarkovaleksei/ai-memory/internal/config"
	"github.com/yarkovaleksei/ai-memory/internal/library"
	"github.com/yarkovaleksei/ai-memory/internal/model"
)

// version подменяется при сборке релиза через -ldflags "-X main.version=…".
var version = "0.0.1-dev"

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "ai-memory-mcp:", err)
		os.Exit(1)
	}
}

// parse разбирает флаги. Справка и сообщения об ошибках — в stderr: stdout
// занят протоколом.
func parse(args []string, stderr io.Writer) (bridge.Options, bool, error) {
	fs := flag.NewFlagSet("ai-memory-mcp", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var opt bridge.Options
	var agent string
	showVersion := fs.Bool("version", false, "print the version and exit")
	fs.StringVar(&opt.Project, "project", "", "project directory; default: the working directory")
	fs.StringVar(&agent, "agent", "", "connected agent, claude or codex; default: guessed from the client name. Search defaults to the other agent")
	fs.BoolVar(&opt.Global, "global", false, "also show global memory (not tied to a project)")
	fs.BoolVar(&opt.AllProjects, "all-projects", false, "show every project, not only the current one")
	fs.DurationVar(&opt.Refresh, "refresh", bridge.DefaultRefresh, "how often to re-read agent memory")
	if err := fs.Parse(args); err != nil {
		return opt, false, err
	}
	if fs.NArg() > 0 {
		return opt, false, fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " "))
	}
	switch model.Agent(agent) {
	case "", model.AgentClaude, model.AgentCodex:
		opt.Agent = model.Agent(agent)
	default:
		return opt, false, fmt.Errorf("-agent: %q, want claude or codex", agent)
	}
	opt.Version = version
	return opt, *showVersion, nil
}

// run запускает сервер. stdout нужен только для -version: в остальном он
// занят протоколом, и сервер пишет в него сам через StdioTransport.
func run(args []string, stdout, stderr io.Writer) error {
	opt, showVersion, err := parse(args, stderr)
	if err != nil {
		return err
	}
	if showVersion {
		fmt.Fprintln(stdout, version)
		return nil
	}
	log := slog.New(slog.NewTextHandler(stderr, nil))

	// Те же корни памяти, что у окна: переменные окружения и config.toml.
	env := config.FromEnvironment()
	base, err := config.Load(env)
	if err != nil {
		return fmt.Errorf("каталоги ai-memory: %w", err)
	}
	settings, err := config.ReadSettings(base.ConfigDir)
	if err != nil {
		log.Warn("настройки не прочитаны, используются умолчания", "error", err)
	}
	lib, err := library.NewInMemory(base.Apply(env, settings), log)
	if err != nil {
		return err
	}
	defer lib.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	b := bridge.New(lib, opt, log)
	// Память читается сразу, чтобы первый вызов инструмента не ждал; ошибка
	// повторится и вернётся агенту при вызове.
	if err := b.Refresh(ctx); err != nil {
		log.Warn("память не прочитана", "error", err)
	}
	if err := b.Server().Run(ctx, &mcp.StdioTransport{}); err != nil && ctx.Err() == nil {
		return err
	}
	return nil
}
