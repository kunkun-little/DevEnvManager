package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type goChange struct {
	Variable  string `json:"variable,omitempty"`
	ID        string `json:"id"`
	Target    string `json:"target"`
	Old       prior  `json:"old"`
	Effective string `json:"effective"`
}

var goRootOptions = []struct{ id, leaf string }{
	{"go-temp", "temp"}, {"go-mod", "mod"}, {"go-cache", "build"}, {"go-path", "workspace"}, {"go-bin", "bin"},
	{"npm-prefix", "prefix"}, {"pnpm-store", "store"}, {"pnpm-cache", "cache"}, {"pnpm-state", "state"}, {"pnpm-global", "global"},
	{"node-cache", "compile"}, {"npm-cache", "npm-cache"}, {"pip-cache", "pip-cache"}, {"python-pycache", "pycache"},
}

const goRootFile = "go-root-batch.json"

func goBatchJournal(create bool) (string, error) {
	d, e := dataDir(create)
	if e != nil {
		return "", e
	}
	return filepath.Join(d, goRootFile), nil
}
func goBatchIsFree() error {
	j, e := goBatchJournal(false)
	if e != nil {
		return e
	}
	if _, e = os.Lstat(j); os.IsNotExist(e) {
		return nil
	}
	if e != nil {
		return e
	}
	return errors.New("已有批量设置的回滚记录。请先检查或撤销该批修改。")
}
func expandGoSelection(ids []string) ([]string, error) {
	if len(ids) == 0 {
		return nil, errors.New("请至少勾选一个目录")
	}
	valid := map[string]bool{}
	for _, id := range ids {
		if valid[id] {
			return nil, errors.New("选择了重复的目录")
		}
		found := false
		for _, o := range goRootOptions {
			if id == o.id {
				found = true
			}
		}
		if !found {
			return nil, errors.New("不支持批量配置这个目录")
		}
		valid[id] = true
	}
	out := []string{}
	for _, o := range goRootOptions {
		if valid[o.id] {
			out = append(out, o.id)
		}
	}
	return out, nil
}
func getGoSettings(w string) (map[string]string, string, error) {
	out, exe, e := probe("go env -json GOCACHE GOMODCACHE GOTMPDIR GOROOT GOPATH GOBIN GOENV GOCACHEPROG GOVERSION", w, 10*time.Second)
	if e != nil {
		return nil, exe, e
	}
	var values map[string]string
	if e = json.Unmarshal([]byte(out), &values); e != nil {
		return nil, exe, errors.New("无法解析 Go 的有效目录")
	}
	return values, exe, nil
}
func writeGoJournal(j string, changes []goChange) error {
	f, e := os.OpenFile(j, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		return e
	}
	e = json.NewEncoder(f).Encode(changes)
	if e == nil {
		e = f.Sync()
	}
	closeErr := f.Close()
	if e == nil {
		e = closeErr
	}
	if e != nil {
		_ = os.Remove(j)
	}
	return e
}
func readGoJournal(j string) ([]goChange, error) {
	st, e := os.Lstat(j)
	if e != nil || !st.Mode().IsRegular() || reparse(st) || st.Size() > 65536 {
		return nil, errors.New("没有有效的统一配置回滚记录")
	}
	f, e := os.Open(j)
	if e != nil {
		return nil, e
	}
	defer f.Close()
	data, e := io.ReadAll(io.LimitReader(f, 65537))
	if e != nil || len(data) > 65536 {
		return nil, errors.New("回滚记录不可读")
	}
	var changes []goChange
	if e = json.Unmarshal(data, &changes); e != nil {
		return nil, errors.New("回滚记录无法解析")
	}
	ids := []string{}
	for _, c := range changes {
		if !filepath.IsAbs(c.Target) || (c.Old.Exists && (c.Old.Kind != 1 || c.Old.Value == "")) || strings.ContainsRune(c.Old.Value, 0) {
			return nil, errors.New("回滚记录内容异常")
		}
		ids = append(ids, c.ID)
	}
	if _, e = expandGoSelection(ids); e != nil {
		return nil, e
	}
	return changes, nil
}
func commitGoChangesWith(changes []goChange, read func(bool, string) (prior, error), write func(string, prior) error) (int, error) {
	for n, c := range changes {
		i, err := journalItem(c.ID, c.Variable)
		if err != nil {
			return n, err
		}
		cur, e := read(false, i.row.Variable)
		if e != nil {
			return n, e
		}
		if cur != c.Old {
			return n, fmt.Errorf("%s 已被外部修改，停止写入", i.row.Variable)
		}
		if e = write(i.row.Variable, prior{Value: c.Target, Exists: true, Kind: 1}); e != nil {
			return n, e
		}
	}
	return len(changes), nil
}
func restoreGoChangesWith(changes []goChange, read func(bool, string) (prior, error), write func(string, prior) error) error {
	// Preflight all values before changing any. A partially applied journal is recoverable.
	for _, c := range changes {
		i, err := journalItem(c.ID, c.Variable)
		if err != nil {
			return err
		}
		v, e := read(false, i.row.Variable)
		if e != nil {
			return e
		}
		if v != c.Old && v != (prior{Value: c.Target, Exists: true, Kind: 1}) {
			return fmt.Errorf("%s 当前值不属于本记录，未覆盖", i.row.Variable)
		}
	}
	for k := len(changes) - 1; k >= 0; k-- {
		c := changes[k]
		i, err := journalItem(c.ID, c.Variable)
		if err != nil {
			return err
		}
		v, e := read(false, i.row.Variable)
		if e != nil {
			return e
		}
		if v == c.Old {
			continue
		}
		if v != (prior{Value: c.Target, Exists: true, Kind: 1}) {
			return fmt.Errorf("%s 刚被外部修改，未覆盖", i.row.Variable)
		}
		if e = write(i.row.Variable, c.Old); e != nil {
			return e
		}
	}
	return nil
}
func (a *App) RollbackGoRoot() Result {
	a.mu.Lock()
	defer a.mu.Unlock()
	release, lockErr := holdConfigLock()
	if lockErr != nil {
		return asResult(lockErr, "")
	}
	defer release()

	j, e := goBatchJournal(false)
	if e != nil {
		return asResult(e, "")
	}
	changes, e := readGoJournal(j)
	if e != nil {
		return asResult(e, "")
	}
	defer a.registry.notify()
	if e = restoreGoChangesWith(changes, a.registry.read, a.registry.write); e != nil {
		return asResult(e, "")
	}
	if e = os.Remove(j); e != nil {
		return Result{Error: "已恢复原变量，但无法删除回滚记录：" + e.Error()}
	}
	return Result{Message: "所选环境变量已恢复为原用户设置；不删除任何配置目录。请重开终端和 IDE 验证。"}
}

