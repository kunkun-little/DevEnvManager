package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var pythonBranch = regexp.MustCompile(`^Python (3\.[0-9]+)\.[0-9]+(?:\s|$)`)

func (a *App) activePackage(i item) item {
	if i.row.ID == "python-version" {
		if wd, e := workdir(""); e == nil {
			v, _, _ := probe("python --version", wd, 6*time.Second)
			if v == "" {
				version := registeredVersion(i.row.ID)
				if version == "" {
					if _, err := resolve("python"); err != nil {
						version = a.LatestVersion(i.row.ID).Version
					}
				}
				v = "Python " + version
			}
			if match := pythonBranch.FindStringSubmatch(v); match != nil {
				i.row.PackageID = "Python.Python." + match[1]
			} else {
				i.row.PackageID = ""
			}
		}
	}
	return i
}

func npmGlobalCommand(i item, verb string) (exe, cmd string, ok bool) {
	if i.row.ID != "pnpm-version" && i.row.ID != "npm-version" {
		return
	}
	if verb != "upgrade" && verb != "uninstall" {
		return
	}
	name := i.row.Tool
	wd, e := workdir("")
	if e != nil {
		return
	}
	prefix, _, e := probe("npm prefix --global", wd, 6*time.Second)
	if e != nil || !filepath.IsAbs(prefix) {
		return
	}
	entry, e := resolve(name)
	if e != nil || !strings.EqualFold(filepath.Dir(entry), filepath.Clean(prefix)) || !strings.EqualFold(filepath.Ext(entry), ".cmd") {
		return
	}
	// Distinguish npm-installed shims from Corepack and bundled distributions.
	if node, e := resolve("node"); name == "npm" && e == nil && strings.EqualFold(filepath.Dir(node), filepath.Dir(entry)) {
		return
	}
	b, e := os.ReadFile(entry)
	if e != nil || len(b) > 32768 {
		return
	}
	marker := "node_modules\\" + name + "\\bin\\"
	if !strings.Contains(strings.ToLower(strings.ReplaceAll(string(b), "/", "\\")), marker) {
		return
	}
	if _, e = os.Stat(filepath.Join(prefix, "node_modules", name, "package.json")); e != nil {
		return
	}
	exe, e = resolve("npm")
	if e != nil {
		return
	}
	if verb == "upgrade" {
		cmd = "npm install --global " + name + "@latest"
	} else {
		cmd = "npm uninstall --global " + name
	}
	return exe, cmd, true
}

var updatePages = map[string]string{
	"node-version":   "https://nodejs.org/en/download/",
	"python-version": "https://www.python.org/downloads/windows/",
	"go-version":     "https://go.dev/doc/install",
	"npm-version":    "https://docs.npmjs.com/downloading-and-installing-node-js-and-npm",
	"pnpm-version":   "https://pnpm.io/installation",
}

func (a *App) PlanMaintenance(id, verb string) Plan {
	i, e := byID(id)
	if e != nil || updatePages[id] == "" {
		return Plan{Error: "不支持此软件包"}
	}
	if verb != "upgrade" && verb != "uninstall" && verb != "install" {
		return Plan{Error: "不支持此操作"}
	}
	if exe, cmd, ok := npmGlobalCommand(i, verb); ok {
		entry, _ := resolve(exeName(i))
		return a.plan(request{action: "pkg", id: id, verb: verb, exe: exe, target: cmd, cwd: entry}, "通过 npm 管理全局安装", []string{entry, cmd})
	}
	if i.row.PackageID != "" {
		p := a.PlanPackage(id, verb)
		if p.Error == "" {
			return p
		}
		if verb == "install" {
			if _, err := resolve(exeName(i)); err == nil {
				return p
			}
		}
		return a.channelPlan(i, verb, p.Error)
	}
	return a.channelPlan(i, verb, "npm 的维护方式取决于 Node.js 安装渠道。")
}
func (a *App) channelPlan(i item, verb, reason string) Plan {
	target := updatePages[i.row.ID]
	summary := "打开官方安装与更新说明"
	if verb == "uninstall" {
		target = "ms-settings:appsfeatures"
		summary = "打开 Windows 已安装的应用"
	}
	lines := []string{reason}
	if entry, e := resolve(exeName(i)); e == nil {
		lines = append(lines, "本机入口："+entry)
	}
	if i.row.ID == "npm-version" {
		lines = append(lines, "Node.js 附带的 npm 随 Node.js 维护。卸载整个 Node.js 也会影响依赖它的项目。")
	}
	if i.row.ID == "pnpm-version" && verb == "uninstall" {
		target = "https://pnpm.io/uninstall"
		summary = "打开 pnpm 官方卸载说明"
	}
	lines = append(lines, summary+"。请在对应渠道核对并操作。")
	return a.plan(request{action: "pkg", id: i.row.ID, verb: verb, target: target}, summary, lines)
}
func (a *App) applyMaintenanceRoute(r request) Result {
	i, _ := byID(r.id)
	if r.exe == "" {
		return asResult(a.openMaintenancePage(r.target), "已打开维护入口")
	}
	exe, cmd, ok := npmGlobalCommand(i, r.verb)
	entry, _ := resolve(exeName(i))
	if !ok || exe != r.exe || cmd != r.target || entry != r.cwd {
		return asResult(errors.New("全局包来源已变化，请重新确认"), "")
	}
	wd, e := workdir("")
	if e != nil {
		return asResult(e, "")
	}
	_, _, e = probePath(cmd, wd, exe, 5*time.Minute)
	return asResult(e, fmt.Sprintf("%s 操作已完成", i.row.Tool))
}
