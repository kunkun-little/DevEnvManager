package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

type configRegistry struct {
	read   func(bool, string) (prior, error)
	write  func(string, prior) error
	notify func()
}

type App struct {
	registry       configRegistry
	mu             sync.Mutex
	pending        map[string]request
	ctxMu          sync.RWMutex
	ctx            context.Context
	packageRunning bool
}

func NewApp() *App {
	return &App{pending: make(map[string]request), registry: configRegistry{regValue, regSetRaw, notifyEnv}}
}
func (a *App) startup(ctx context.Context) {
	a.ctxMu.Lock()
	a.ctx = ctx
	a.ctxMu.Unlock()
}

type Row struct {
	ID          string `json:"id"`
	Tool        string `json:"tool"`
	Title       string `json:"title"`
	Command     string `json:"command"`
	Variable    string `json:"variable"`
	Cleanup     string `json:"cleanup"`
	Description string `json:"description"`
	Docs        string `json:"docs"`
	PackageID   string `json:"packageID"`
}
type Snapshot struct {
	SettingVariable   string `json:"settingVariable,omitempty"`
	RegisteredVersion string `json:"registeredVersion"`
	Rollback          bool   `json:"rollback"`
	Value             string `json:"value"`
	Path              string `json:"path"`
	User              string `json:"user"`
	Machine           string `json:"machine"`
	Inherited         string `json:"inherited"`
	SizeBytes         int64  `json:"sizeBytes"`
	SizeStatus        string `json:"sizeStatus"`
	Error             string `json:"error"`
}
type DirectoryPreview struct {
	Label    string `json:"label"`
	Variable string `json:"variable"`
	Current  string `json:"current"`
	Target   string `json:"target"`
}

type Plan struct {
	Directories []DirectoryPreview `json:"directories,omitempty"`
	Token       string             `json:"token"`
	Summary     string             `json:"summary"`
	Lines       []string           `json:"lines"`
	Error       string             `json:"error"`
}
type Result struct {
	Message string `json:"message"`
	Error   string `json:"error"`
}
type item struct {
	row   Row
	size  bool
	guard []string
}

func spec(id, tool, title, cmd, key, cleanup, description, docs, pkg string, size bool, guard ...string) item {
	return item{Row{id, tool, title, cmd, key, cleanup, description, docs, pkg}, size, guard}
}

const goDocs = "https://pkg.go.dev/cmd/go#hdr-Environment_variables"
const pipDocs = "https://pip.pypa.io/en/stable/cli/pip_cache/"
const pnpmDocs = "https://pnpm.io/settings"

