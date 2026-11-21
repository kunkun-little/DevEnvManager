//go:build windows

package main

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

//go:embed all:frontend/dist
var assets embed.FS

func rejectNonDirectory(path string) error {
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	flags, err := syscall.GetFileAttributes(p)
	if err != nil {
		return err
	}
	if flags&0x10 == 0 || flags&0x400 != 0 {
		return fmt.Errorf("数据目录不能是文件、联接或符号链接：%s", path)
	}
	return nil
}

func portableDataDir() (string, string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", "", err
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return "", "", err
	}
	base := filepath.Dir(exe)
	data := filepath.Join(base, "DevEnvManagerData")
	web := filepath.Join(data, "WebView2")
	for _, path := range []string{data, web} {
		if err := os.Mkdir(path, 0700); err != nil && !os.IsExist(err) {
			return "", "", err
		}
		if err := rejectNonDirectory(path); err != nil {
			return "", "", err
		}
	}
	// WebView2 also reads this process-local override; keep data portable
	// rather than carrying forward a caller-supplied data-location override.
	if err := os.Setenv("WEBVIEW2_USER_DATA_FOLDER", web); err != nil {
		return "", "", err
	}
	return data, web, nil
}

func reportStartup(err error) {
	message, _ := syscall.UTF16PtrFromString("无法启动开发环境管家：\n" + strings.ReplaceAll(err.Error(), "\x00", " ") + "\n\n请完整解压到可写文件夹，并确认安装了微软 WebView2 Runtime。")
	title, _ := syscall.UTF16PtrFromString("开发环境管家")
	syscall.NewLazyDLL("user32.dll").NewProc("MessageBoxW").Call(0, uintptr(unsafe.Pointer(message)), uintptr(unsafe.Pointer(title)), 0x10)
}

func main() {
	_, web, err := portableDataDir()
	if err != nil {
		reportStartup(err)
		os.Exit(1)
	}
	app := NewApp()
	msg := windows.DefaultMessages()
	msg.Error = "开发环境管家"
	msg.ContactAdmin = "缺少或无法使用 Microsoft Edge WebView2 Runtime。请从微软官网安装或修复，再启动本工具。"
	msg.Webview2NotInstalled = "WebView2 Runtime 不可用"
	err = wails.Run(&options.App{
		Title: "开发环境管家", Width: 1136, Height: 745, MinWidth: 870, MinHeight: 615,
		BackgroundColour: options.NewRGB(242, 247, 250),
		AssetServer:      &assetserver.Options{Assets: assets},
		Bind:             []interface{}{app},
		OnStartup:        app.startup,
		Windows:          &windows.Options{Theme: windows.SystemDefault, WebviewUserDataPath: web, Messages: msg, DisablePinchZoom: true},
	})
	if err != nil {
		reportStartup(err)
		os.Exit(1)
	}
}
