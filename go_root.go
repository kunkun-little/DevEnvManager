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
	ID        string `json:"id"`
	Target    string `json:"target"`
	Old       prior  `json:"old"`
	Effective string `json:"effective"`
}

var goRootOptions = []struct{ id, leaf string }{
	{"go-temp", "temp"}, {"go-mod", "mod"}, {"go-cache", "build"},
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
	return errors.New("已有 Go 统一目录的回滚记录。请先检查或撤销该批修改。")
}
func expandGoSelection(ids []string) ([]string, error) {
	if len(ids) == 0 {
		return nil, errors.New("请至少勾选一个 Go 目录")
	}
	valid := map[string]bool{}
	for _, id := range ids {
		if valid[id] {
			return nil, errors.New("选择了重复的 Go 目录")
		}
		found := false
		for _, o := range goRootOptions {
			if id == o.id {
				found = true
			}
		}
		if !found {
			return nil, errors.New("不支持统一配置这个 Go 目录")
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
	out, exe, e := probe("go env -json GOCACHE GOMODCACHE GOTMPDIR GOROOT GOPATH GOBIN GOENV GOCACHEPROG", w, 10*time.Second)
	if e != nil {
		return nil, exe, e
	}
	var values map[string]string
	if e = json.Unmarshal([]byte(out), &values); e != nil {
		return nil, exe, errors.New("无法解析 Go 的有效目录")
	}
	return values, exe, nil
}
func goRootPreflight(root string, ids []string) (string, []goChange, error) {
	ids, e := expandGoSelection(ids)
	if e != nil {
		return "", nil, e
	}
	root, e = localPath(strings.TrimSpace(root))
	if e != nil {
		return "", nil, e
	}
	w, e := workdir("")
	if e != nil {
		return "", nil, e
	}
	values, exe, e := getGoSettings(w)
	if e != nil {
		return "", nil, e
	}
	d, e := dataDir(false)
	if e != nil {
		return "", nil, e
	}
	effective := []string{values["GOCACHE"], values["GOMODCACHE"], values["GOTMPDIR"], values["GOROOT"], values["GOBIN"], values["GOENV"]}
	if values["GOTMPDIR"] == "" {
		effective = append(effective, os.TempDir())
	}
	gopaths := filepath.SplitList(values["GOPATH"])
	for _, p := range gopaths {
		if p != "" {
			for _, name := range []string{"src", "pkg", "bin"} {
				effective = append(effective, filepath.Join(p, name))
			}
		}
	}
	wanted := map[string]bool{}
	for _, id := range ids {
		wanted[id] = true
	}
	changes := make([]goChange, 0, len(ids))
	for _, o := range goRootOptions {
		if !wanted[o.id] {
			continue
		}
		i, _ := byID(o.id)
		key := i.row.Variable
		if _, e = os.Lstat(filepath.Join(d, o.id+".json")); !os.IsNotExist(e) {
			return "", nil, fmt.Errorf("%s 有单项回滚记录或无法检查，请先处理", key)
		}
		if key == "GOCACHE" && values["GOCACHEPROG"] != "" {
			return "", nil, errors.New("已配置外部 Go 缓存程序 GOCACHEPROG，不自动改动 GOCACHE")
		}
		user, e := regValue(false, key)
		if e != nil {
			return "", nil, e
		}
		machine, e := regValue(true, key)
		if e != nil {
			return "", nil, e
		}
		if user.Exists && (user.Kind != 1 || user.Value == "") {
			return "", nil, fmt.Errorf("%s 的用户设置类型特殊，拒绝覆盖", key)
		}
		expected := machine
		if user.Exists {
			expected = user
		}
		if expected.Exists && expected.Kind != 1 {
			return "", nil, fmt.Errorf("%s 的继承值来自可展开或特殊注册表项；无法安全确认覆盖优先级", key)
		}
		inherited, hasInherited := os.LookupEnv(key)
		if expected.Exists != hasInherited || expected.Value != inherited {
			return "", nil, fmt.Errorf("%s 的当前进程值和用户/系统持久设置不一致，可能有终端覆盖或环境还未刷新。请从资源管理器重开本应用；如仍不一致，请先检查该变量", key)
		}
		target, e := localPath(filepath.Join(root, o.leaf))
		if e != nil {
			return "", nil, fmt.Errorf("%s：%w", key, e)
		}
		if e = emptyTarget(target); e != nil {
			return "", nil, fmt.Errorf("%s：%w", key, e)
		}
		if inside(target, exe) || inside(exe, target) {
			return "", nil, fmt.Errorf("%s 与 Go 可执行文件路径重叠", key)
		}
		for _, p := range effective {
			if p != "" && filepath.IsAbs(p) && (inside(p, target) || inside(target, p)) {
				return "", nil, fmt.Errorf("%s 的目标和现有 Go 目录重叠：%s", key, p)
			}
		}
		oldPath := values[key]
		if oldPath == "" && key == "GOTMPDIR" {
			oldPath = os.TempDir() + "（默认临时目录）"
		}
		changes = append(changes, goChange{o.id, target, user, oldPath})
	}
	return exe, changes, nil
}
func (a *App) PlanGoRoot(root string, ids []string) Plan {
	if e := goBatchIsFree(); e != nil {
		return Plan{Error: e.Error()}
	}
	exe, changes, e := goRootPreflight(root, ids)
	if e != nil {
		return Plan{Error: e.Error()}
	}
	lines := []string{"使用 Go 当前有效目录作为参考；操作仅写所选的当前用户环境变量。", "不会修改 GOPATH、GOROOT、GOBIN、GOENV，也不复制、移动或删除旧内容。"}
	for _, c := range changes {
		i, _ := byID(c.ID)
		old := c.Effective
		if old == "" {
			old = "（未设置）"
		}
		lines = append(lines, fmt.Sprintf("%s：%s → %s", i.row.Variable, old, c.Target))
	}
	lines = append(lines, "请先停止 Go 构建。修改完成后从资源管理器重新打开终端和 IDE，运行 go env 核对。旧模块和缓存保留，新目录首次构建可能需重新下载。")
	return a.plan(request{action: "cfg-go", target: filepath.Clean(strings.TrimSpace(root)), exe: exe, goChanges: changes}, "统一设置所选 Go 目录", lines)
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
func ensureEmptyWritableGoTarget(p string) error {
	if _, e := localPath(p); e != nil {
		return e
	}
	if e := emptyTarget(p); e != nil {
		return e
	}
	if e := os.MkdirAll(p, 0700); e != nil {
		return e
	}
	if _, e := localPath(p); e != nil {
		return e
	}
	if e := emptyTarget(p); e != nil {
		return e
	}
	t, e := os.CreateTemp(p, ".devenv-write-")
	if e != nil {
		return e
	}
	name := t.Name()
	e = t.Close()
	del := os.Remove(name)
	if e == nil {
		e = del
	}
	return e
}
func (a *App) applyGoRoot(r request) Result {
	if e := goBatchIsFree(); e != nil {
		return asResult(e, "")
	}
	ids := []string{}
	for _, c := range r.goChanges {
		ids = append(ids, c.ID)
	}
	exe, changes, e := goRootPreflight(r.target, ids)
	if e != nil {
		return asResult(e, "")
	}
	if exe != r.exe || len(changes) != len(r.goChanges) {
		return Result{Error: "Go 安装或所选目录已变化，请重新预览"}
	}
	for k, c := range changes {
		if c != r.goChanges[k] {
			return Result{Error: "Go 环境变量或目标路径已变化，请重新预览"}
		}
	}
	for _, c := range changes {
		if e = ensureEmptyWritableGoTarget(c.Target); e != nil {
			return asResult(e, "")
		}
	}
	j, e := goBatchJournal(true)
	if e != nil {
		return asResult(e, "")
	}
	if e = writeGoJournal(j, changes); e != nil {
		return asResult(e, "")
	}
	if count, err := commitGoChanges(changes); err != nil {
		e = err
		defer notifyEnv()
		if recovery := restoreGoChanges(changes[:count]); recovery != nil {
			return Result{Error: "批量写入失败：" + e.Error() + "；恢复未确认：" + recovery.Error() + "。已保留回滚记录，请使用「撤销统一配置」检查。"}
		}
		if del := os.Remove(j); del != nil {
			return Result{Error: "写入失败，已恢复原变量但回滚记录无法移除：" + del.Error()}
		}
		return Result{Error: "写入失败，已恢复原变量；已创建的空目录会保留：" + e.Error()}
	}
	notifyEnv()
	return Result{Message: "所选 Go 变量已写入。旧目录未更改；可以统一撤销。请重新打开终端和 IDE，再运行 go env 验证。"}
}
func commitGoChanges(changes []goChange) (int, error) {
	return commitGoChangesWith(changes, regValue, regSetRaw)
}
func commitGoChangesWith(changes []goChange, read func(bool, string) (prior, error), write func(string, prior) error) (int, error) {
	for n, c := range changes {
		i, _ := byID(c.ID)
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
func restoreGoChanges(changes []goChange) error {
	return restoreGoChangesWith(changes, regValue, regSetRaw)
}
func restoreGoChangesWith(changes []goChange, read func(bool, string) (prior, error), write func(string, prior) error) error {
	// Preflight all values before changing any. A partially applied journal is recoverable.
	for _, c := range changes {
		i, _ := byID(c.ID)
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
		i, _ := byID(c.ID)
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
	defer notifyEnv()
	if e = restoreGoChanges(changes); e != nil {
		return asResult(e, "")
	}
	if e = os.Remove(j); e != nil {
		return Result{Error: "已恢复原变量，但无法删除回滚记录：" + e.Error()}
	}
	return Result{Message: "所选 Go 环境变量已恢复为原用户设置；不删除任何配置目录。请重开终端和 IDE 验证。"}
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