var items = []item{
	spec("node-version", "Node.js", "版本", "node --version", "", "", "当前 PATH 选中的版本；安装管理核对 WinGet 登记的 Node.js LTS 或正式版。", "https://nodejs.org/en/download/", "OpenJS.NodeJS.LTS", false),
	spec("node-exe", "Node.js", "运行路径", "node -p process.execPath", "", "", "Node 实际启动路径；不会通过变量迁移现有安装。", "https://nodejs.org/api/process.html#processexecpath", "", false),
	spec("node-cache", "Node.js", "JS 编译缓存", "", "NODE_COMPILE_CACHE", "", "Node 22.1+ 才支持。设置会启用编译缓存并指定位置。重启 Node 进程。", "https://nodejs.org/api/cli.html#node_compile_cachedir", "", true),
	spec("npm-version", "npm", "版本", "npm --version", "", "", "当前 PATH 中 npm 入口的版本。通常随 Node.js 安装。", "https://docs.npmjs.com/cli/", "", false),
	spec("npm-cache", "npm", "下载缓存", "npm config get cache", "NPM_CONFIG_CACHE", "npm cache clean --force", "项目或进程配置可能覆盖用户值。清后离线安装可能缺包。", "https://docs.npmjs.com/cli/cache/", "", true, "_cacache"),
	spec("npm-prefix", "npm", "全局前缀", "npm config get prefix", "NPM_CONFIG_PREFIX", "", "全局包及命令入口的安装目录。切换后要核对 PATH 中的旧前缀和全局命令。", "https://docs.npmjs.com/cli/using-npm/config#prefix", "", false),
	spec("npm-config", "npm", "用户 npmrc", "npm config get userconfig", "", "", "仅报告配置文件路径，不读写配置及凭据。", "https://docs.npmjs.com/cli/using-npm/config#userconfig", "", false),
	spec("pnpm-version", "pnpm", "版本", "pnpm --version", "", "", "Corepack 模式仅用已存在的本地包；维护前核对 npm 全局安装或 WinGet 包。", "https://pnpm.io/installation", "pnpm.pnpm", false),
	spec("pnpm-store", "pnpm", "共享包存储根目录", "pnpm config get store-dir", "NPM_CONFIG_STORE_DIR", "pnpm store prune", "填写存储根目录。pnpm 在其中管理 v10 等版本子目录；既有项目切换存储后可能需要重新安装依赖。", "https://pnpm.io/cli/store", "", true, "files"),
	spec("pnpm-cache", "pnpm", "元数据缓存设置", "pnpm config get cache-dir", "NPM_CONFIG_CACHE_DIR", "", "显示 cache-dir 配置值。未设置时，pnpm 使用该版本的默认缓存位置。", pnpmDocs, "", true),
	spec("pnpm-state", "pnpm", "状态目录设置", "pnpm config get state-dir", "NPM_CONFIG_STATE_DIR", "", "工具状态不当成缓存清理。", pnpmDocs, "", false),
	spec("pnpm-global", "pnpm", "全局包目录设置", "pnpm config get global-dir", "NPM_CONFIG_GLOBAL_DIR", "", "全局包目录。旧包可能引用原位置，切换后请核对全局包与命令。", pnpmDocs, "", false),
	spec("python-version", "Python", "解释器版本", "python --version", "", "", "反映当前用户 PATH / 虚拟环境；WinGet 按当前解释器的主次版本匹配。", "https://docs.python.org/3/using/windows.html", "Python.Python.3.13", false),
	spec("python-pip", "Python", "pip 版本", "python -I -B -m pip --version", "", "", "隔离模式忽略用户 site-packages；如果 pip 仅安装在那里，本项可能不可见。", pipDocs, "", false),
	spec("pip-cache", "Python", "pip 下载及轮子缓存", "python -I -B -m pip cache dir", "PIP_CACHE_DIR", "python -I -B -m pip cache purge", "清理下载和 wheel 缓存，可能需重新下载或构建。", pipDocs, "", true, "http", "http-v2", "wheels"),
	spec("python-user", "Python", "用户包基目录", "python -I -B -m site --user-base", "", "", "属于安装路径，不应当作缓存。隔离查询可能与项目会话不同。", "https://docs.python.org/3/library/site.html", "", false),
	spec("python-pycache", "Python", "字节码缓存前缀", "", "PYTHONPYCACHEPREFIX", "", "仅处理已配置的集中缓存目录。解释器重开生效，-E/-I 可忽略。", "https://docs.python.org/3/using/cmdline.html#envvar-PYTHONPYCACHEPREFIX", "", true),
	spec("go-version", "Go", "版本", "go version", "", "", "探测强制使用本地 Go 版本，不下载被项目指定的工具链。", "https://go.dev/doc/toolchain", "GoLang.Go", false),
	spec("go-cache", "Go", "构建缓存", "go env GOCACHE", "GOCACHE", "go clean -cache", "可重建的编译结果；清理后首次构建变慢。", goDocs, "", true),
	spec("go-mod", "Go", "模块缓存", "go env GOMODCACHE", "GOMODCACHE", "", "包含模块、离线依赖以及下载的工具链。删除旧模块缓存后可能需要重新下载。", goDocs, "", true),
	spec("go-temp", "Go", "构建临时工作目录", "go env GOTMPDIR", "GOTMPDIR", "", "仅影响 Go 自身构建临时目录；不会改系统 TEMP。空值表示默认临时目录。", goDocs, "", true),
	spec("go-path", "Go", "工作区路径", "go env GOPATH", "GOPATH", "", "GOPATH 包含工作区、已安装命令，也可能包含源码。多个工作区路径请分别核对。", goDocs, "", false),
	spec("go-bin", "Go", "命令安装位置", "go env GOBIN", "GOBIN", "", "GOBIN 控制 go install 的命令安装位置。默认使用首个 GOPATH 下的 bin。", goDocs, "", false),
	spec("go-file", "Go", "Go 环境配置文件", "go env GOENV", "", "", "该文件是 go env -w 写入的位置；进程环境变量优先。", goDocs, "", false),
	spec("go-root", "Go", "Go 安装目录", "go env GOROOT", "", "", "当前运行的 Go 工具链根目录（GOROOT）。通过安装器或版本管理器维护。", goDocs, "", false),
}

