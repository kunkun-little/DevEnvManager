package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func overlap(a, b string) bool { return a != "" && b != "" && (inside(a, b) || inside(b, a)) }

func checkBatchConflict(ids []string) error {
	j, e := goBatchJournal(false)
	if e != nil {
		return e
	}
	if _, e = os.Lstat(j); os.IsNotExist(e) {
		return nil
	}
	changes, e := readGoJournal(j)
	if e != nil {
		return e
	}
	for _, c := range changes {
		for _, id := range ids {
			if c.ID == id {
				return errors.New("所选项目有批量撤销记录，请先撤销该批设置")
			}
		}
	}
	return nil
}

func (a *App) BatchItems() []string {
	j, e := goBatchJournal(false)
	if e != nil {
		return nil
	}
	changes, e := readGoJournal(j)
	if e != nil {
		return nil
	}
	var ids []string
	for _, c := range changes {
		ids = append(ids, c.ID)
	}
	return ids
}

func sourcePath(i item, wd string, goEnv map[string]string) (string, error) {
	if i.row.Tool == "Go" {
		key := i.row.Variable
		if key == "GOCACHE" && goEnv["GOCACHEPROG"] != "" {
			return "", errors.New("GOCACHE 由外部缓存程序管理")
		}
		if key == "GOBIN" && goEnv[key] == "" {
			paths := filepath.SplitList(goEnv["GOPATH"])
			if len(paths) > 0 {
				return filepath.Join(paths[0], "bin"), nil
			}
		}
		// Empty GOTMPDIR inherits a shared OS directory. It has no private data to migrate.
		return goEnv[key], nil
	}
	if i.row.Command == "" {
		return os.Getenv(i.row.Variable), nil
	}
	v, _, e := probe(i.row.Command, wd, 8*time.Second)
	if e == nil && i.row.Tool == "pnpm" {
		return packageDirectorySource(i, v, wd)
	}
	return v, e
}

