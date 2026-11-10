package markdown

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Frontmatter — редактируемый frontmatter. Хранит исходный yaml.Node, поэтому
// порядок ключей, неизвестные поля и комментарии переживают правку (AGENTS.md).
type Frontmatter struct {
	root *yaml.Node // MappingNode
}

// NewFrontmatter — пустой frontmatter для нового файла.
func NewFrontmatter() *Frontmatter {
	return &Frontmatter{root: &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}}
}

// ParseFrontmatterNode разбирает текст frontmatter (без строк «---»).
func ParseFrontmatterNode(fm string) (*Frontmatter, error) {
	if strings.TrimSpace(fm) == "" {
		return NewFrontmatter(), nil
	}
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(fm), &doc); err != nil {
		return nil, fmt.Errorf("frontmatter: %w", err)
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) != 1 || doc.Content[0].Kind != yaml.MappingNode {
		return nil, fmt.Errorf("frontmatter: ожидался словарь ключ: значение")
	}
	return &Frontmatter{root: doc.Content[0]}, nil
}

// Get возвращает строковое значение по пути ключей и признак, что ключ есть.
func (f *Frontmatter) Get(path ...string) (string, bool) {
	n := f.lookup(path)
	if n == nil || n.Kind != yaml.ScalarNode {
		return "", false
	}
	return n.Value, true
}

// Has сообщает, есть ли ключ по пути (любого вида).
func (f *Frontmatter) Has(path ...string) bool { return f.lookup(path) != nil }

// Set задаёт строковое значение по пути, создавая недостающие словари в конце.
// Существующий ключ остаётся на своём месте и сохраняет стиль кавычек.
func (f *Frontmatter) Set(value string, path ...string) {
	cur := f.root
	for i, key := range path {
		last := i == len(path)-1
		var val *yaml.Node
		for j := 0; j+1 < len(cur.Content); j += 2 {
			if cur.Content[j].Value == key {
				val = cur.Content[j+1]
				break
			}
		}
		if val == nil {
			val = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
			if last {
				val = &yaml.Node{Kind: yaml.ScalarNode}
			}
			cur.Content = append(cur.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}, val)
		}
		if last {
			if val.Kind != yaml.ScalarNode {
				*val = yaml.Node{Kind: yaml.ScalarNode}
			}
			val.Value = value
			// Значение — строка: «42» не должно стать числом. Исключение — дата и
			// время: Claude пишет их без кавычек, и так же пишем мы.
			val.Tag = "!!str"
			if _, err := time.Parse(time.RFC3339Nano, value); err == nil {
				val.Tag = "!!timestamp"
			}
			if val.Style&(yaml.LiteralStyle|yaml.FoldedStyle) != 0 && !strings.Contains(value, "\n") {
				val.Style = 0
			}
			return
		}
		if val.Kind != yaml.MappingNode {
			*val = yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		}
		cur = val
	}
}

func (f *Frontmatter) lookup(path []string) *yaml.Node {
	cur := f.root
	for _, key := range path {
		if cur == nil || cur.Kind != yaml.MappingNode {
			return nil
		}
		var next *yaml.Node
		for j := 0; j+1 < len(cur.Content); j += 2 {
			if cur.Content[j].Value == key {
				next = cur.Content[j+1]
				break
			}
		}
		cur = next
	}
	return cur
}

// String собирает текст frontmatter без строк «---», с переводом строки в конце.
func (f *Frontmatter) String() (string, error) {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(f.root); err != nil {
		return "", fmt.Errorf("frontmatter: %w", err)
	}
	if err := enc.Close(); err != nil {
		return "", fmt.Errorf("frontmatter: %w", err)
	}
	return buf.String(), nil
}

// Join собирает файл: frontmatter между «---» и тело через пустую строку.
func Join(fm *Frontmatter, body string) (string, error) {
	text, err := fm.String()
	if err != nil {
		return "", err
	}
	body = strings.TrimLeft(body, "\n")
	if body != "" && !strings.HasSuffix(body, "\n") {
		body += "\n"
	}
	return "---\n" + text + "---\n\n" + body, nil
}
