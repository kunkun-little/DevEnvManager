package main

import "testing"

func TestParseExactWinGetEntry(t *testing.T) {
	const id = "OpenJS.NodeJS.LTS"
	ok, v := parseExactWinGetEntry("Name Id Version Available Source\nNode.js (LTS) OpenJS.NodeJS.LTS 22.14.0 22.23.0 winget\n", id)
	if !ok || v != "22.14.0" {
		t.Fatalf("%v %q", ok, v)
	}
	if ok, _ := parseExactWinGetEntry("Node OpenJS.NodeJS.LTS.Nightly 22.0.0 winget", id); ok {
		t.Fatal("accepted prefix match")
	}
	if ok, v := parseExactWinGetEntry("Node OpenJS.NodeJS.LTS Unknown winget", id); !ok || v != "" {
		t.Fatalf("unknown version should not be interpreted as numeric: %v %q", ok, v)
	}
}
func TestWingetActionRejectsUnknownOrWrongRegistration(t *testing.T) {
	if e := gatePackageAction("install", PackageState{Available: true, Error: "no network"}); e == nil {
		t.Fatal("unknown was treated as absent")
	}
	if e := gatePackageAction("install", PackageState{Available: true, Registered: true}); e == nil {
		t.Fatal("duplicate install allowed")
	}
	if e := gatePackageAction("uninstall", PackageState{Available: true}); e == nil {
		t.Fatal("uninstalled unmatched package")
	}
	if e := gatePackageAction("upgrade", PackageState{Available: true, Registered: true}); e != nil {
		t.Fatal(e)
	}
}