func (a *App) prepareDirectories(target string, ids []string, policy string, batch bool) ([]goChange, error) {
	if !transfersData(policy) && policy != "keep" && policy != "delete" {
		return nil, errors.New("请选择旧数据处理方式")
	}
	ids, e := expandGoSelection(ids)
	if e != nil {
		return nil, e
	}
	if !batch && len(ids) != 1 {
		return nil, errors.New("单项设置只能选择一项")
	}
	if batch {
		e = goBatchIsFree()
	} else {
		e = checkBatchConflict(ids)
	}
	if e != nil {
		return nil, e
	}
	root, e := localPath(strings.TrimSpace(target))
	if e != nil {
		return nil, e
	}
	wd, e := workdir("")
	if e != nil {
		return nil, e
	}
	d, e := dataDir(false)
	if e != nil {
		return nil, e
	}
	var changes []goChange
	var gv map[string]string
	var pnpmVersion string
	for _, id := range ids {
		i, _ := byID(id)
		if i.row.Tool == "pnpm" {
			if pnpmVersion == "" {
				pnpmVersion, _, e = probe("pnpm --version", wd, 8*time.Second)
				if e != nil {
					return nil, e
				}
			}
			i, e = pnpmDirectoryItem(i, pnpmVersion)
			if e != nil {
				return nil, e
			}
		}
		key := i.row.Variable
		exe, e := resolve(exeName(i))
		if e != nil {
			return nil, e
		}
		if e = supportsVersion(i); e != nil {
			return nil, e
		}
		if _, e = os.Lstat(filepath.Join(d, id+".json")); !os.IsNotExist(e) {
			return nil, fmt.Errorf("%s 有未撤销的单项记录或记录不可读", key)
		}
		if i.row.Tool == "Go" && gv == nil {
			gv, _, e = getGoSettings(wd)
			if e != nil {
				return nil, e
			}
		}
		src, e := sourcePath(i, wd, gv)
		if e != nil {
			return nil, e
		}
		old, e := a.registry.read(false, key)
		if e != nil {
			return nil, e
		}
		expected := old
		if !expected.Exists {
			expected, e = a.registry.read(true, key)
			if e != nil {
				return nil, e
			}
		}
		inherited, ok := os.LookupEnv(key)
		if expected.Exists != ok || expected.Value != inherited || expected.Exists && (expected.Kind != 1 || expected.Value == "") {
			return nil, fmt.Errorf("%s 的用户、系统与进程设置不一致，请在新打开的工具窗口中核对", key)
		}
		dst := root
		if batch {
			for _, o := range goRootOptions {
				if o.id == id {
					dst = filepath.Join(root, o.leaf)
					break
				}
			}
		}
		if _, e = localPath(dst); e != nil {
			return nil, e
		}
		if inside(dst, exe) || inside(filepath.Dir(exe), dst) {
			return nil, fmt.Errorf("%s 的目标与安装目录重叠", key)
		}
		if strings.Contains(dst, ";") && id == "go-path" {
			return nil, errors.New("工作区目录不能包含分号")
		}
		if old.Exists && strings.EqualFold(filepath.Clean(old.Value), dst) {
			if batch {
				continue
			}
			return nil, fmt.Errorf("%s 已设置为该目录", key)
		}
		if gv != nil {
			if overlap(gv["GOROOT"], dst) || overlap(gv["GOENV"], dst) {
				return nil, errors.New("目标与 Go 安装目录或配置文件重叠")
			}
			if key != "GOPATH" {
				for _, gp := range filepath.SplitList(gv["GOPATH"]) {
					for _, leaf := range []string{"src", "pkg", "bin"} {
						path := filepath.Join(gp, leaf)
						if key == "GOMODCACHE" && inside(filepath.Join(gp, "pkg", "mod"), dst) || key == "GOBIN" && strings.EqualFold(path, dst) && leaf == "bin" {
							continue
						}
						if overlap(path, dst) {
							return nil, fmt.Errorf("目标与现有工作区的 %s 目录重叠", leaf)
						}
					}
				}
			}
			for _, other := range []string{"GOCACHE", "GOMODCACHE", "GOTMPDIR", "GOBIN"} {
				if other != key && gv[other] != "" && overlap(gv[other], dst) {
					return nil, fmt.Errorf("目标与 %s 重叠", other)
				}
			}
		}
		if i.row.Tool == "npm" || i.row.Tool == "pnpm" {
			if src == "" && policy != "keep" {
				return nil, fmt.Errorf("%s 未返回已配置的目录。请选择「仅切换目录」以保留默认位置的数据", i.row.Title)
			}
			if e = verifyPackageDirectory(i, wd, exe); e != nil {
				return nil, e
			}
		}
		if e = checkTransfer(i, src, dst, policy, wd); e != nil {
			return nil, fmt.Errorf("%s：%w", key, e)
		}
		changes = append(changes, goChange{ID: id, Target: dst, Old: old, Effective: src, Variable: key})
	}
	if len(changes) == 0 {
		return nil, errors.New("所选目录均已设置为目标路径")
	}
	for n, c := range changes {
		for _, other := range changes[n+1:] {
			if overlap(c.Target, other.Effective) || overlap(other.Target, c.Effective) || clearsOldData(policy) && overlap(c.Effective, other.Effective) {
				return nil, fmt.Errorf("%s 与 %s 的目录重叠，请分开设置", c.ID, other.ID)
			}
		}
	}
	return changes, nil
}

// Only explicit directory changes can discard a verified Go module cache.
func directoryCleaner(i item) item {
	if i.row.ID == "pnpm-store" {
		i.row.Cleanup = ""
	} // prune removes unreferenced packages from the active version only.
	if i.row.ID == "go-mod" {
		i.row.Cleanup = "go clean -modcache"
		i.guard = []string{filepath.Join("cache", "download")}
	}
	return i
}
func clearsOldData(policy string) bool { return policy == "delete" || policy == "migrate-clean" }
func transfersData(policy string) bool { return policy == "migrate" || policy == "migrate-clean" }

