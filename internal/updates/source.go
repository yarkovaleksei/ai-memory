// Package updates — обновление из приложения (ADR-031): найти выпуск на
// GitHub, скачать пакет своего формата, сверить его и подсказать, как
// поставить. Пакет — часть ядра: Wails не знает, в сеть ходит только по
// явному вызову.
package updates

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"
)

// Repo — репозиторий выпусков.
const Repo = "yarkovaleksei/ai-memory"

// LatestURL — последний опубликованный выпуск. Черновики и пререлизы GitHub
// сюда не отдаёт.
const LatestURL = "https://api.github.com/repos/" + Repo + "/releases/latest"

// DefaultHosts — куда разрешено ходить: API, страница релиза и хранилища
// вложений, на которые github.com перенаправляет скачивание.
var DefaultHosts = []string{"api.github.com", "github.com", "objects.githubusercontent.com", "release-assets.githubusercontent.com"}

const (
	maxAPIBody   = 1 << 20 // ответ API о выпуске
	maxSumsBody  = 64 << 10
	maxPackage   = 1 << 30 // пакеты ai-memory — десятки МиБ
	maxRedirects = 5
)

// Error — отказ кодом с параметрами: текст для человека собирает фронтенд
// по словарю (ADR-008).
type Error struct {
	Code   string
	Params map[string]string
	Err    error // исходная ошибка, для журнала
}

func (e *Error) Error() string {
	s := e.Code
	if len(e.Params) > 0 {
		s = fmt.Sprintf("%s %v", s, e.Params)
	}
	if e.Err != nil {
		s += ": " + e.Err.Error()
	}
	return s
}

func (e *Error) Unwrap() error { return e.Err }

func fail(err error, code string, kv ...string) *Error {
	e := &Error{Code: code, Err: err}
	if len(kv) > 1 {
		e.Params = map[string]string{}
		for i := 0; i+1 < len(kv); i += 2 {
			e.Params[kv[i]] = kv[i+1]
		}
	}
	return e
}

// Release — выпуск на GitHub.
type Release struct {
	Tag      string
	Version  string // тег без «v»
	NotesURL string // страница выпуска
	Assets   []Asset
}

// Asset — вложение выпуска.
type Asset struct {
	Name   string
	URL    string
	Size   int64
	Digest string // SHA-256 в hex из поля digest API; пусто, если GitHub его не дал
}

func (r Release) asset(name string) (Asset, bool) {
	for _, a := range r.Assets {
		if a.Name == name {
			return a, true
		}
	}
	return Asset{}, false
}

// Client ходит к GitHub. Нулевое значение не годится: см. NewClient.
type Client struct {
	LatestURL string
	Hosts     []string
	HTTP      *http.Client
	UserAgent string
}

// NewClient — клиент для настоящего GitHub.
func NewClient(userAgent string) *Client {
	return &Client{LatestURL: LatestURL, Hosts: DefaultHosts, HTTP: &http.Client{Timeout: 0}, UserAgent: userAgent}
}

// http — копия клиента, который не уходит с разрешённых хостов и не
// переходит на http.
func (c *Client) http() *http.Client {
	hc := *c.HTTP
	hc.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= maxRedirects {
			return fail(nil, "update.network", "reason", "redirects")
		}
		return c.allowed(req.URL)
	}
	return &hc
}

func (c *Client) allowed(u *url.URL) error {
	if u.Scheme != "https" || !slices.Contains(c.Hosts, u.Hostname()) {
		return fail(nil, "update.host", "host", u.Host)
	}
	return nil
}

func (c *Client) get(ctx context.Context, raw, accept string) (*http.Response, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fail(err, "update.host", "host", raw)
	}
	if err := c.allowed(u); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fail(err, "update.network")
	}
	req.Header.Set("Accept", accept)
	if c.UserAgent != "" {
		req.Header.Set("User-Agent", c.UserAgent)
	}
	resp, err := c.http().Do(req)
	if err != nil {
		var ue *Error
		if errors.As(err, &ue) {
			return nil, ue
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fail(err, "update.network")
	}
	return resp, nil
}

