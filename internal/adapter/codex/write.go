package codex

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/yarkovaleksei/ai-memory/internal/markdown"
	"github.com/yarkovaleksei/ai-memory/internal/model"
	"github.com/yarkovaleksei/ai-memory/internal/writer"
)

// Планы записи для памяти Codex. Единственный канал — ad-hoc-заметки в активном
// поколении (ADR-002, Q3): новая заметка, правка заметки, заметка «забыть».
// Удалять файлы заметок нельзя никогда; это же проверяет Guard.

// now подменяется в тестах.
var now = time.Now

// NotesDir — каталог ad-hoc-заметок поколения.
func NotesDir(home string, g Generation) string {
	return filepath.Join(filepath.FromSlash(home), g.Dir(), notesDir)
}

// PlanNewNote — новая заметка «# Заголовок» с текстом.
func PlanNewNote(home string, g Generation, title, body string) (writer.Plan, error) {
	return planNote(home, g, "codex.note.create", "", title, body)
}

// PlanForget — заметка, которая просит Codex исключить факт из памяти. Сам
// текст просьбы задаёт пользователь: Codex читает заметки как данные.
func PlanForget(home string, g Generation, title, body string) (writer.Plan, error) {
	return planNote(home, g, "codex.note.forget", "forget-", title, body)
}

func planNote(home string, g Generation, code, prefix, title, body string) (writer.Plan, error) {
	p, err := PlanNotes(home, g, []NoteSpec{{Title: title, Body: body, Forget: prefix != ""}})
	if err != nil {
		return writer.Plan{}, err
	}
	p.Op = writer.Op{Code: code, Params: map[string]string{"title": strings.TrimSpace(title)}}
	return p, nil
}

// NoteSpec — заметка в плане: обычная или «забыть».
type NoteSpec struct {
	Title, Body string
	Forget      bool
}

// PlanNotes — несколько новых заметок одним планом (объединение дублей: итог
// и «забыть» для каждой записи Codex). Имена файлов не пересекаются ни с
// диском, ни друг с другом. Op плана задаёт вызывающий.
func PlanNotes(home string, g Generation, specs []NoteSpec) (writer.Plan, error) {
	dir := NotesDir(home, g)
	taken := map[string]bool{}
	var changes []writer.Change
	for _, sp := range specs {
		title := strings.TrimSpace(sp.Title)
		if title == "" || strings.Contains(title, "\n") {
			return writer.Plan{}, writer.Fail("codex.note.title-invalid")
		}
		if strings.TrimSpace(sp.Body) == "" {
			return writer.Plan{}, writer.Fail("codex.note.body-empty")
		}
		prefix := ""
		if sp.Forget {
			prefix = "forget-"
		}
		base := now().Format("2006-01-02") + "-" + prefix + Slug(title)
		path := filepath.Join(dir, base+".md")
		for i := 2; exists(path) || taken[path]; i++ {
			path = filepath.Join(dir, base+"-"+strconv.Itoa(i)+".md")
		}
		taken[path] = true
		changes = append(changes, writer.Change{Path: path, AfterExists: true,
			After: "# " + title + "\n\n" + strings.TrimSpace(sp.Body) + "\n"})
	}
	return writer.NewPlan(writer.Op{}, changes...), nil
}

// PlanUpdateNote заменяет текст заметки целиком; Codex вольёт правку при
// следующей консолидации.
func PlanUpdateNote(file, hash, content string) (writer.Plan, error) {
	data, err := os.ReadFile(file)
	if errors.Is(err, fs.ErrNotExist) {
		return writer.Plan{}, writer.Fail("write.conflict", "path", file, "reason", "deleted")
	}
	if err != nil {
		return writer.Plan{}, writer.Fail("write.unreadable", "path", file, "error", err.Error())
	}
	if hash != "" && model.ContentHash(data) != hash {
		return writer.Plan{}, writer.Fail("write.conflict", "path", file, "reason", "changed")
	}
	if strings.TrimSpace(content) == "" {
		// Пустая заметка — это фактически удаление, которое запрещено.
		return writer.Plan{}, writer.Fail("codex.note.body-empty")
	}
	if content == string(data) {
		return writer.Plan{}, writer.Fail("write.no-changes")
	}
	return writer.NewPlan(writer.Op{Code: "codex.note.update", Params: map[string]string{"file": filepath.Base(file)}},
		writer.Change{Path: file, BeforeExists: true, Before: string(data), AfterExists: true, After: content}), nil
}

// CancelNote — новое содержимое нашей заметки при её отмене. Файл заметки
// удалять нельзя (ADR-002), поэтому заголовок остаётся, а текст заменяется
// сообщением об отмене: правку заметки Codex вливает так же, как новую
// (ADR-014). Текст сообщения задаёт интерфейс.
func CancelNote(current, message string) string {
	title := markdown.FirstHeading(current)
	if title == "" {
		title = "note"
	}
	return "# " + title + "\n\n" + strings.TrimSpace(message) + "\n"
}

// IsNote — путь лежит прямо в каталоге заметок поколения.
func IsNote(home string, g Generation, path string) bool {
	return filepath.Dir(path) == NotesDir(home, g) && strings.EqualFold(filepath.Ext(path), ".md")
}

func exists(p string) bool {
	_, err := os.Lstat(p)
	return err == nil
}

var translit = map[rune]string{
	'а': "a", 'б': "b", 'в': "v", 'г': "g", 'д': "d", 'е': "e", 'ё': "e", 'ж': "zh", 'з': "z",
	'и': "i", 'й': "y", 'к': "k", 'л': "l", 'м': "m", 'н': "n", 'о': "o", 'п': "p", 'р': "r",
	'с': "s", 'т': "t", 'у': "u", 'ф': "f", 'х': "h", 'ц': "ts", 'ч': "ch", 'ш': "sh", 'щ': "sch",
	'ъ': "", 'ы': "y", 'ь': "", 'э': "e", 'ю': "yu", 'я': "ya",
}

// Slug — латинское имя файла из заголовка, как у заметок самого Codex
// (2026-11-03-minimal-scope-builds.md).
func Slug(title string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(title) {
		s, known := translit[r]
		switch {
		case r >= 'a' && r <= 'z' || r >= '0' && r <= '9':
			s = string(r)
		case known && s == "":
			continue // ь и ъ не разделяют слово
		}
		if s == "" {
			if b.Len() > 0 {
				dash = true
			}
			continue
		}
		if dash {
			b.WriteByte('-')
			dash = false
		}
		b.WriteString(s)
		if b.Len() >= 60 {
			break
		}
	}
	if b.Len() == 0 {
		return "note"
	}
	return strings.Trim(b.String(), "-")
}
