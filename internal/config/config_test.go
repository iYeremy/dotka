package config

import (
	"path/filepath"
	"testing"
)

func TestExcluded(t *testing.T) {
	c := Default()
	cases := map[string]bool{
		".cache":                            true,
		".cache/foo":                        true,
		".config/chromium/Default/x":        true,
		".config/discord/Cache":             true, // by base name
		".config/hypr/hyprland.lua":         false,
		".config/cache-not-excluded/a.conf": false,
		".local/share/dotka/objects/ab":     true,
	}
	for p, want := range cases {
		if got := c.Excluded(p); got != want {
			t.Errorf("Excluded(%q) = %v, want %v", p, got, want)
		}
	}
}

func TestRootsAddRemoveAndPersist(t *testing.T) {
	c := Default()
	if ok, err := c.AddRoot(".local/share/nvim"); !ok || err != nil {
		t.Fatalf("AddRoot = %v %v", ok, err)
	}
	if ok, _ := c.AddRoot(".local/share/nvim/"); ok {
		t.Fatal("duplicate root (with trailing slash) accepted")
	}
	if _, err := c.AddRoot("../etc"); err == nil {
		t.Fatal("unsafe root accepted")
	}
	dir := t.TempDir()
	if err := c.Save(dir); err != nil {
		t.Fatal(err)
	}
	l, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !UnderRoots(l.Roots, filepath.Join(".local/share/nvim", "lazy", "x.lua")) {
		t.Fatal("persisted root missing")
	}
	if !l.RemoveRoot(".local/share/nvim") || UnderRoots(l.Roots, ".local/share/nvim/x") {
		t.Fatal("RemoveRoot failed")
	}
}
