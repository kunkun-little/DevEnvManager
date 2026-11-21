package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestDirectoryPolicies(t *testing.T) {
	i, _ := byID("go-path")
	root := t.TempDir()
	dst := filepath.Join(root, "existing")
	src := filepath.Join(root, "workspace")
	if e := os.Mkdir(dst, 0700); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(filepath.Join(dst, "main.go"), []byte("data"), 0600); e != nil {
		t.Fatal(e)
	}
	if e := checkTransfer(i, src, dst, "keep", root); e != nil {
		t.Fatal(e)
	}
	if e := checkTransfer(i, src, dst, "migrate", root); e == nil {
		t.Fatal("overwriting a populated directory")
	}
	if e := checkTransfer(i, src, filepath.Join(src, "other"), "keep", root); e == nil {
		t.Fatal("nested target")
	}
	if e := checkTransfer(i, src, filepath.Join(root, "fresh"), "delete", root); e == nil {
		t.Fatal("offered to delete workspace")
	}
	if i.row.Variable != "GOPATH" {
		t.Fatal(i)
	}
	i, _ = byID("go-bin")
	v, e := sourcePath(i, root, map[string]string{"GOPATH": src})
	if e != nil || v != filepath.Join(src, "bin") {
		t.Fatal(v, e)
	}
}
func TestCopyDirectoryData(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("uses Windows Robocopy")
	}
	dir := t.TempDir()
	src := filepath.Join(dir, "旧 工作区")
	dst := filepath.Join(dir, "新 工作区")
	for _, p := range []string{filepath.Join(src, "empty"), dst} {
		if e := os.MkdirAll(p, 0700); e != nil {
			t.Fatal(e)
		}
	}
	if e := os.WriteFile(filepath.Join(src, "project.txt"), []byte("fixture"), 0600); e != nil {
		t.Fatal(e)
	}
	if e := copyDirectory(src, dst); e != nil {
		t.Fatal(e)
	}
	for _, dir := range []string{src, dst} {
		if st, e := os.Stat(filepath.Join(dir, "empty")); e != nil || !st.IsDir() {
			t.Fatal(e)
		}
		b, e := os.ReadFile(filepath.Join(dir, "project.txt"))
		if e != nil || string(b) != "fixture" {
			t.Fatal(string(b), e)
		}
	}
	if e := copyDirectory(src, dst); e == nil {
		t.Fatal("published over nonempty target")
	}
	files, e := os.ReadDir(dir)
	if e != nil {
		t.Fatal(e)
	}
	for _, f := range files {
		if strings.HasPrefix(f.Name(), ".devenv-copy-") {
			t.Fatal("staging directory leaked")
		}
	}
}
func TestReleaseChannels(t *testing.T) {
	for _, tc := range []struct{ id, data, want string }{
		{"go-version", `[{"ref":"refs/tags/go1.25rc1"},{"ref":"refs/tags/go1.24.10"},{"ref":"refs/tags/go1.24.9"}]`, "1.24.10"},
		{"node-version", `[{"version":"v25.0.0","lts":false},{"version":"v24.1.0","lts":"Krypton"}]`, "25.0.0"},
		{"python-version", `[{"name":"Python 3.15.1","is_published":true},{"name":"Python 3.16.0a1","is_published":true,"pre_release":true},{"name":"Python 3.14.12","is_published":true}]`, "3.15.1"},
		{"npm-version", `{"version":"12.1.0"}`, "12.1.0"},
		{"pnpm-version", `{"version":"11.0.1"}`, "11.0.1"},
	} {
		v, e := parseRelease(tc.id, []byte(tc.data))
		if e != nil || v != tc.want {
			t.Fatalf("%s: %s %v", tc.id, v, e)
		}
	}
	if _, e := parseRelease("npm-version", []byte("{}")); e == nil {
		t.Fatal("missing version")
	}
}

// Run the normal desktop entry points with a private PATH, real fixture files
// and an in-memory user registry. No installed interpreter is invoked.
func TestConfirmedDirectoryChange(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows paths")
	}
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	source := filepath.Join(root, "source")
	dest := filepath.Join(root, "target")
	for _, dir := range []string{bin, source} {
		if e := os.Mkdir(dir, 0700); e != nil {
			t.Fatal(e)
		}
	}
	if e := os.WriteFile(filepath.Join(bin, "python.exe"), nil, 0600); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(filepath.Join(source, "sample.pyc"), []byte("fixture"), 0600); e != nil {
		t.Fatal(e)
	}
	t.Setenv("PATH", bin)
	const key = "PYTHONPYCACHEPREFIX"
	t.Setenv(key, source)
	a := NewApp()
	old := prior{Value: source, Exists: true, Kind: 1}
	user := map[string]prior{key: old}
	a.registry = configRegistry{
		read: func(machine bool, k string) (prior, error) {
			if machine {
				return prior{}, nil
			}
			return user[k], nil
		},
		write:  func(k string, p prior) error { user[k] = p; return nil },
		notify: func() {},
	}
	p1 := a.PlanDirectories(dest, []string{"python-pycache"}, "migrate", false)
	if p1.Error != "" || len(p1.Directories) != 1 || p1.Directories[0].Target != dest {
		t.Fatalf("plan: %+v", p1)
	}
	if _, e := os.Stat(dest); !os.IsNotExist(e) || user[key] != old {
		t.Fatal("preview had side effects")
	}
	if result := a.ApplyConfig(p1.Token); result.Error != "" {
		t.Fatal(result.Error)
	}
	if user[key].Value != dest {
		t.Fatal(user)
	}
	if b, e := os.ReadFile(filepath.Join(dest, "sample.pyc")); e != nil || string(b) != "fixture" {
		t.Fatal(e)
	}
	if r := a.ApplyConfig(p1.Token); r.Error == "" {
		t.Fatal("replayed plan")
	}
	if r := a.Rollback("python-pycache"); r.Error != "" || user[key] != old {
		t.Fatal(r, user)
	}
}