func byID(id string) (item, error) {
	for _, v := range items {
		if v.row.ID == id {
			return v, nil
		}
	}
	return item{}, errors.New("不支持此项目")
}
func (a *App) Catalog() []Row {
	out := make([]Row, len(items))
	for i := range items {
		out[i] = items[i].row
	}
	return out
}

func exeName(i item) string {
	switch i.row.Tool {
	case "Node.js":
		return "node"
	case "Python":
		return "python"
	case "Go":
		return "go"
	default:
		return i.row.Tool
	}
}
func workdir(cwd string) (string, error) {
	if strings.TrimSpace(cwd) == "" {
		var e error
		cwd, e = os.UserHomeDir()
		if e != nil {
			return "", e
		}
	}
	if !filepath.IsAbs(cwd) {
		return "", errors.New("请输入绝对路径的可信项目目录")
	}
	st, e := os.Stat(cwd)
	if e != nil || !st.IsDir() {
		return "", errors.New("工作目录无法访问")
	}
	return filepath.Clean(cwd), nil
}
func baseDir() (string, error) {
	p, e := os.Executable()
	if e != nil {
		return "", e
	}
	p, e = filepath.EvalSymlinks(p)
	if e != nil {
		return "", e
	}
	return filepath.Dir(p), nil
}
func inside(base, p string) bool {
	r, e := filepath.Rel(strings.ToLower(filepath.Clean(base)), strings.ToLower(filepath.Clean(p)))
	return e == nil && (r == "." || (r != ".." && !strings.HasPrefix(r, ".."+string(filepath.Separator))))
}

var specialPart = regexp.MustCompile(`(?i)^(con|prn|aux|nul|com[1-9¹²³]|lpt[1-9¹²³])(\.|$)`)

func localPath(raw string) (string, error) {
	invalid := errors.New("仅允许本地固定盘上的专用绝对目录，不允许盘根、系统目录或目录链接")
	if !strings.ContainsRune(raw, ':') || !filepath.IsAbs(raw) || len(raw) < 4 || raw[1] != ':' || !strings.ContainsRune("/\\", rune(raw[2])) || strings.ContainsAny(raw[2:], `<>:"|?*%`) {
		return "", invalid
	}
	for _, c := range raw {
		if c < 32 {
			return "", invalid
		}
	}
	for _, c := range strings.FieldsFunc(raw[3:], func(r rune) bool { return r == '/' || r == '\\' }) {
		if strings.HasSuffix(c, ".") || strings.HasSuffix(c, " ") || specialPart.MatchString(c) {
			return "", invalid
		}
	}
	p := filepath.Clean(raw)
	if len(p) < 4 || !fixedDrive(p) {
		return "", invalid
	}
	h, _ := os.UserHomeDir()
	b, e := baseDir()
	if e != nil {
		return "", e
	}
	if inside(p, b) || inside(b, p) {
		return "", invalid
	}
	for _, v := range []string{h, os.Getenv("LOCALAPPDATA"), os.Getenv("APPDATA")} {
		if v != "" && inside(p, v) {
			return "", invalid
		}
	}
	for _, key := range []string{"SystemRoot", "ProgramFiles", "ProgramFiles(x86)", "ProgramData"} {
		if v := os.Getenv(key); v != "" && (inside(p, v) || inside(v, p)) {
			return "", invalid
		}
	}
	for n := p; ; n = filepath.Dir(n) {
		st, e := os.Lstat(n)
		if e != nil && !os.IsNotExist(e) {
			return "", e
		}
		if e == nil && (!st.IsDir() || reparse(st)) {
			return "", invalid
		}
		if filepath.Dir(n) == n {
			break
		}
	}
	return p, nil
}
func scan(p string, limit time.Duration, rejectLinks bool) (int64, string, error) {
	start := time.Now()
	var n int64
	var seen int
	var partial bool
	stop := errors.New("达到时间或条目上限")
	var walk func(string, int) error
	walk = func(dir string, depth int) error {
		if depth >= 128 {
			return stop
		}
		if time.Since(start) > limit || seen >= 40000 {
			return stop
		}
		st, e := os.Lstat(dir)
		if e != nil {
			return e
		}
		if !st.IsDir() || reparse(st) {
			return errors.New("目录被换成了文件或重解析点")
		}
		f, e := os.Open(dir)
		if e != nil {
			return e
		}
		defer f.Close()
		for {
			items, e := f.ReadDir(128)
			for _, d := range items {
				if time.Since(start) > limit || seen >= 40000 {
					return stop
				}
				seen++
				info, e := d.Info()
				if e != nil {
					if rejectLinks {
						return e
					}
					partial = true
					continue
				}
				if reparse(info) {
					if rejectLinks {
						return errors.New("缓存内有重解析点，已停止")
					}
					partial = true
					continue
				}
				if d.IsDir() {
					if e = walk(filepath.Join(dir, d.Name()), depth+1); e != nil {
						if rejectLinks || errors.Is(e, stop) {
							return e
						}
						partial = true
					}
				} else if info.Mode().IsRegular() {
					n += info.Size()
				}
			}
			if errors.Is(e, io.EOF) {
				return nil
			}
			if e != nil {
				return e
			}
		}
	}
	fail := walk(p, 0)
	if fail != nil && rejectLinks {
		return n, "未完成", fail
	}
	if fail != nil || partial {
		return n, "部分逻辑大小（有跳过或超限）", nil
	}
	return n, "逻辑大小（不等于实际磁盘占用）", nil
}

