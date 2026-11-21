package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCatalog(t *testing.T) {
	a := NewApp()
	seen := map[string]bool{}
	for _, r := range a.Catalog() {
		if r.ID == "" || r.Title == "" || r.Docs == "" || seen[r.ID] {
			t.Fatalf("invalid row %+v", r)
		}
		seen[r.ID] = true
		if strings.ContainsAny(r.Command+r.Cleanup, "|&%<>\n;") {
			t.Fatalf("unsupported command characters: %+v", r)
		}
		if r.Cleanup != "" && r.Command == "" {
			t.Fatalf("unobservable cleanup %+v", r)
		}
	}
	if len(seen) != 25 {
		t.Fatalf("catalog size %d", len(seen))
	}
}
func TestPlanToken(t *testing.T) {
	a := NewApp()
	p := a.plan(request{action: "test"}, "Test", nil)
	if len(p.Token) != 48 || p.Error != "" {
		t.Fatalf("%+v", p)
	}
	if _, e := a.take(p.Token, "test"); e != nil {
		t.Fatal(e)
	}
	if _, e := a.take(p.Token, "test"); e == nil {
		t.Fatal("replayed")
	}
	p = a.plan(request{action: "test"}, "Test", nil)
	r := a.pending[p.Token]
	r.expires = time.Now().Add(-time.Second)
	a.pending[p.Token] = r
	if _, e := a.take(p.Token, "test"); e == nil {
		t.Fatal("accepted expired token")
	}
	if got := a.Rollback("../payload"); got.Error == "" {
		t.Fatal("accepted arbitrary rollback target")
	}
}
func TestScan(t *testing.T) {
	dir := t.TempDir()
	if e := os.WriteFile(filepath.Join(dir, "cache"), []byte("abc"), 0600); e != nil {
		t.Fatal(e)
	}
	n, s, e := scan(dir, time.Second, true)
	if n != 3 || e != nil || !strings.Contains(s, "逻辑大小") {
		t.Fatalf("%d %s %v", n, s, e)
	}
	if e := os.Symlink(dir, filepath.Join(dir, "loop")); e != nil {
		t.Skip("symlinks not enabled")
	}
	if _, _, e := scan(dir, time.Second, true); e == nil {
		t.Fatal("clean scan accepted symlink")
	}
	n, s, e = scan(dir, time.Second, false)
	if n != 3 || e != nil || !strings.Contains(s, "部分") {
		t.Fatalf("%d %s %v", n, s, e)
	}
}
func TestInside(t *testing.T) {
	base := filepath.Join("base", "home")
	if !inside(base, filepath.Join(base, "test")) || inside(base, filepath.Join("base", "homeother", "test")) {
		t.Fatal("path segment comparison failed")
	}
}
