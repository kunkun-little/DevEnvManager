package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Release struct {
	LTS     string `json:"lts,omitempty"`
	Version string `json:"version"`
	Channel string `json:"channel"`
	Error   string `json:"error"`
}
type releaseSource struct{ url, channel string }

var releaseSources = map[string]releaseSource{
	"go-version":     {"https://api.github.com/repos/golang/go/git/matching-refs/tags/go1.", "Go 稳定版"},
	"node-version":   {"https://nodejs.org/dist/index.json", "Node.js 正式版"},
	"npm-version":    {"https://registry.npmjs.org/npm/latest", "npm latest"},
	"pnpm-version":   {"https://registry.npmjs.org/pnpm/latest", "pnpm latest"},
	"python-version": {"https://www.python.org/api/v2/downloads/release/?is_published=true", "Python 稳定版"},
}
var stableVersion = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`)

func bestVersion(a, b string) string {
	if !stableVersion.MatchString(b) {
		return a
	}
	if a == "" {
		return b
	}
	av, bv := strings.Split(a, "."), strings.Split(b, ".")
	for k := range av {
		x, _ := strconv.Atoi(av[k])
		y, _ := strconv.Atoi(bv[k])
		if y > x {
			return b
		}
		if y < x {
			return a
		}
	}
	return a
}
func parseRelease(id string, b []byte) (string, error) {
	var latest string
	switch id {
	case "npm-version", "pnpm-version":
		var v struct{ Version string }
		if e := json.Unmarshal(b, &v); e != nil {
			return "", e
		}
		latest = bestVersion("", v.Version)
	default:
		var rows []struct {
			Version   json.RawMessage
			Ref       string
			LTS       any
			Name      string
			Published bool `json:"is_published"`
			Preview   bool `json:"pre_release"`
		}
		if e := json.Unmarshal(b, &rows); e != nil {
			return "", e
		}
		for _, r := range rows {
			v := ""
			switch id {
			case "go-version":
				v = strings.TrimPrefix(r.Ref, "refs/tags/go")
			case "node-version", "node-lts":
				if lts, ok := r.LTS.(string); id == "node-version" || (ok && lts != "") {
					_ = json.Unmarshal(r.Version, &v)
					v = strings.TrimPrefix(v, "v")
				}
			case "python-version":
				if r.Published && !r.Preview {
					v = strings.TrimPrefix(r.Name, "Python ")
				}
			}
			latest = bestVersion(latest, v)
		}
	}
	if latest == "" {
		return "", errors.New("发布源没有返回可识别的正式版本")
	}
	return latest, nil
}

type cachedRelease struct {
	sync.Mutex
	result  Release
	expires time.Time
}

var releaseCache sync.Map
var releaseClient = &http.Client{Timeout: 8 * time.Second}

func (a *App) LatestVersion(id string) Release {
	s, ok := releaseSources[id]
	if !ok {
		return Release{Error: "无此发布源"}
	}
	entry, _ := releaseCache.LoadOrStore(id, &cachedRelease{})
	c := entry.(*cachedRelease)
	c.Lock()
	defer c.Unlock()
	if c.expires.After(time.Now()) {
		return c.result
	}
	result := Release{Channel: s.channel}
	v, e := func() (string, error) {
		r, e := releaseClient.Get(s.url)
		if e != nil {
			return "", e
		}
		defer r.Body.Close()
		if r.StatusCode != 200 {
			return "", fmt.Errorf("HTTP %d", r.StatusCode)
		}
		const max = 3 << 20
		b, e := io.ReadAll(io.LimitReader(r.Body, max+1))
		if e != nil {
			return "", e
		}
		if len(b) > max {
			return "", errors.New("发布索引超出大小限制")
		}
		if id == "node-version" {
			result.LTS, _ = parseRelease("node-lts", b)
		}
		return parseRelease(id, b)
	}()
	if e != nil {
		result.Error = "暂时无法查询 · " + s.channel
		if c.result.Version != "" {
			result.Version = c.result.Version
			result.LTS = c.result.LTS
			result.Error += "（上次查询结果）"
		}
		c.expires = time.Now().Add(15 * time.Second)
	} else {
		result.Version = v
		c.expires = time.Now().Add(30 * time.Minute)
	}
	c.result = result
	return result
}
