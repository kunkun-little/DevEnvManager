package main

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestGoBatchSelection(t *testing.T) {
	got, e := expandGoSelection([]string{"go-cache", "go-temp"})
	if e != nil || !reflect.DeepEqual(got, []string{"go-temp", "go-cache"}) {
		t.Fatalf("%v: %v", got, e)
	}
	for _, ids := range [][]string{nil, {"go-temp", "go-temp"}, {"go-path"}, {"../../go-temp"}} {
		if _, e := expandGoSelection(ids); e == nil {
			t.Fatalf("accepted %v", ids)
		}
	}
}
func TestGoBatchRecoverPartial(t *testing.T) {
	changes := []goChange{{ID: "go-temp", Target: `E:\Go\temp`}, {ID: "go-mod", Target: `E:\Go\mod`, Old: prior{Value: `C:\go\mod`, Exists: true, Kind: 1}}}
	current := map[string]prior{"GOMODCACHE": changes[1].Old}
	read := func(_ bool, k string) (prior, error) { return current[k], nil }
	write := func(k string, p prior) error { current[k] = p; return nil }
	count, e := commitGoChangesWith(changes, read, func(k string, p prior) error {
		if k == "GOMODCACHE" {
			return errors.New("simulated write failure")
		}
		return write(k, p)
	})
	if e == nil || count != 1 || current["GOTMPDIR"].Value != changes[0].Target {
		t.Fatalf("count=%d err=%v state=%+v", count, e, current)
	}
	if e = restoreGoChangesWith(changes[:count], read, write); e != nil {
		t.Fatal(e)
	}
	if current["GOTMPDIR"] != (prior{}) || current["GOMODCACHE"] != changes[1].Old {
		t.Fatalf("original values not restored: %+v", current)
	}
}
func TestGoRestoreNeedsWholePreflight(t *testing.T) {
	changes := []goChange{{ID: "go-temp", Target: `E:\Go\temp`}, {ID: "go-mod", Target: `E:\Go\mod`}}
	entries := map[string]prior{"GOTMPDIR": {Value: changes[0].Target, Exists: true, Kind: 1}, "GOMODCACHE": {Value: `R:\third-party`, Exists: true, Kind: 1}}
	n := 0
	err := restoreGoChangesWith(changes, func(_ bool, k string) (prior, error) { return entries[k], nil }, func(k string, v prior) error { n++; return nil })
	if err == nil || n != 0 {
		t.Fatalf("should reject external override before mutating: %d, %v", n, err)
	}
}
func TestGoBatchJournal(t *testing.T) {
	path := filepath.Join(t.TempDir(), goRootFile)
	want := []goChange{{ID: "go-mod", Target: filepath.Join(t.TempDir(), "mod"), Effective: "old"}}
	if e := writeGoJournal(path, want); e != nil {
		t.Fatal(e)
	}
	if e := writeGoJournal(path, want); !os.IsExist(e) {
		t.Fatalf("should reject journal overwrite: %v", e)
	}
	got, e := readGoJournal(path)
	if e != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("journal %v: %v", got, e)
	}
}
