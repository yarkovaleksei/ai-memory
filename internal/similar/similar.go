// Package similar ищет кандидатов в дубли среди записей памяти без LLM.
//
// Записи сравниваются по TF-IDF буквенных триграмм внутри слов, косинусной
// мерой. Триграммы ловят разные формы одного слова («пакеты» / «пакетов»), а
// технические слова (yarn, main, CHANGELOG.md) совпадают и между русскими
// фактами Claude и английскими разделами Codex. Частые триграммы весят меньше
// (IDF): они почти не различают записи.
//
// Объём памяти — сотни записей, поэтому сравнение «каждая с каждой» через
// инвертированный индекс занимает миллисекунды и не требует хранилища.
package similar

import (
	"math"
	"sort"
	"strings"
	"unicode"

	"github.com/yarkovaleksei/ai-memory/internal/model"
)

// Options — какие пары считать.
type Options struct {
	MinScore   float64 // порог сходства 0..1
	CrossAgent bool    // только пары записей разных агентов
	ProjectID  string  // только группы, где есть запись этого проекта; пусто — все
}

// DefaultMinScore подобран на фикстурах: дубли «yarn» и «релиз» между фактами
// Claude и записями Codex дают 0,25–0,47, несвязанные записи — не больше 0,12.
const DefaultMinScore = 0.2

// Pair — две похожие записи.
type Pair struct {
	A     string  `json:"a"`
	B     string  `json:"b"`
	Score float64 `json:"score"`
}

// Group — связная группа похожих записей: каждая похожа хотя бы на одну другую.
type Group struct {
	ItemIDs []string `json:"itemIds"`
	Pairs   []Pair   `json:"pairs"`
	Score   float64  `json:"score"` // наибольшее сходство пары в группе
}

// Comparable — записи, среди которых ищутся дубли: факты, заметки и разделы
// сводки. Индекс MEMORY.md, сырые воспоминания и сводки сессий — служебное или
// слишком длинное, их сходство с фактами — шум.
func Comparable(it model.Item) bool {
	switch it.Kind {
	case model.KindClaudeFact, model.KindCodexAdHocNote, model.KindCodexSection:
		return !it.Project.Temp && strings.TrimSpace(it.Body+it.Title) != ""
	}
	return false
}

// skipPair — пары, которые дублями не считаются: разделы одной сводки, копии
// одной записи Codex в двух поколениях и заметка Codex рядом с разделом сводки —
// консолидация переносит заметки в сводку намеренно.
func skipPair(a, b model.Item, opt Options) bool {
	if opt.CrossAgent && a.Agent == b.Agent {
		return true
	}
	if a.File == b.File {
		return true
	}
	if a.Agent == model.AgentCodex && b.Agent == model.AgentCodex {
		return a.SourceID != b.SourceID || a.Kind != b.Kind
	}
	return false
}

// Find возвращает группы похожих записей, самые похожие первыми.
func Find(items []model.Item, opt Options) []Group {
	var docs []model.Item
	for _, it := range items {
		if Comparable(it) {
			docs = append(docs, it)
		}
	}
	vecs := vectors(docs)

	// Инвертированный индекс: триграмма → (документ, вес).
	type posting struct {
		doc int
		w   float64
	}
	inv := map[string][]posting{}
	for i, v := range vecs {
		for g, w := range v {
			inv[g] = append(inv[g], posting{i, w})
		}
	}
	dots := map[[2]int]float64{}
	for _, list := range inv {
		for x := 0; x < len(list); x++ {
			for y := x + 1; y < len(list); y++ {
				dots[[2]int{list[x].doc, list[y].doc}] += list[x].w * list[y].w
			}
		}
	}

	var pairs []Pair
	for k, score := range dots {
		a, b := docs[k[0]], docs[k[1]]
		if score < opt.MinScore || skipPair(a, b, opt) {
			continue
		}
		pairs = append(pairs, Pair{A: a.ID, B: b.ID, Score: round(score)})
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].Score != pairs[j].Score {
			return pairs[i].Score > pairs[j].Score
		}
		return pairs[i].A+pairs[i].B < pairs[j].A+pairs[j].B
	})

	groups := group(pairs)
	if opt.ProjectID == "" {
		return groups
	}
	project := map[string]string{}
	for _, d := range docs {
		project[d.ID] = d.ProjectID
	}
	out := groups[:0]
	for _, g := range groups {
		for _, id := range g.ItemIDs {
			if project[id] == opt.ProjectID {
				out = append(out, g)
				break
			}
		}
	}
	return out
}

// group собирает пары в связные группы (union-find).
func group(pairs []Pair) []Group {
	parent := map[string]string{}
	var find func(string) string
	find = func(x string) string {
		if p, ok := parent[x]; ok && p != x {
			parent[x] = find(p)
			return parent[x]
		}
		parent[x] = x
		return x
	}
	for _, p := range pairs {
		ra, rb := find(p.A), find(p.B)
		if ra != rb {
			parent[ra] = rb
		}
	}
	byRoot := map[string]*Group{}
	var order []string
	for _, p := range pairs { // пары уже по убыванию сходства: порядок групп тоже
		r := find(p.A)
		g := byRoot[r]
		if g == nil {
			g = &Group{Score: p.Score}
			byRoot[r] = g
			order = append(order, r)
		}
		g.Pairs = append(g.Pairs, p)
		for _, id := range []string{p.A, p.B} {
			if !contains(g.ItemIDs, id) {
				g.ItemIDs = append(g.ItemIDs, id)
			}
		}
	}
	out := make([]Group, 0, len(order))
	for _, r := range order {
		out = append(out, *byRoot[r])
	}
	return out
}

// vectors строит нормированные TF-IDF-векторы триграмм.
func vectors(docs []model.Item) []map[string]float64 {
	tfs := make([]map[string]float64, len(docs))
	df := map[string]int{}
	for i, d := range docs {
		tf := trigrams(d.Title + " " + d.Description + " " + d.Body)
		tfs[i] = tf
		for g := range tf {
			df[g]++
		}
	}
	n := float64(len(docs))
	out := make([]map[string]float64, len(docs))
	for i, tf := range tfs {
		v := map[string]float64{}
		var norm float64
		for g, c := range tf {
			w := (1 + math.Log(c)) * math.Log(1+n/float64(df[g]))
			v[g] = w
			norm += w * w
		}
		if norm > 0 {
			norm = math.Sqrt(norm)
			for g := range v {
				v[g] /= norm
			}
		}
		out[i] = v
	}
	return out
}

// trigrams — счётчики триграмм слов текста с границами слова: «_ya», «yar»…
// Слова короче трёх букв пропускаются: предлоги и союзы не различают записи.
func trigrams(text string) map[string]float64 {
	out := map[string]float64{}
	for _, w := range strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}) {
		r := []rune("_" + w + "_")
		if len(r) < 5 {
			continue
		}
		for i := 0; i+3 <= len(r); i++ {
			out[string(r[i:i+3])]++
		}
	}
	return out
}

func round(x float64) float64 { return math.Round(x*1000) / 1000 }

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
