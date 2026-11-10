// Package catalog сводит записи всех адаптеров в проекты: один Project на путь,
// какими бы агентами и профилями этот путь ни был представлен.
package catalog

import (
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/yarkovaleksei/ai-memory/internal/model"
)

// GlobalID — проект для памяти, не привязанной к каталогу.
const GlobalID = "global"

// Build проставляет записям ProjectID и возвращает проекты в порядке показа:
// глобальная память, обычные проекты по имени, нераспознанные, временные.
func Build(items []model.Item) ([]model.Project, []model.Item) {
	byID := map[string]*model.Project{}
	agents := map[string]map[model.Agent]bool{}
	sources := map[string]map[string]bool{}

	out := make([]model.Item, len(items))
	for i, it := range items {
		id, path, resolved := key(it.Project)
		p, ok := byID[id]
		if !ok {
			p = &model.Project{ID: id, Path: path, Global: id == GlobalID, Resolved: resolved}
			p.Label = label(it.Project, path)
			byID[id] = p
			agents[id] = map[model.Agent]bool{}
			sources[id] = map[string]bool{}
		}
		if it.Project.Temp {
			p.Temp = true
		}
		p.ItemCount++
		agents[id][it.Agent] = true
		sources[id][it.SourceID] = true
		it.ProjectID = id
		out[i] = it
	}

	projects := make([]model.Project, 0, len(byID))
	for id, p := range byID {
		for _, a := range []model.Agent{model.AgentClaude, model.AgentCodex} {
			if agents[id][a] {
				p.Agents = append(p.Agents, a)
			}
		}
		for s := range sources[id] {
			p.SourceIDs = append(p.SourceIDs, s)
		}
		sort.Strings(p.SourceIDs)
		projects = append(projects, *p)
	}
	sort.Slice(projects, func(i, j int) bool {
		a, b := projects[i], projects[j]
		if ra, rb := rank(a), rank(b); ra != rb {
			return ra < rb
		}
		if la, lb := strings.ToLower(a.Label), strings.ToLower(b.Label); la != lb {
			return la < lb
		}
		return a.ID < b.ID
	})
	return projects, out
}

func rank(p model.Project) int {
	switch {
	case p.Global:
		return 0
	case p.Temp:
		return 3
	case !p.Resolved:
		return 2
	default:
		return 1
	}
}

// key — идентификатор проекта. На Windows и macOS файловые системы по умолчанию
// нечувствительны к регистру, поэтому путь сравнивается в нижнем регистре.
func key(ref model.ProjectRef) (id, path string, resolved bool) {
	switch {
	case ref.Global:
		return GlobalID, "", true
	case ref.Path != "":
		p := filepath.Clean(ref.Path)
		k := p
		if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
			k = strings.ToLower(k)
		}
		return "path:" + filepath.ToSlash(k), p, true
	case ref.Encoded != "":
		return "encoded:" + ref.Encoded, "", false
	default:
		return GlobalID, "", true
	}
}

// label — короткое имя для дерева: последний элемент пути. Для глобальной памяти
// пусто: надпись берёт фронтенд из словаря.
func label(ref model.ProjectRef, path string) string {
	switch {
	case path != "":
		return filepath.Base(path)
	case ref.Encoded != "":
		return ref.Encoded
	default:
		return ""
	}
}
