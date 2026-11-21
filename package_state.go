package main

import (
	"errors"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

type PackageState struct {
	Available  bool   `json:"available"`
	Registered bool   `json:"registered"`
	Version    string `json:"version"`
	Evidence   string `json:"evidence"`
	Error      string `json:"error"`
}

var packageVersion = regexp.MustCompile(`[0-9]+\.[0-9]+\.[0-9]+`)
var wingetVersionText = regexp.MustCompile(`^v?[0-9][0-9a-zA-Z._+-]*$`)

func parseExactWinGetEntry(output, id string) (bool, string) {
	for _, line := range strings.Split(output, "\n") {
		tokens := strings.Fields(line)
		for k, t := range tokens {
			if !strings.EqualFold(t, id) {
				continue
			}
			if k == 0 || k+1 >= len(tokens) {
				continue
			}
			version := tokens[k+1]
			if !wingetVersionText.MatchString(version) {
				version = ""
			}
			return true, version
		}
	}
	return false, ""
}

// DetectPackage only checks registration under a known exact WinGet catalog ID;
// WinGet can recognize an installation from another source. It never proves ownership.
func (a *App) DetectPackage(id string) PackageState {
	i, e := byID(id)
	if e != nil || i.row.PackageID == "" {
		return PackageState{Error: "此项没有受支持的 WinGet 包"}
	}
	exe, e := resolve("winget")
	if e != nil {
		return PackageState{Error: "未发现 WinGet"}
	}
	_, s := matchPackage(a.activePackage(i), exe)
	return s
}

func detectPackageWithExe(i item, exe string) PackageState {
	args := []string{"list", "--id", i.row.PackageID, "--exact", "--source", "winget", "--disable-interactivity"}
	o, e := runWinget(exe, args)
	s := PackageState{Available: true}
	if e != nil {
		// https://github.com/microsoft/winget-cli/blob/master/doc/windows/package-manager/winget/returnCodes.md
		var exit *exec.ExitError
		if errors.As(e, &exit) && uint32(exit.ExitCode()) == 0x8A150014 {
			s.Evidence = "此精确 WinGet 包未匹配；不代表没有其他版本或来源的安装。"
		} else {
			s.Error = e.Error()
		}
		return s
	}
	s.Registered, s.Version = parseExactWinGetEntry(o, i.row.PackageID)
	if !s.Registered {
		s.Error = "WinGet 已退出，但无法从输出确认精确包匹配"
		return s
	}
	s.Evidence = "精确包 ID 匹配不代表最初由 WinGet 安装，也不等于 PATH 命中的版本。"
	return s
}

func gatePackageAction(verb string, state PackageState) error {
	if verb == "list" {
		return nil
	}
	if !state.Available || state.Error != "" {
		if state.Error != "" {
			return errors.New("无法确认目标包的注册状态：" + state.Error)
		}
		return errors.New("无法确认目标包的注册状态")
	}
	if verb == "install" && state.Registered {
		return errors.New("该精确包已登记；请选更新，不重复安装")
	}
	if (verb == "upgrade" || verb == "uninstall") && !state.Registered {
		return errors.New("未匹配到该精确包，不执行更新或卸载")
	}
	return nil
}

func matchPackage(i item, exe string) (item, PackageState) {
	s := detectPackageWithExe(i, exe)
	active := ""
	if s.Registered && i.row.Command != "" {
		if wd, e := workdir(""); e == nil {
			v, _, _ := probe(i.row.Command, wd, 8*time.Second)
			active = packageVersion.FindString(v)
		}
	}
	mismatch := s.Registered && active != "" && active != s.Version
	if i.row.ID == "node-version" && (!s.Registered || mismatch) && s.Error == "" {
		other := i
		other.row.PackageID = "OpenJS.NodeJS"
		alt := detectPackageWithExe(other, exe)
		if alt.Registered && (active == "" || alt.Version == active) {
			return other, alt
		}
	}
	if mismatch {
		s.Error = "WinGet 登记版本与当前运行版本不同，请通过当前安装渠道维护"
	}
	return i, s
}