// GoRootRollbackAvailable only checks the app's portable recovery journal.
func (a *App) GoRootRollbackAvailable() bool {
	j, err := goBatchJournal(false)
	if err != nil {
		return false
	}
	st, err := os.Lstat(j)
	return err == nil && st.Mode().IsRegular() && !reparse(st)
}

// Read the Go configuration in one process. Keep the effective bin path visible
// even when GOBIN itself is unset.
func (a *App) InspectGo(cwd string) map[string]Snapshot {
	wd, e := workdir(cwd)
	var values map[string]string
	var exe string
	if e == nil {
		values, exe, e = getGoSettings(wd)
	}
	result := make(map[string]Snapshot)
	for _, i := range items {
		if i.row.Tool != "Go" {
			continue
		}
		s := readVariables(i)
		s.Path = exe
		if e != nil {
			s.Error = e.Error()
			if i.row.ID == "go-version" && exe == "" {
				s.RegisteredVersion = registeredVersion(i.row.ID)
			}
		} else {
			switch i.row.ID {
			case "go-version":
				s.Value = values["GOVERSION"]
			case "go-file":
				s.Value = values["GOENV"]
			case "go-root":
				s.Value = values["GOROOT"]
			case "go-bin":
				s.Value, _ = sourcePath(i, wd, values)
			default:
				s.Value = values[i.row.Variable]
			}
		}
		result[i.row.ID] = withSize(i, s)
	}
	return result
}