// Latest — последний выпуск. Нет выпуска (404) — ошибка update.no-release, а
// не «версия актуальна»: иначе опечатка в адресе молча выдала бы «обновлений
// нет».
func (c *Client) Latest(ctx context.Context) (Release, error) {
	resp, err := c.get(ctx, c.LatestURL, "application/vnd.github+json")
	if err != nil {
		return Release{}, err
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return Release{}, fail(nil, "update.no-release")
	case resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests:
		return Release{}, fail(nil, "update.rate-limited")
	case resp.StatusCode != http.StatusOK:
		return Release{}, fail(nil, "update.http", "status", fmt.Sprint(resp.StatusCode))
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxAPIBody+1))
	if err != nil {
		return Release{}, fail(err, "update.network")
	}
	if len(body) > maxAPIBody {
		return Release{}, fail(nil, "update.bad-release", "reason", "too-large")
	}
	var raw struct {
		TagName    string `json:"tag_name"`
		HTMLURL    string `json:"html_url"`
		Draft      bool   `json:"draft"`
		Prerelease bool   `json:"prerelease"`
		Assets     []struct {
			Name   string `json:"name"`
			URL    string `json:"browser_download_url"`
			Size   int64  `json:"size"`
			Digest string `json:"digest"`
			State  string `json:"state"`
		} `json:"assets"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return Release{}, fail(err, "update.bad-release", "reason", "json")
	}
	if raw.Draft || raw.Prerelease || !stableTag.MatchString(raw.TagName) {
		return Release{}, fail(nil, "update.no-release")
	}
	r := Release{Tag: raw.TagName, Version: strings.TrimPrefix(raw.TagName, "v"), NotesURL: raw.HTMLURL}
	for _, a := range raw.Assets {
		if a.State != "" && a.State != "uploaded" {
			continue // недокачанное вложение
		}
		digest, _ := strings.CutPrefix(a.Digest, "sha256:")
		if !strings.HasPrefix(a.Digest, "sha256:") {
			digest = ""
		}
		r.Assets = append(r.Assets, Asset{Name: a.Name, URL: a.URL, Size: a.Size, Digest: strings.ToLower(digest)})
	}
	return r, nil
}

// fetch пишет тело ответа в w, не больше limit байт. progress получает число
// принятых байт.
func (c *Client) fetch(ctx context.Context, raw string, w io.Writer, limit int64, progress func(int64)) (int64, error) {
	resp, err := c.get(ctx, raw, "application/octet-stream")
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, fail(nil, "update.http", "status", fmt.Sprint(resp.StatusCode))
	}
	var n int64
	buf := make([]byte, 64<<10)
	last := time.Time{}
	for {
		k, rerr := resp.Body.Read(buf)
		if k > 0 {
			if n+int64(k) > limit {
				return n, fail(nil, "update.size", "reason", "too-large")
			}
			if _, err := w.Write(buf[:k]); err != nil {
				return n, fail(err, "update.write")
			}
			n += int64(k)
			if progress != nil && time.Since(last) > 200*time.Millisecond {
				progress(n)
				last = time.Now()
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			if ctx.Err() != nil {
				return n, ctx.Err()
			}
			return n, fail(rerr, "update.network")
		}
	}
	if progress != nil {
		progress(n)
	}
	return n, nil
}

// ParseSums ищет сумму файла name в тексте формата sha256sum / shasum -a 256:
// «<hex>  <имя>», имя может начинаться с «*» (двоичный режим).
func ParseSums(text, name string) (string, bool) {
	for line := range strings.Lines(text) {
		f := strings.Fields(line)
		if len(f) != 2 {
			continue
		}
		if strings.TrimPrefix(f[1], "*") == name && isHexSHA256(f[0]) {
			return strings.ToLower(f[0]), true
		}
	}
	return "", false
}

func isHexSHA256(s string) bool {
	if len(s) != 64 {
		return false
	}
	for _, r := range s {
		if !strings.ContainsRune("0123456789abcdefABCDEF", r) {
			return false
		}
	}
	return true
}