func (a *App) Inspect(id, cwd string) Snapshot {
	i, e := byID(id)
	var s Snapshot
	if e != nil {
		s.Error = e.Error()
		return s
	}
	if i.row.Tool == "pnpm" && i.row.Variable != "" {
		wd, err := workdir(cwd)
		if err != nil {
			return Snapshot{Error: err.Error()}
		}
		version, _, err := probe("pnpm --version", wd, 8*time.Second)
		if err != nil {
			return Snapshot{Error: err.Error()}
		}
		i, err = pnpmDirectoryItem(i, version)
		if err != nil {
			return Snapshot{Error: err.Error()}
		}
	}
	return inspectItem(i, cwd)
}
func inspectItem(i item, cwd string) Snapshot {
	s := readVariables(i)
	s.SettingVariable = i.row.Variable
	id := i.row.ID
	if i.row.Command != "" {
		w, e := workdir(cwd)
		if e != nil {
			s.Error = e.Error()
			return s
		}
		s.Value, s.Path, e = probe(i.row.Command, w, 8*time.Second)
		if e != nil {
			if s.Path == "" && strings.HasSuffix(id, "-version") {
				s.RegisteredVersion = registeredVersion(id)
			}
			s.Error = e.Error()
			return s
		}
		if i.row.Tool == "pnpm" && i.row.Variable != "" {
			s.Value, e = packageDirectorySource(i, s.Value, w)
			if e != nil {
				s.Error = e.Error()
			}
		}
	} else {
		s.Value = s.Inherited
	}
	return withSize(i, s)
}

func readVariables(i item) Snapshot {
	var s Snapshot
	if i.row.Variable != "" {
		if d, e := dataDir(false); e == nil {
			st, e := os.Lstat(filepath.Join(d, i.row.ID+".json"))
			s.Rollback = e == nil && st.Mode().IsRegular() && !reparse(st)
		}
		s.Inherited = os.Getenv(i.row.Variable)
		v, e := regValue(false, i.row.Variable)
		if e == nil {
			s.User = v.Value
		}
		m, e := regValue(true, i.row.Variable)
		if e == nil {
			s.Machine = m.Value
		}
	}
	return s
}

func withSize(i item, s Snapshot) Snapshot {
	if i.size && s.Value != "" && s.Value != "undefined" {
		p, e := localPath(s.Value)
		if e != nil {
			s.SizeStatus = "非安全可扫描路径"
			return s
		}
		if _, e = os.Stat(p); os.IsNotExist(e) {
			s.SizeStatus = "目录尚未创建"
			return s
		}
		s.SizeBytes, s.SizeStatus, _ = scan(p, 2*time.Second, false)
	}
	return s
}

type prior struct {
	Value  string
	Exists bool
	Kind   uint32
}
type backup struct {
	Variable string `json:",omitempty"`
	Old      prior
	Applied  string
}
type cleanItem struct{ ID, Dir, Exe string }
type request struct {
	action, id, target, cwd, verb, exe string
	old                                prior
	clean                              []cleanItem
	goChanges                          []goChange
	policy                             string
	packageState                       PackageState
	expires                            time.Time
}

