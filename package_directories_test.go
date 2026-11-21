package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestPnpmDirectoryVariables(t *testing.T) {
	i, _ := byID("pnpm-store")
	for _, tc := range []struct{ version, key string }{{"10.22.0", "NPM_CONFIG_STORE_DIR"}, {"11.0.0", "PNPM_CONFIG_STORE_DIR"}, {"12.1.0", "PNPM_CONFIG_STORE_DIR"}} {
		configured, e := pnpmDirectoryItem(i, tc.version)
		if e != nil || configured.row.Variable != tc.key {
			t.Fatal(configured, e)
		}
		restored, e := journalItem(i.row.ID, tc.key)
		if e != nil || restored.row.Variable != tc.key {
			t.Fatal(restored, e)
		}
	}
	if _, e := journalItem(i.row.ID, "PATH"); e == nil {
		t.Fatal("unrelated variable allowed")
	}
	if directoryCleaner(i).row.Cleanup != "" {
		t.Fatal("store prune cannot discard a whole store")
	}
	base := t.TempDir()
	v, e := pnpmStoreRoot(filepath.Join(base, "v10"))
	if e != nil || v != base {
		t.Fatal(v, e)
	}
	if _, e = pnpmStoreRoot(filepath.Join(base, "custom")); e == nil {
		t.Fatal("inferred unrecognised layout")
	}
}

// Opt-in smoke test for the Windows npm and pnpm 10 CLIs. The files and
// effective settings are real fixtures; registry writes use only this map.
func TestInstalledPackageDirectoryFlow(t *testing.T) {
	if runtime.GOOS != "windows" || os.Getenv("DEVENVMANAGER_TEST_PACKAGE_CLI") != "1" {
		t.Skip("requires opt-in Windows npm and pnpm 10 CLIs")
	}
	root := t.TempDir()
	for k, dir := range map[string]string{"APPDATA": "roaming", "LOCALAPPDATA": "local", "HOME": "home", "USERPROFILE": "home", "XDG_CONFIG_HOME": "config", "PNPM_HOME": "pnpm-home", "COREPACK_HOME": "corepack"} {
		p := filepath.Join(root, dir)
		if e := os.MkdirAll(p, 0700); e != nil {
			t.Fatal(e)
		}
		t.Setenv(k, p)
	}
	wd := os.Getenv("USERPROFILE")
	rc := filepath.Join(wd, ".npmrc")
	t.Setenv("NPM_CONFIG_USERCONFIG", rc)
	t.Setenv("NPM_CONFIG_GLOBALCONFIG", filepath.Join(root, "unused-rc"))
	npmPrefix := filepath.Join(root, "npm-old")
	npmText := "prefix=" + filepath.ToSlash(npmPrefix) + "\n"
	if e := os.WriteFile(rc, []byte(npmText), 0600); e != nil {
		t.Fatal(e)
	}
	ids := []string{"pnpm-store", "pnpm-cache", "pnpm-state", "pnpm-global"}
	configDir := filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "pnpm")
	if e := os.MkdirAll(configDir, 0700); e != nil {
		t.Fatal(e)
	}
	var lines []string
	for _, id := range ids {
		name := strings.TrimPrefix(id, "pnpm-")
		dir := filepath.Join(root, "old", name)
		if e := os.MkdirAll(dir, 0700); e != nil {
			t.Fatal(e)
		}
		lines = append(lines, name+"-dir="+filepath.ToSlash(dir))
		if name == "store" {
			p := filepath.Join(dir, "v10")
			if e := os.Mkdir(p, 0700); e != nil {
				t.Fatal(e)
			}
			if e := os.WriteFile(filepath.Join(p, "fixture"), []byte("data"), 0600); e != nil {
				t.Fatal(e)
			}
		}
	}
	rcPath := filepath.Join(configDir, "rc")
	pnpmText := strings.Join(lines, "\n") + "\n"
	if e := os.WriteFile(rcPath, []byte(pnpmText), 0600); e != nil {
		t.Fatal(e)
	}
	oldCache := filepath.Join(root, "npm-old-cache")
	t.Setenv("NPM_CONFIG_CACHE", oldCache)
	initial := map[string]prior{"NPM_CONFIG_CACHE": {Value: oldCache, Exists: true, Kind: 1}}
	state := map[string]prior{"NPM_CONFIG_CACHE": initial["NPM_CONFIG_CACHE"]}
	app := NewApp()
	app.registry = configRegistry{read: func(_ bool, k string) (prior, error) { return state[k], nil }, write: func(k string, v prior) error { state[k] = v; return nil }, notify: func() {}}
	for _, tc := range []struct {
		ids   []string
		batch bool
		mode  string
	}{{[]string{"npm-prefix"}, false, "keep"}, {[]string{"npm-cache"}, false, "keep"}, {ids, true, "migrate"}} {
		dst := filepath.Join(root, fmt.Sprintf("目标 %s", tc.ids[0]))
		p := app.PlanDirectories(dst, tc.ids, tc.mode, tc.batch)
		if p.Error != "" || len(p.Directories) != len(tc.ids) {
			t.Fatalf("plan %+v", p)
		}
		if _, e := os.Stat(dst); !os.IsNotExist(e) {
			t.Fatalf("planning touched destination: %v", e)
		}
		if r := app.ApplyConfig(p.Token); r.Error != "" {
			t.Fatal(r.Error)
		}
		for _, d := range p.Directories {
			if state[d.Variable].Value != d.Target {
				t.Fatal(d, state)
			}
		}
		if tc.batch {
			actual, e := os.ReadFile(filepath.Join(dst, "store", "v10", "fixture"))
			if e != nil || string(actual) != "data" {
				t.Fatal("wrong store layout", e)
			}
			exe, e := resolve("pnpm")
			if e != nil {
				t.Fatal(e)
			}
			effective, _, e := probeWithEnv("pnpm store path", wd, exe, 8*time.Second, []string{"NPM_CONFIG_STORE_DIR=" + filepath.Join(dst, "store")})
			if e != nil || !strings.EqualFold(filepath.Clean(effective), filepath.Join(dst, "store", "v10")) {
				t.Fatal(effective, e)
			}
			if r := app.RollbackGoRoot(); r.Error != "" {
				t.Fatal(r.Error)
			}
		} else if r := app.Rollback(tc.ids[0]); r.Error != "" {
			t.Fatal(r.Error)
		}
		for _, d := range p.Directories {
			if state[d.Variable] != initial[d.Variable] {
				t.Fatal("rollback left override", d)
			}
		}
	}
	for path, want := range map[string]string{rc: npmText, rcPath: pnpmText} {
		b, e := os.ReadFile(path)
		if e != nil || string(b) != want {
			t.Fatal("fixture config changed", path, e)
		}
	}
}
