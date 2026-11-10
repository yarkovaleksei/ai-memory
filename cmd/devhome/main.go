// Команда devhome готовит дом для `wails3 task dev`: свежую копию фикстур, в
// которой выдуманный дом /home/me заменён путём самой копии.
//
// Окно разработки пишет в память, поэтому работает на копии, а не на
// fixtures/home. Замена пути нужна, чтобы проекты из памяти (например,
// /home/me/projects/demo) указывали на существующие каталоги копии: тогда
// видны их инструкции и навыки (ADR-019). Написано на Go, а не на sed и cp:
// так задача одинаково работает на Linux, macOS и Windows (ADR-009).
//
//	go run ./cmd/devhome <fixtures/home> <каталог копии>
package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// fakeHome — дом, от которого записаны пути в фикстурах (fixtures/README.md).
const fakeHome = "/home/me/"

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "использование: devhome <fixtures/home> <каталог копии>")
		os.Exit(2)
	}
	if err := run(os.Args[1], os.Args[2]); err != nil {
		fmt.Fprintln(os.Stderr, "devhome:", err)
		os.Exit(1)
	}
}

func run(src, dst string) error {
	dst, err := filepath.Abs(dst)
	if err != nil {
		return fmt.Errorf("путь копии: %w", err)
	}
	if err := os.RemoveAll(dst); err != nil {
		return fmt.Errorf("удаление старой копии: %w", err)
	}
	home := dst + string(filepath.Separator)
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		to := home
		if strings.HasSuffix(p, ".jsonl") {
			// Путь внутри JSON-строки: обратные слэши Windows экранируются.
			to = strings.ReplaceAll(home, `\`, `\\`)
		}
		out := strings.ReplaceAll(string(data), fakeHome, to)
		return os.WriteFile(target, []byte(out), 0o644)
	})
}
