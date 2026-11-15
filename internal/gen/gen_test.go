package gen

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/iYeremy/dotka/internal/config"
	"github.com/iYeremy/dotka/internal/store"
)

// newManager creates a fake home and store for tests.
func newManager(t *testing.T) *Manager {
	t.Helper()
	home := t.TempDir()
	s, err := store.Create(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	cfg.Roots = []string{".config", ".zshrc"}
	return &Manager{Home: home, Store: s, Config: cfg}
}

func write(t *testing.T, home, rel, content string) {
	t.Helper()
	p := filepath.Join(home, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func kinds(changes []Change) map[string]string {
	m := map[string]string{}
	for _, c := range changes {
		m[c.Path] = c.Kind
	}
	return m
}

func TestSnapshotAndDiff(t *testing.T) {
	m := newManager(t)
	write(t, m.Home, ".config/hypr/hyprland.conf", "exec-once = waybar\n")
	write(t, m.Home, ".config/kitty/kitty.conf", "font_size 11\n")
	write(t, m.Home, ".zshrc", "echo hi\n")
	g1, _, err := m.Snapshot("before", OriginManual)
	if err != nil {
		t.Fatal(err)
	}
	if g1.ID != 1 {
		t.Fatalf("first id = %d", g1.ID)
	}

	// Simulate an installer.
	write(t, m.Home, ".config/hypr/hyprland.conf", "exec-once = waybar\nexec-once = swww-daemon\n")
	os.Remove(filepath.Join(m.Home, ".config/kitty/kitty.conf"))
	os.Symlink("/dots/kitty.conf", filepath.Join(m.Home, ".config/kitty/kitty.conf"))
	write(t, m.Home, ".config/waybar/config.jsonc", "{}")
	os.Chmod(filepath.Join(m.Home, ".zshrc"), 0o600)

	cur, err := m.Current()
	if err != nil {
		t.Fatal(err)
	}
	got := kinds(Diff(g1.Entries, cur.Entries))
	want := map[string]string{
		".config/hypr/hyprland.conf":  Modified,
		".config/kitty/kitty.conf":    TypeChg,
		".config/waybar":              Added,
		".config/waybar/config.jsonc": Added,
		".zshrc":                      ModeChg,
	}
	for p, k := range want {
		if got[p] != k {
			t.Errorf("%s: got %q, want %q", p, got[p], k)
		}
	}
	if len(got) != len(want) {
		t.Errorf("unexpected changes: %v", got)
	}

	g2, _, err := m.Snapshot("after", OriginManual)
	if err != nil {
		t.Fatal(err)
	}
	if head, _ := m.Store.Head(); head != g2.ID {
		t.Fatalf("HEAD = %d, want %d", head, g2.ID)
	}
	// Nothing changed since g2.
	cur, _ = m.Current()
	if d := Diff(g2.Entries, cur.Entries); len(d) != 0 {
		t.Fatalf("expected no changes, got %v", kinds(d))
	}
}

func TestFilterRootsIgnoresNewlyTrackedRoots(t *testing.T) {
	changes := []Change{
		{Path: ".config/a", Kind: Added},
		{Path: ".local/share/nvim/x", Kind: Added},
	}
	f := FilterRoots(changes, []string{".config"}, []string{".config", ".local/share/nvim"})
	if len(f) != 1 || f[0].Path != ".config/a" {
		t.Fatalf("got %v", f)
	}
}

func TestDiffExtra(t *testing.T) {
	o := Extra{
		Packages: &Packages{Explicit: []string{"hyprland", "kitty"}},
		Startup:  []StartupItem{{Kind: "hypr-exec-once", Command: "waybar", Source: "a", Line: 1}},
	}
	n := Extra{
		Packages: &Packages{Explicit: []string{"hyprland", "swww"}, Foreign: []string{"swww"}},
		Startup: []StartupItem{
			{Kind: "hypr-exec-once", Command: "waybar", Source: "a", Line: 5}, // moved: same item
			{Kind: "hypr-exec-once", Command: "swww-daemon", Source: "a", Line: 6},
		},
	}
	d := DiffExtra(o, n)
	if len(d.PkgAdded) != 1 || d.PkgAdded[0] != "swww" || len(d.PkgRemoved) != 1 || d.PkgRemoved[0] != "kitty" {
		t.Errorf("packages diff wrong: %+v", d)
	}
	if len(d.StartupAdded) != 1 || d.StartupAdded[0].Command != "swww-daemon" || len(d.StartupRemoved) != 0 {
		t.Errorf("startup diff wrong: %+v", d)
	}
	// Without package info on one side, no package changes are reported.
	if d := DiffExtra(Extra{}, n); len(d.PkgAdded) != 0 {
		t.Error("package diff reported without baseline")
	}
}

func TestLiveObjects(t *testing.T) {
	m := newManager(t)
	write(t, m.Home, ".zshrc", "v1\n")
	m.Snapshot("", OriginManual)
	write(t, m.Home, ".zshrc", "v2\n")
	m.Snapshot("", OriginManual)
	keep, err := m.LiveObjects()
	if err != nil {
		t.Fatal(err)
	}
	if !keep[store.HashBytes([]byte("v1\n"))] || !keep[store.HashBytes([]byte("v2\n"))] {
		t.Fatal("both versions must be live")
	}
}