func checkTransfer(i item, src, dst, policy, wd string) error {
	for _, p := range filepath.SplitList(src) {
		if filepath.IsAbs(p) && overlap(p, dst) && !(policy == "keep" && strings.EqualFold(filepath.Clean(p), dst)) {
			return errors.New("旧目录与目标目录相同或互相包含")
		}
	}
	if policy == "keep" {
		return nil
	}
	if e := emptyTarget(dst); e != nil {
		return e
	}
	i = directoryCleaner(i)
	if clearsOldData(policy) && i.row.Cleanup == "" {
		return errors.New("该目录可能包含源码、命令或其他用户文件，请迁移或保留旧数据")
	}
	if src == "" {
		return nil
	}
	if len(filepath.SplitList(src)) != 1 {
		return errors.New("检测到多个目录，请选择仅切换配置并保留原文件")
	}
	p, e := localPath(src)
	if e != nil {
		return e
	}
	if inside(p, os.TempDir()) || inside(p, wd) {
		return errors.New("旧路径与共享临时目录或用户主目录重叠，请选择仅切换配置")
	}
	if _, e = os.Stat(p); os.IsNotExist(e) {
		return nil
	} else if e != nil {
		return e
	}
	if clearsOldData(policy) {
		c, e := checkClean(i, wd)
		if e != nil {
			return e
		}
		if !strings.EqualFold(c.Dir, p) {
			return errors.New("缓存位置已变化")
		}
	}
	return nil
}

func (a *App) PlanDirectories(target string, ids []string, policy string, batch bool) Plan {
	changes, e := a.prepareDirectories(target, ids, policy, batch)
	if e != nil {
		return Plan{Error: e.Error()}
	}
	action := "cfg"
	if batch {
		action = "cfg-go"
	}
	var paths []DirectoryPreview
	for _, c := range changes {
		i, _ := byID(c.ID)
		src := c.Effective
		if src == "" {
			src = "未配置专用目录"
		}
		paths = append(paths, DirectoryPreview{i.row.Title, c.Variable, src, c.Target})
	}
	descriptions := map[string]string{"migrate-clean": "先复制旧数据；所有新目录准备完成且配置写入成功后，再清理原位置的缓存。", "migrate": "将旧数据复制到新目录，保留原目录副本，以便核对和撤销。", "keep": "仅切换目录，沿用目标中已有的数据。", "delete": "先切换目录，再由原工具删除旧缓存数据。撤销配置无法恢复已删除的缓存。"}
	lines := []string{descriptions[policy], "请先关闭正在使用这些目录的进程。设置完成后，重新打开终端和 IDE。"}
	var packageKeys []string
	for _, c := range changes {
		i, _ := byID(c.ID)
		if i.row.Tool == "npm" || i.row.Tool == "pnpm" {
			packageKeys = append(packageKeys, c.Variable)
		}
		if c.ID == "npm-prefix" {
			lines = append(lines, "全局命令入口随 prefix 改变。本工具保留 PATH，切换后请核对 PATH 中的旧前缀。")
			if entry, e := resolve("pnpm"); e == nil && inside(c.Effective, entry) {
				lines = append(lines, "当前 pnpm 入口："+entry+"，位于旧前缀内。")
			}
		}
		if c.ID == "pnpm-store" {
			lines = append(lines, "设置的是 store 根目录，pnpm 会管理其版本子目录。既有项目可能需要重新安装依赖。")
		}
	}
	if len(packageKeys) > 0 {
		lines = append(lines, "写入当前用户变量："+strings.Join(packageKeys, "、")+"。原配置文件保留，撤销可恢复原变量。")
	}
	p := a.plan(request{action: action, target: strings.TrimSpace(target), goChanges: changes, policy: policy}, "确认设置后的目录", lines)
	p.Directories = paths
	return p
}

// Preserve the configuration-only semantics of the original desktop bindings.
func (a *App) PlanConfig(id, target string) Plan {
	return a.PlanDirectories(target, []string{id}, "keep", false)
}
func (a *App) PlanGoRoot(root string, ids []string) Plan {
	return a.PlanDirectories(root, ids, "keep", true)
}

