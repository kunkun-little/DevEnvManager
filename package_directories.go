package main

import (
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

func pnpmDirectoryItem(i item, version string) (item, error) {
	major, _, ok := strings.Cut(strings.TrimSpace(version), ".")
	n, err := strconv.Atoi(major)
	if !ok || err != nil || n < 1 {
		return i, errors.New("无法识别 pnpm 版本，无法确定目录设置使用的变量")
	}
	if n >= 11 {
		i.row.Variable = "P" + i.row.Variable
	}
	return i, nil
}

// A rollback must use the variable named in the original plan even if pnpm
// was upgraded since then. Only allow keys belonging to this catalog item.
func journalItem(id, variable string) (item, error) {
	i, e := byID(id)
	if e != nil {
		return i, e
	}
	if i.row.Variable == "" {
		return i, errors.New("只读项目没有目录设置")
	}
	if variable == "" || variable == i.row.Variable {
		return i, nil
	}
	if i.row.Tool == "pnpm" && variable == "P"+i.row.Variable {
		i.row.Variable = variable
		return i, nil
	}
	return i, errors.New("撤销记录的变量与目录项目不匹配")
}

func pnpmConfiguredPath(s string) string {
	if s == "undefined" || s == "null" {
		return ""
	}
	return s
}

// A version directory belongs to pnpm. When no store root was configured,
// derive it only from an identifiable versioned store path.
func pnpmStoreRoot(effective string) (string, error) {
	p := filepath.Clean(effective)
	leaf := filepath.Base(p)
	if !filepath.IsAbs(p) || len(leaf) < 2 || leaf[0] != 'v' {
		return "", errors.New("无法识别当前 pnpm 存储根目录")
	}
	if _, err := strconv.ParseUint(leaf[1:], 10, 32); err != nil {
		return "", errors.New("无法识别当前 pnpm 存储版本子目录")
	}
	return filepath.Dir(p), nil
}

func packageDirectorySource(i item, value, wd string) (string, error) {
	value = pnpmConfiguredPath(value)
	if i.row.ID == "pnpm-store" && value == "" {
		p, _, e := probe("pnpm store path", wd, 8*time.Second)
		if e != nil {
			return "", e
		}
		return pnpmStoreRoot(p)
	}
	return value, nil
}

func verifyPackageDirectory(i item, wd, exe string) error {
	// npm can create the cache it reports even for "config get". Probe support
	// with a private path so a preview never creates the user's destination.
	data, err := dataDir(false)
	if err != nil {
		return err
	}
	target := filepath.Join(data, "Temp", "setting-check", i.row.Variable)
	result, _, e := probeWithEnv(i.row.Command, wd, exe, 8*time.Second, []string{i.row.Variable + "=" + target})
	if e != nil {
		return e
	}
	if !strings.EqualFold(filepath.Clean(result), target) {
		return fmt.Errorf("当前 %s 的 %s 未采用 %s，已停止设置", i.row.Tool, i.row.Title, i.row.Variable)
	}
	return nil
}

// Read the pnpm version once per scan. Limit concurrent CLI reads so scanning
// a machine with several installed tools does not launch every command at once.
func (a *App) InspectPnpm(cwd string) map[string]Snapshot {
	v, _ := byID("pnpm-version")
	ver := inspectItem(v, cwd)
	result := map[string]Snapshot{v.row.ID: ver}
	var mu sync.Mutex
	var wg sync.WaitGroup
	slots := make(chan struct{}, 2)
	for _, i := range items {
		if i.row.Tool != "pnpm" || i.row.Variable == "" {
			continue
		}
		if ver.Error != "" {
			result[i.row.ID] = Snapshot{Error: ver.Error}
			continue
		}
		configured, e := pnpmDirectoryItem(i, ver.Value)
		if e != nil {
			result[i.row.ID] = Snapshot{Error: e.Error()}
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			slots <- struct{}{}
			defer func() { <-slots }()
			snap := inspectItem(configured, cwd)
			mu.Lock()
			result[configured.row.ID] = snap
			mu.Unlock()
		}()
	}
	wg.Wait()
	return result
}
