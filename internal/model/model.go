// Package model — общие типы ядра: хранилища памяти, проекты и записи.
//
// Пакет не зависит ни от чего, кроме стандартной библиотеки: его импортируют
// адаптеры, индекс, writer и сервисы Wails.
package model

import (
	"crypto/sha256"
	"encoding/hex"
	"time"
)

// Agent — агент, которому принадлежит память.
type Agent string

const (
	AgentClaude Agent = "claude"
	AgentCodex  Agent = "codex"
)

// Source — одно хранилище памяти: профиль Claude Code или поколение памяти Codex.
type Source struct {
	ID       string    `json:"id"`       // "claude:/home/me/.claude", "codex:v2"
	Agent    Agent     `json:"agent"`    // чей это источник
	Label    string    `json:"label"`    // короткое имя для интерфейса: ".claude", "Codex v2"
	Root     string    `json:"root"`     // абсолютный путь корня памяти
	ReadOnly bool      `json:"readOnly"` // источник только для чтения целиком (Codex v1)
	Warnings []Warning `json:"warnings"` // проблемы на уровне источника
	// Files — хранилище инструкций и навыков, а не памяти (ADR-019): туда не
	// пишутся факты и заметки.
	Files bool `json:"files,omitempty"`
}

// ItemKind — вид записи памяти.
type ItemKind string

const (
	KindClaudeFact     ItemKind = "claude-fact"           // файл факта Claude
	KindClaudeIndex    ItemKind = "claude-index"          // MEMORY.md
	KindCodexSection   ItemKind = "codex-summary-section" // раздел memory_summary.md (и MEMORY.md v1)
	KindCodexRollout   ItemKind = "codex-rollout"         // rollout_summaries/*.md
	KindCodexAdHocNote ItemKind = "codex-adhoc-note"      // extensions/ad_hoc/notes/*.md
	KindCodexRaw       ItemKind = "codex-raw"             // raw_memories.md (v1)

	// Фаза 4 (ADR-010, ADR-019): инструкции и навыки агентов.
	KindInstructions ItemKind = "instructions" // CLAUDE.md, AGENTS.md и правила: файл целиком
	KindSkill        ItemKind = "skill"        // каталог с SKILL.md
)

// Memory — запись автопамяти агента, а не инструкции или навык: к ней
// применимы перенос факта, объединение и «Забыть» (ADR-019).
func (k ItemKind) Memory() bool {
	switch k {
	case KindInstructions, KindSkill:
		return false
	}
	return true
}

// WriteMode показывает, что интерфейс сможет сделать с записью начиная с фазы 2.
type WriteMode string

const (
	WriteDirect  WriteMode = "direct"   // факты Claude, ad-hoc-заметки
	WriteViaNote WriteMode = "via-note" // сгенерированное Codex: только через новую заметку
	WriteNone    WriteMode = "none"     // MEMORY.md, всё из Codex v1
)

// Warning — код и параметры, а не текст: текст собирает фронтенд по словарю (ADR-008).
type Warning struct {
	Code   string            `json:"code"`
	Params map[string]string `json:"params,omitempty"`
}

// NewWarning — короткая запись для частого случая с парами ключ-значение.
func NewWarning(code string, kv ...string) Warning {
	w := Warning{Code: code}
	if len(kv) > 1 {
		w.Params = make(map[string]string, len(kv)/2)
		for i := 0; i+1 < len(kv); i += 2 {
			w.Params[kv[i]] = kv[i+1]
		}
	}
	return w
}

// ProjectRef — к какому проекту адаптер отнёс запись. Нормализацией в Project
// занимается каталог: он сводит ссылки разных агентов на один и тот же путь.
type ProjectRef struct {
	Global  bool   // глобальная память без проекта
	Path    string // абсолютный путь проекта, если известен
	Encoded string // закодированное имя каталога Claude, если путь восстановить не удалось
	Temp    bool   // проект во временном каталоге (Q7)
}

// Item — одна запись памяти.
type Item struct {
	ID          string         `json:"id"`
	SourceID    string         `json:"sourceId"`
	ProjectID   string         `json:"projectId"` // проставляет каталог по Project
	Agent       Agent          `json:"agent"`
	Kind        ItemKind       `json:"kind"`
	Type        string         `json:"type"` // user | feedback | project | reference, если известен
	Title       string         `json:"title"`
	Description string         `json:"description"`
	Body        string         `json:"body"`        // Markdown без frontmatter
	Frontmatter map[string]any `json:"frontmatter"` // разобранный frontmatter или заголовок rollout-сводки
	Links       []Link         `json:"links"`
	File        string         `json:"file"`   // абсолютный путь
	Anchor      string         `json:"anchor"` // заголовок раздела внутри файла
	ModTime     time.Time      `json:"modTime"`
	Hash        string         `json:"hash"` // sha256 содержимого файла
	Write       WriteMode      `json:"write"`
	Warnings    []Warning      `json:"warnings"`
	// Pending — агент ещё не учёл запись: заметка Codex ждёт консолидации.
	Pending bool `json:"pending"`

	// Поля инструкций и навыков (ADR-019).
	// Role — откуда запись: для инструкций main | local | override | rule,
	// для навыков user | project | system | synced | memory.
	Role string `json:"role,omitempty"`
	// Target — куда ведёт симлинк каталога навыка, если навык подключён симлинком.
	Target string `json:"target,omitempty"`
	// Shared — на тот же каталог навыка указывают и другие записи: правка
	// затронет всех.
	Shared bool `json:"shared,omitempty"`
	// Repo — корень git-репозитория проекта, в котором лежит файл: изменение
	// появится в его git status.
	Repo string `json:"repo,omitempty"`
	// RealDir — каталог навыка с раскрытыми симлинками: по нему ищутся общие навыки.
	RealDir string `json:"-"`

	Project ProjectRef `json:"-"`
}

// Link — ссылка из текста записи на другую запись.
type Link struct {
	Text     string `json:"text"`     // как ссылка записана в тексте: slug или путь
	TargetID string `json:"targetId"` // ID записи, если нашлась
}

// ItemID строит стабильный идентификатор записи по источнику, файлу и якорю.
// Идентификатор не меняется при правке содержимого, поэтому годится для URL.
func ItemID(sourceID, relFile, anchor string) string {
	sum := sha256.Sum256([]byte(sourceID + "\x00" + relFile + "\x00" + anchor))
	return hex.EncodeToString(sum[:12])
}

// ContentHash — sha256 содержимого файла для оптимистичной блокировки (фаза 2).
func ContentHash(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

// Project — память одного проекта из всех хранилищ.
type Project struct {
	ID        string   `json:"id"`   // "global", "path:<путь>", "encoded:<имя>"
	Path      string   `json:"path"` // пусто для глобальной памяти и нераспознанных
	Label     string   `json:"label"`
	Global    bool     `json:"global"`
	Temp      bool     `json:"temp"`
	Resolved  bool     `json:"resolved"` // путь восстановлен
	Agents    []Agent  `json:"agents"`
	SourceIDs []string `json:"sourceIds"`
	ItemCount int      `json:"itemCount"`
}
