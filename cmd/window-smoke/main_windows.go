//go:build windows

// window-smoke is a source-only test helper. It checks only the expected
// window and orderly close of the EXE it started, not frontend functionality.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

var (
	user32      = syscall.NewLazyDLL("user32.dll")
	enumWindows = user32.NewProc("EnumWindows")
	isVisible   = user32.NewProc("IsWindowVisible")
	windowPID   = user32.NewProc("GetWindowThreadProcessId")
	windowText  = user32.NewProc("GetWindowTextW")
	windowClass = user32.NewProc("GetClassNameW")
	postMessage = user32.NewProc("PostMessageW")
	enumerate   = syscall.NewCallback(func(hwnd, _ uintptr) uintptr {
		// The helper performs one synchronous window search; no Go pointer is passed as LPARAM.
		s := activeSearch
		if owner(hwnd) != s.pid {
			return 1
		}
		if visible, _, _ := isVisible.Call(hwnd); visible == 0 {
			return 1
		}
		// The app's missing-WebView2/startup errors use the same caption
		// as the main window, but are standard dialogs.
		var class [64]uint16
		if n, _, _ := windowClass.Call(hwnd, uintptr(unsafe.Pointer(&class[0])), uintptr(len(class))); n == 0 || syscall.UTF16ToString(class[:]) == "#32770" {
			return 1
		}
		var title [512]uint16
		if n, _, _ := windowText.Call(hwnd, uintptr(unsafe.Pointer(&title[0])), uintptr(len(title))); n != 0 {
			got := syscall.UTF16ToString(title[:])
			if (s.exact && got == s.title) || (!s.exact && strings.Contains(got, s.title)) {
				s.hwnd = hwnd
				s.observed = got
				return 0
			}
		}
		return 1
	})
)

var activeSearch *search

type search struct {
	pid      uint32
	title    string
	exact    bool
	hwnd     uintptr
	observed string
}

func owner(hwnd uintptr) uint32 {
	var pid uint32
	if thread, _, _ := windowPID.Call(hwnd, uintptr(unsafe.Pointer(&pid))); thread == 0 {
		return 0
	}
	return pid
}

func smoke(exe, title string, exact bool, timeout time.Duration) error {
	if exe == "" || title == "" || timeout <= 0 {
		return errors.New("-exe, -title and a positive -timeout are required")
	}
	abs, err := filepath.Abs(exe)
	if err != nil {
		return err
	}
	if !strings.EqualFold(filepath.Ext(abs), ".exe") {
		return errors.New("test input must be an .exe file")
	}
	cmd := exec.Command(abs)
	cmd.Dir = filepath.Dir(abs)
	if err = cmd.Start(); err != nil {
		return err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	exited := false
	defer func() {
		if !exited {
			// Process.Kill targets the original Windows process handle; never kill by name.
			_ = cmd.Process.Kill()
			select {
			case <-done:
			case <-time.After(3 * time.Second):
			}
		}
	}()
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	poll := time.NewTicker(100 * time.Millisecond)
	defer poll.Stop()
	s := search{pid: uint32(cmd.Process.Pid), title: title, exact: exact}
	activeSearch = &s
	defer func() { activeSearch = nil }()
	for s.hwnd == 0 {
		select {
		case err = <-done:
			exited = true
			return fmt.Errorf("process exited before the expected window: %v", err)
		case <-deadline.C:
			return fmt.Errorf("no visible owned window matching %q within %s", title, timeout)
		case <-poll.C:
			enumWindows.Call(enumerate, 0)
		}
	}
	fmt.Printf("PASS: visible main window %q belongs to PID %d\n", s.observed, s.pid)
	// Repeat process-ownership check immediately before posting the benign close.
	select {
	case err = <-done:
		exited = true
		return fmt.Errorf("process exited before graceful close was requested: %v", err)
	default:
	}
	if owner(s.hwnd) != s.pid {
		return errors.New("main window disappeared or changed owner before close")
	}
	if ok, _, e := postMessage.Call(s.hwnd, 0x0010, 0, 0); ok == 0 { // WM_CLOSE
		return fmt.Errorf("posting WM_CLOSE failed: %v", e)
	}
	closeDeadline := time.NewTimer(timeout)
	defer closeDeadline.Stop()
	select {
	case err = <-done:
		exited = true
		if err != nil {
			return fmt.Errorf("process did not exit successfully: %w", err)
		}
		fmt.Println("PASS: the same EXE exited with code 0 after WM_CLOSE")
		return nil
	case <-closeDeadline.C:
		return fmt.Errorf("EXE did not exit after WM_CLOSE within %s", timeout)
	}
}

func main() {
	exe := flag.String("exe", "", "path to the Windows GUI executable")
	title := flag.String("title", "开发环境管家", "expected main window title or title substring")
	exact := flag.Bool("exact", false, "require the complete title to match")
	timeout := flag.Duration("timeout", 30*time.Second, "timeout for each of startup and exit")
	flag.Parse()
	if err := smoke(*exe, *title, *exact, *timeout); err != nil {
		fmt.Fprintln(os.Stderr, "FAIL:", err)
		os.Exit(1)
	}
}
