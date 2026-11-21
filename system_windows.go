package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

var (
	kernel             = syscall.NewLazyDLL("kernel32.dll")
	getSystemDirectory = kernel.NewProc("GetSystemDirectoryW")
	getDriveType       = kernel.NewProc("GetDriveTypeW")
	getLogicalDrives   = kernel.NewProc("GetLogicalDrives")
	getDiskFreeSpaceEx = kernel.NewProc("GetDiskFreeSpaceExW")
	advapi             = syscall.NewLazyDLL("advapi32.dll")
	regSetProc         = advapi.NewProc("RegSetValueExW")
	regDeleteProc      = advapi.NewProc("RegDeleteValueW")
	regCreateProc      = advapi.NewProc("RegCreateKeyExW")
	broadcastEnv       = syscall.NewLazyDLL("user32.dll").NewProc("SendMessageTimeoutW")
)

func reparse(st fs.FileInfo) bool {
	if st.Mode()&os.ModeSymlink != 0 {
		return true
	}
	d, ok := st.Sys().(*syscall.Win32FileAttributeData)
	return ok && d.FileAttributes&syscall.FILE_ATTRIBUTE_REPARSE_POINT != 0
}
func fixedDrive(p string) bool {
	vol := filepath.VolumeName(p)
	if len(vol) != 2 {
		return false
	}
	q, e := syscall.UTF16PtrFromString(vol + "\\")
	if e != nil {
		return false
	}
	x, _, _ := getDriveType.Call(uintptr(unsafe.Pointer(q)))
	return x == 3
}
func (a *App) DriveStats() []Drive {
	out := []Drive{}
	mask, _, _ := getLogicalDrives.Call()
	for i := 0; i < 26; i++ {
		if mask&(1<<i) == 0 {
			continue
		}
		root := string(rune('A'+i)) + ":\\"
		if !fixedDrive(root) {
			continue
		}
		p, _ := syscall.UTF16PtrFromString(root)
		var avail, total, free uint64
		v, _, _ := getDiskFreeSpaceEx.Call(uintptr(unsafe.Pointer(p)), uintptr(unsafe.Pointer(&avail)), uintptr(unsafe.Pointer(&total)), uintptr(unsafe.Pointer(&free)))
		if v != 0 && free <= total {
			out = append(out, Drive{root[:2], total, free, total - free})
		}
	}
	return out
}
func regValue(machine bool, key string) (prior, error) {
	var h syscall.Handle
	root := syscall.HKEY_CURRENT_USER
	k := "Environment"
	if machine {
		root = syscall.HKEY_LOCAL_MACHINE
		k = `SYSTEM\CurrentControlSet\Control\Session Manager\Environment`
	}
	path, _ := syscall.UTF16PtrFromString(k)
	if e := syscall.RegOpenKeyEx(syscall.Handle(root), path, 0, syscall.KEY_QUERY_VALUE, &h); e != nil {
		if errors.Is(e, syscall.ERROR_FILE_NOT_FOUND) {
			return prior{}, nil
		}
		return prior{}, e
	}
	defer syscall.RegCloseKey(h)
	q, _ := syscall.UTF16PtrFromString(key)
	var kind, n uint32
	if e := syscall.RegQueryValueEx(h, q, nil, &kind, nil, &n); e != nil {
		if errors.Is(e, syscall.ERROR_FILE_NOT_FOUND) {
			return prior{}, nil
		}
		return prior{}, e
	}
	if kind != 1 && kind != 2 {
		return prior{Exists: true, Kind: kind}, nil
	}
	if n > 65536 || n%2 != 0 {
		return prior{}, errors.New("环境变量长度异常")
	}
	buf := make([]uint16, int(n/2)+1)
	if e := syscall.RegQueryValueEx(h, q, nil, &kind, (*byte)(unsafe.Pointer(&buf[0])), &n); e != nil {
		return prior{}, e
	}
	return prior{Value: syscall.UTF16ToString(buf), Exists: true, Kind: kind}, nil
}
func regSetRaw(key string, v prior) error {
	path, _ := syscall.UTF16PtrFromString("Environment")
	q, _ := syscall.UTF16PtrFromString(key)
	var h syscall.Handle
	res, _, _ := regCreateProc.Call(uintptr(syscall.HKEY_CURRENT_USER), uintptr(unsafe.Pointer(path)), 0, 0, 0, uintptr(syscall.KEY_SET_VALUE), 0, uintptr(unsafe.Pointer(&h)), 0)
	if res != 0 {
		return syscall.Errno(res)
	}
	defer syscall.RegCloseKey(h)
	if !v.Exists {
		res, _, _ = regDeleteProc.Call(uintptr(h), uintptr(unsafe.Pointer(q)))
	} else {
		buf, e := syscall.UTF16FromString(v.Value)
		if e != nil {
			return e
		}
		res, _, _ = regSetProc.Call(uintptr(h), uintptr(unsafe.Pointer(q)), 0, uintptr(v.Kind), uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)*2))
	}
	if res != 0 {
		return syscall.Errno(res)
	}
	return nil
}
func regSet(key string, v prior) error {
	if e := regSetRaw(key, v); e != nil {
		return e
	}
	notifyEnv()
	return nil
}
func notifyEnv() {

	txt, _ := syscall.UTF16PtrFromString("Environment")
	var result uintptr
	broadcastEnv.Call(0xffff, 0x001a, 0, uintptr(unsafe.Pointer(txt)), 0x0002, 500, uintptr(unsafe.Pointer(&result)))
}
func resolve(name string) (string, error) {
	p, e := exec.LookPath(name)
	if e != nil || !filepath.IsAbs(p) {
		return "", fmt.Errorf("当前 PATH 中未找到可信的 %s 入口", name)
	}
	ext := strings.ToLower(filepath.Ext(p))
	if ext != ".exe" && !(ext == ".cmd" && (name == "npm" || name == "pnpm")) {
		return "", errors.New("不支持此命令入口类型")
	}
	if name == "python" && strings.HasSuffix(strings.ToLower(filepath.Dir(p)), `\microsoft\windowsapps`) {
		return "", errors.New("PATH 命中 Microsoft Store Python 别名，请先使用实际解释器")
	}
	return filepath.Clean(p), nil
}
func cmdExe() (string, error) {
	var buf [1024]uint16
	n, _, e := getSystemDirectory.Call(uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	if n == 0 || n >= uintptr(len(buf)) {
		return "", e
	}
	return filepath.Join(syscall.UTF16ToString(buf[:]), "cmd.exe"), nil
}
func safeEnv(w string) []string {
	strip := map[string]bool{}
	for _, k := range []string{"NODE_OPTIONS", "NODE_COMPILE_CACHE", "NODE_V8_COVERAGE", "NODE_REDIRECT_WARNINGS", "PYTHONPATH", "PYTHONSTARTUP", "PYTHONPYCACHEPREFIX", "PYTHONDONTWRITEBYTECODE", "PIP_LOG", "PIP_LOG_FILE", "PIP_DISABLE_PIP_VERSION_CHECK", "GOTOOLCHAIN", "COREPACK_ENABLE_NETWORK", "COREPACK_DEFAULT_TO_LATEST", "COREPACK_ENABLE_AUTO_PIN", "NPM_CONFIG_MANAGE_PACKAGE_MANAGER_VERSIONS", "PNPM_CONFIG_MANAGE_PACKAGE_MANAGER_VERSIONS", "NO_COLOR", "NO_DEFAULT_CURRENT_DIRECTORY_IN_EXE_PATH", "COREPACK_ENV_FILE"} {
		strip[k] = true
	}
	var out []string
	for _, e := range os.Environ() {
		k, _, _ := strings.Cut(e, "=")
		if !strip[strings.ToUpper(k)] {
			out = append(out, e)
		}
	}
	return append(out, "PYTHONDONTWRITEBYTECODE=1", "PIP_DISABLE_PIP_VERSION_CHECK=1", "GOTOOLCHAIN=local", "COREPACK_ENABLE_NETWORK=0", "COREPACK_DEFAULT_TO_LATEST=0", "COREPACK_ENABLE_AUTO_PIN=0", "COREPACK_ENV_FILE=0", "npm_config_manage_package_manager_versions=false", "PNPM_CONFIG_MANAGE_PACKAGE_MANAGER_VERSIONS=false", "NO_COLOR=1", "NoDefaultCurrentDirectoryInExePath=1")
}

type bounded struct{ b bytes.Buffer }

func (b *bounded) Write(p []byte) (int, error) {
	n := len(p)
	if m := 65536 - b.b.Len(); m > 0 {
		if len(p) > m {
			p = p[:m]
		}
		_, _ = b.b.Write(p)
	}
	return n, nil
}
func probe(command, w string, t time.Duration) (string, string, error) {
	args := strings.Fields(command)
	if len(args) < 2 {
		return "", "", errors.New("无效探测命令")
	}
	path, e := resolve(args[0])
	if e != nil {
		return "", "", e
	}
	return probePath(command, w, path, t)
}
func probePath(command, w, path string, t time.Duration) (string, string, error) {
	args := strings.Fields(command)
	if len(args) < 2 {
		return "", path, errors.New("无效命令")
	}
	var e error
	ctx, cancel := context.WithTimeout(context.Background(), t)
	defer cancel()
	c := exec.CommandContext(ctx, path, args[1:]...)
	c.Dir = w
	c.Env = safeEnv(w)
	c.WaitDelay = 1500 * time.Millisecond
	c.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	if strings.EqualFold(filepath.Ext(path), ".cmd") {
		if strings.ContainsAny(path, "\"%&!^|<>()\r\n") {
			return "", path, errors.New("CMD shim 路径含不能安全执行的字符")
		}
		// Windows npm/pnpm wrappers can prefer cwd for resolving node.exe. Never run outside trusted paths.
		if _, e := os.Lstat(filepath.Join(w, "node.exe")); e == nil || !os.IsNotExist(e) {
			return "", path, errors.New("工作目录存在 node.exe，或无法验证它不存在")
		}
		sys, e := cmdExe()
		if e != nil {
			return "", path, e
		}
		c.Path = sys
		c.SysProcAttr.CmdLine = `"` + sys + `" /d /s /v:off /c ""` + path + `" ` + strings.Join(args[1:], " ") + `"`
	}
	var output bounded
	c.Stdout = &output
	e = c.Run()
	if ctx.Err() != nil {
		return "", path, errors.New("命令超时；子进程可能仍在运行")
	}
	if e != nil {
		return "", path, fmt.Errorf("命令没有成功（%w）；请核查可信终端中的命令和版本", e)
	}
	if output.b.Len() >= 65536 {
		return "", path, errors.New("命令输出超限")
	}
	return strings.TrimSpace(output.b.String()), path, nil
}
func runWinget(exe string, args []string) (string, error) {
	w, e := workdir("")
	if e != nil {
		return "", e
	}
	// The read-only list can time out; do not kill a running installer at an arbitrary deadline.
	var c *exec.Cmd
	if len(args) > 0 && args[0] == "list" {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		c = exec.CommandContext(ctx, exe, args...)
	} else {
		c = exec.Command(exe, args...)
	}
	c.Dir = w
	c.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	c.WaitDelay = 2 * time.Second
	var output bounded
	c.Stdout = &output
	c.Stderr = &output
	e = c.Run()
	v := strings.TrimSpace(output.b.String())
	if e != nil {
		return v, fmt.Errorf("WinGet 未确认成功（%w）。本工具不会代为同意协议或提升权限。", e)
	}
	return v, nil
}
func (a *App) ChooseDirectory() (string, error) {
	a.ctxMu.RLock()
	ctx := a.ctx
	a.ctxMu.RUnlock()
	if ctx == nil {
		return "", errors.New("窗口尚未准备好")
	}
	return wailsruntime.OpenDirectoryDialog(ctx, wailsruntime.OpenDialogOptions{Title: "选择目标目录"})
}

var createConfigMutex = kernel.NewProc("CreateMutexW")
var releaseConfigMutex = kernel.NewProc("ReleaseMutex")

func holdConfigLock() (func(), error) {
	name, _ := syscall.UTF16PtrFromString(`Local\DevEnvManager.Config.v1`)
	goruntime.LockOSThread()
	ptr, _, e := createConfigMutex.Call(0, 0, uintptr(unsafe.Pointer(name)))
	if ptr == 0 {
		goruntime.UnlockOSThread()
		return nil, e
	}
	h := syscall.Handle(ptr)
	status, e := syscall.WaitForSingleObject(h, 2500)
	if e != nil || (status != 0 && status != 0x80) {
		_ = syscall.CloseHandle(h)
		goruntime.UnlockOSThread()
		return nil, errors.New("另一窗口正在写入用户配置，请稍后重试")
	}
	return func() { releaseConfigMutex.Call(ptr); _ = syscall.CloseHandle(h); goruntime.UnlockOSThread() }, nil
}
