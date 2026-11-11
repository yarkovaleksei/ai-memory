package updates

import (
	"regexp"
	"strconv"
)

// semver — версия сборки: X.Y.Z и необязательный хвост. Хвост бывает двух
// видов (Taskfile.yml, APP_VERSION): «-3-gabc123» и «-dirty» от git describe
// между тегами и «-dev» и прочие пререлизы.
var semver = regexp.MustCompile(`^v?(\d+)\.(\d+)\.(\d+)(-.*)?$`)

// describe — хвост git describe после тега: сборка сделана после выпуска
// X.Y.Z и по возможностям не старше его.
var describe = regexp.MustCompile(`^(-\d+-g[0-9a-f]+)?(-dirty)?$`)

// stableTag — тег выпуска, который приложение готово предложить: строго
// vX.Y.Z, без пререлизных хвостов.
var stableTag = regexp.MustCompile(`^v(\d+)\.(\d+)\.(\d+)$`)

type version struct {
	n   [3]int
	pre bool // пререлиз: младше X.Y.Z без хвоста
}

func parseVersion(s string) (version, bool) {
	m := semver.FindStringSubmatch(s)
	if m == nil {
		return version{}, false
	}
	var v version
	for i := range 3 {
		n, err := strconv.Atoi(m[i+1])
		if err != nil {
			return version{}, false
		}
		v.n[i] = n
	}
	v.pre = m[4] != "" && !describe.MatchString(m[4])
	return v, true
}

// Newer сообщает, что выпуск latest новее текущей сборки current. Сборка с
// непонятной версией считается старше любого выпуска: лучше предложить
// обновление, чем молча остаться на неизвестной версии.
func Newer(latest, current string) bool {
	l, ok := parseVersion(latest)
	if !ok || l.pre {
		return false
	}
	c, ok := parseVersion(current)
	if !ok {
		return true
	}
	for i := range 3 {
		if l.n[i] != c.n[i] {
			return l.n[i] > c.n[i]
		}
	}
	return c.pre
}