func (a *App) plan(r request, summary string, lines []string) Plan {
	var b [24]byte
	if _, e := rand.Read(b[:]); e != nil {
		return Plan{Error: "无法生成确认凭证"}
	}
	token := hex.EncodeToString(b[:])
	r.expires = time.Now().Add(4 * time.Minute)
	a.mu.Lock()
	defer a.mu.Unlock()
	for k, v := range a.pending {
		if v.expires.Before(time.Now()) {
			delete(a.pending, k)
		}
	}
	if len(a.pending) >= 48 {
		return Plan{Error: "待确认操作过多，请稍后重试"}
	}
	a.pending[token] = r
	return Plan{Token: token, Summary: summary, Lines: lines}
}
func (a *App) take(token, action string) (request, error) {
	r, ok := a.pending[token]
	delete(a.pending, token)
	if !ok || r.action != action || r.expires.Before(time.Now()) {
		return r, errors.New("确认凭证无效或已过期，请重新预览")
	}
	return r, nil
}
func asResult(e error, msg string) Result {
	if e != nil {
		return Result{Error: e.Error()}
	}
	return Result{Message: msg}
}
func dataDir(create bool) (string, error) {
	d, e := baseDir()
	if e != nil {
		return "", e
	}
	d = filepath.Join(d, "DevEnvManagerData")
	fi, e := os.Lstat(d)
	if os.IsNotExist(e) && create {
		if e = os.Mkdir(d, 0700); e != nil && !os.IsExist(e) {
			return "", e
		}
		fi, e = os.Lstat(d)
	}
	if os.IsNotExist(e) && !create {
		return d, nil
	}
	if e != nil {
		return "", e
	}
	if !fi.IsDir() || reparse(fi) {
		return "", errors.New("应用数据目录不可使用重解析点")
	}
	return d, nil
}
func emptyTarget(p string) error {
	x, e := os.Open(p)
	if os.IsNotExist(e) {
		return nil
	}
	if e != nil {
		return e
	}
	defer x.Close()
	n, err := x.Readdirnames(1)
	if err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	if len(n) > 0 {
		return errors.New("请选择不存在或为空的工具专用目录")
	}
	return nil
}
func supportsVersion(i item) error {
	if i.row.ID != "node-cache" {
		return nil
	}
	w, e := workdir("")
	if e != nil {
		return e
	}
	v, _, e := probe("node --version", w, 8*time.Second)
	if e != nil {
		return e
	}
	parts := strings.Split(strings.TrimPrefix(v, "v"), ".")
	if len(parts) < 2 {
		return errors.New("无法辨认 Node.js 版本")
	}
	m, _ := strconv.Atoi(parts[0])
	n, _ := strconv.Atoi(parts[1])
	if m < 22 || m == 22 && n < 1 {
		return errors.New("Node.js 编译缓存需要 22.1+")
	}
	return nil
}
func (a *App) Rollback(id string) Result {
	a.mu.Lock()
	defer a.mu.Unlock()
	release, lockErr := holdConfigLock()
	if lockErr != nil {
		return asResult(lockErr, "")
	}
	defer release()

	i, e := byID(id)
	if e != nil || i.row.Variable == "" {
		return Result{Error: "此项目没有可回滚的变量"}
	}
	if e = checkBatchConflict([]string{id}); e != nil {
		return asResult(e, "")
	}
	d, e := dataDir(false)
	if e != nil {
		return asResult(e, "")
	}
	p := filepath.Join(d, id+".json")
	st, e := os.Lstat(p)
	if e != nil || !st.Mode().IsRegular() || reparse(st) || st.Size() > 65536 {
		return Result{Error: "没有有效的回滚记录"}
	}
	b, e := os.ReadFile(p)
	if e != nil {
		return asResult(e, "")
	}
	var rec backup
	if e = json.Unmarshal(b, &rec); e != nil {
		return asResult(e, "")
	}
	if rec.Applied == "" || (rec.Old.Exists && (rec.Old.Kind != 1 || rec.Old.Value == "")) {
		return Result{Error: "回滚记录字段无效"}
	}
	i, e = journalItem(id, rec.Variable)
	if e != nil {
		return asResult(e, "")
	}
	cur, e := a.registry.read(false, i.row.Variable)
	if e != nil {
		return asResult(e, "")
	}
	if !cur.Exists || cur.Kind != 1 || cur.Value != rec.Applied {
		return Result{Error: "当前值已经在外部变化，不覆盖"}
	}
	if e = a.registry.write(i.row.Variable, rec.Old); e != nil {
		return asResult(e, "")
	}
	a.registry.notify()
	if e = os.Remove(p); e != nil {
		return asResult(e, "")
	}
	return Result{Message: "已恢复原用户值；不删除曾使用的目录。重开终端和 IDE 后复查。"}
}
func checkClean(i item, w string) (cleanItem, error) {
	var out cleanItem
	if i.row.Cleanup == "" {
		return out, errors.New("此项不提供清理")
	}
	command := i.row.Command
	if i.row.ID == "pnpm-store" {
		command = "pnpm store path"
	}
	val, exe, e := probe(command, w, 8*time.Second)
	if e != nil {
		return out, e
	}
	p, e := localPath(val)
	if e != nil {
		return out, e
	}
	if inside(p, w) || inside(p, exe) {
		return out, errors.New("缓存与工作目录或安装入口重叠，拒绝清理")
	}
	if i.row.ID == "go-cache" {
		v, _, e := probe("go env GOCACHEPROG", w, 8*time.Second)
		if e != nil || v != "" {
			return out, errors.New("外部 Go 缓存程序状态不明确或已启用，拒绝清理")
		}
		r := filepath.Join(p, "README")
		m, e := os.Lstat(r)
		if e != nil || !m.Mode().IsRegular() || reparse(m) || m.Size() > 4096 {
			return out, errors.New("未识别到标准 Go 缓存")
		}
		b, e := os.ReadFile(r)
		if e != nil || !strings.HasPrefix(string(b), "This directory holds cached build artifacts from the Go build system.") {
			return out, errors.New("未识别到标准 Go 缓存")
		}
	} else {
		ok := false
		for _, name := range i.guard {
			if d, e := os.Lstat(filepath.Join(p, name)); e == nil && d.IsDir() && !reparse(d) {
				ok = true
				break
			}
		}
		if !ok {
			return out, errors.New("找不到工具专用缓存结构")
		}
	}
	if _, _, e = scan(p, 9*time.Second, true); e != nil {
		return out, e
	}
	return cleanItem{i.row.ID, p, exe}, nil
}
func (a *App) PlanCleanup(ids []string, cwd string) Plan {
	w, e := workdir(cwd)
	if e != nil {
		return Plan{Error: e.Error()}
	}
	lines := []string{"请停止安装、构建和相关项目。调用原生清理命令，缓存可能需联网重建；不清理项目依赖、安装目录或 Go 模块。"}
	var jobs []cleanItem
	seen := map[string]bool{}
	if len(ids) == 0 {
		for _, i := range items {
			if i.row.Cleanup != "" {
				ids = append(ids, i.row.ID)
			}
		}
	}
	if len(ids) > len(items) {
		return Plan{Error: "请求项目过多"}
	}
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		i, e := byID(id)
		if e != nil {
			continue
		}
		c, e := checkClean(i, w)
		if e != nil {
			lines = append(lines, i.row.Tool+"（跳过）："+e.Error())
			continue
		}
		lines = append(lines, i.row.Cleanup+" → "+c.Dir)
		jobs = append(jobs, c)
	}
	if len(jobs) == 0 {
		return Plan{Error: "没有可安全清理的项目。", Lines: lines}
	}
	return a.plan(request{action: "clean", cwd: w, clean: jobs}, "清理选中的缓存", lines)
}
func (a *App) ApplyCleanup(token string) Result {
	a.mu.Lock()
	defer a.mu.Unlock()
	r, e := a.take(token, "clean")
	if e != nil {
		return asResult(e, "")
	}
	var lines []string
	success := 0
	for _, j := range r.clean {
		i, _ := byID(j.ID)
		c, e := checkClean(i, r.cwd)
		if e == nil && c != j {
			e = errors.New("缓存位置或命令入口在确认前后发生变化")
		}
		if e == nil {
			_, _, e = probePath(i.row.Cleanup, r.cwd, c.Exe, 75*time.Second)
		}
		if e == nil {
			success++
			lines = append(lines, i.row.Tool+"：清理命令退出成功。")
		} else {
			lines = append(lines, i.row.Tool+"：未确认成功，"+e.Error())
		}
	}
	if success == 0 {
		return Result{Error: strings.Join(lines, "\n")}
	}
	return Result{Message: strings.Join(lines, "\n")}
}
func (a *App) PlanPackage(id, verb string) Plan {
	i, e := byID(id)
	i = a.activePackage(i)
	if e != nil || i.row.PackageID == "" {
		return Plan{Error: "此项没有已知安全的安装来源；请选该工具的版本项"}
	}
	if verb == "update" {
		verb = "upgrade"
	}
	switch verb {
	case "list", "install", "upgrade", "uninstall":
	default:
		return Plan{Error: "不支持此操作"}
	}
	exe, e := resolve("winget")
	if e != nil {
		return Plan{Error: "未找到 WinGet。需 Windows 应用安装程序；本工具不代为安装。"}
	}
	var state PackageState
	if verb != "list" {
		i, state = matchPackage(i, exe)
	}
	args := fmt.Sprintf("%s --id %s --exact --source winget --disable-interactivity", verb, i.row.PackageID)
	lines := []string{"winget " + args}
	if verb != "list" {
		if e = gatePackageAction(verb, state); e != nil {
			return Plan{Error: e.Error()}
		}
		if state.Registered {
			ver := state.Version
			if ver == "" {
				ver = "未能解析版本"
			}
			lines = append(lines, "已匹配精确包，登记版本："+ver)
		} else {
			lines = append(lines, "未匹配此精确包，不代表没有其他版本。")
		}
		if active, err := resolve(exeName(i)); err == nil {
			lines = append(lines, "当前 PATH 入口："+active+"。该入口不一定对应 WinGet 匹配，请核对。")
			if verb == "install" {
				return Plan{Error: "当前 PATH 已找到该工具，但不是已确认的 WinGet 精确包。为避免添加冲突版本，已阻止安装；请通过原安装渠道管理。", Lines: lines}
			}
		}
	}
	lines = append(lines, "精确目录匹配不证明原安装归属；更新和卸载前先核对目标。", "不自动同意软件或源协议。安装器仍可显示自己的界面或请求管理员权限。")
	return a.plan(request{action: "pkg", id: id, verb: verb, exe: exe, packageState: state, cwd: i.row.PackageID}, "运行 WinGet", lines)
}
func (a *App) ApplyPackage(token string) Result {
	a.mu.Lock()
	r, e := a.take(token, "pkg")
	if e != nil {
		a.mu.Unlock()
		return asResult(e, "")
	}
	if a.packageRunning {
		a.mu.Unlock()
		return Result{Error: "另一项 WinGet 操作正在运行"}
	}
	a.packageRunning = true
	a.mu.Unlock()
	defer func() { a.mu.Lock(); a.packageRunning = false; a.mu.Unlock() }()
	if r.target != "" {
		return a.applyMaintenanceRoute(r)
	}
	i, _ := byID(r.id)
	i = a.activePackage(i)
	exe, e := resolve("winget")
	if e != nil || exe != r.exe {
		return Result{Error: "WinGet 命令已变化，请重试"}
	}
	if r.verb != "list" {
		matched, state := matchPackage(i, exe)
		i = matched
		if i.row.PackageID != r.cwd {
			return Result{Error: "安装渠道已变化，请重新确认"}
		}
		if e = gatePackageAction(r.verb, state); e != nil {
			return asResult(e, "")
		}
		if state.Registered != r.packageState.Registered || state.Version != r.packageState.Version {
			return Result{Error: "WinGet 登记状态或版本已变化，请重新预览"}
		}
		if r.verb == "install" {
			if _, err := resolve(exeName(i)); err == nil {
				return Result{Error: "当前 PATH 找到了现有入口，已停止重复安装"}
			}
		}
	}
	args := []string{r.verb, "--id", i.row.PackageID, "--exact", "--source", "winget", "--disable-interactivity"}
	v, e := runWinget(exe, args)
	if e != nil {
		return Result{Error: e.Error(), Message: v}
	}
	if v != "" {
		v += "\n\n"
	}
	return Result{Message: v + "WinGet 正常退出（退出码 0）。请重新扫描版本与 PATH。"}
}

type Drive struct {
	Name       string `json:"name"`
	TotalBytes uint64 `json:"totalBytes"`
	FreeBytes  uint64 `json:"freeBytes"`
	UsedBytes  uint64 `json:"usedBytes"`
}