func saveDirectoryJournal(changes []goChange, batch bool) (string, error) {
	if batch {
		p, e := goBatchJournal(true)
		if e != nil {
			return "", e
		}
		return p, writeGoJournal(p, changes)
	}
	d, e := dataDir(true)
	if e != nil {
		return "", e
	}
	c := changes[0]
	p := filepath.Join(d, c.ID+".json")
	f, e := os.OpenFile(p, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return "", e
	}
	e = json.NewEncoder(f).Encode(backup{Old: c.Old, Applied: c.Target, Variable: c.Variable})
	if e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e == nil {
		e = ce
	}
	if e != nil {
		_ = os.Remove(p)
	}
	return p, e
}

func prepareTarget(c goChange, policy string) error {
	if _, e := localPath(c.Target); e != nil {
		return e
	}
	if e := os.MkdirAll(c.Target, 0700); e != nil {
		return e
	}
	if _, e := localPath(c.Target); e != nil {
		return e
	}
	if policy != "keep" {
		if e := emptyTarget(c.Target); e != nil {
			return e
		}
	}
	marker, e := os.CreateTemp(c.Target, ".devenv-write-")
	if e != nil {
		return e
	}
	if e = marker.Close(); e != nil {
		return e
	}
	if e = os.Remove(marker.Name()); e != nil {
		return e
	}
	if !transfersData(policy) || c.Effective == "" {
		return nil
	}
	if _, e = os.Stat(c.Effective); os.IsNotExist(e) {
		return nil
	} else if e != nil {
		return e
	}
	return copyDirectory(c.Effective, c.Target)
}

func (a *App) ApplyConfig(token string) Result {
	a.mu.Lock()
	defer a.mu.Unlock()
	action := a.pending[token].action
	if action != "cfg" && action != "cfg-go" {
		return Result{Error: "目录确认已失效，请重新确定"}
	}
	r, e := a.take(token, action)
	if e != nil {
		return asResult(e, "")
	}
	release, e := holdConfigLock()
	if e != nil {
		return asResult(e, "")
	}
	defer release()
	batch := action == "cfg-go"
	var ids []string
	for _, c := range r.goChanges {
		ids = append(ids, c.ID)
	}
	changes, e := a.prepareDirectories(r.target, ids, r.policy, batch)
	if e != nil {
		return asResult(e, "")
	}
	if len(changes) != len(r.goChanges) {
		return Result{Error: "待修改项目已变化，请重新确认"}
	}
	for n, c := range changes {
		if c != r.goChanges[n] {
			return Result{Error: "生效目录或配置已改变，请重新确认"}
		}
	}
	for _, c := range changes {
		if e = prepareTarget(c, r.policy); e != nil {
			return Result{Error: fmt.Sprintf("用户配置尚未写入，原数据保留。请检查并保留或清空已准备的目标目录 %s 后重试：%v", c.Target, e)}
		}
	}
	journal, e := saveDirectoryJournal(changes, batch)
	if e != nil {
		return asResult(e, "")
	}
	defer a.registry.notify()
	if count, e := commitGoChangesWith(changes, a.registry.read, a.registry.write); e != nil {
		if re := restoreGoChangesWith(changes[:count], a.registry.read, a.registry.write); re != nil {
			return Result{Error: fmt.Sprintf("写入失败：%v；恢复失败：%v。撤销记录已保留。", e, re)}
		}
		if re := os.Remove(journal); re != nil {
			return asResult(re, "")
		}
		return asResult(e, "")
	}
	if clearsOldData(r.policy) {
		wd, _ := workdir("")
		for _, c := range changes {
			i, _ := byID(c.ID)
			i = directoryCleaner(i)
			if c.Effective == "" {
				continue
			}
			if _, err := os.Stat(c.Effective); os.IsNotExist(err) {
				continue
			}
			job, err := checkClean(i, wd)
			if err == nil && !strings.EqualFold(job.Dir, filepath.Clean(c.Effective)) {
				err = errors.New("旧缓存位置已变化")
			}
			if err == nil {
				_, _, err = probePath(i.row.Cleanup, wd, job.Exe, 75*time.Second)
			}
			if err != nil {
				return Result{Message: "目录设置已生效。旧缓存未全部清理：" + err.Error()}
			}
		}
	}
	return Result{Message: "目录设置已保存。请重新打开终端和 IDE 以使用新目录。"}
}
