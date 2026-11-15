package gen

import (
	"strings"
	"testing"

	"github.com/iYeremy/dotka/internal/scan"
)

func TestRunInfoDisplay(t *testing.T) {
	r := RunInfo{Command: []string{"/home/u/dots/super-rice/install.sh", "--all"}, Dir: "/home/u/dots"}
	if got := r.Display(); got != "./super-rice/install.sh --all" {
		t.Errorf("Display = %q", got)
	}
	r.Dir = "/elsewhere"
	if got := r.Display(); got != ".../super-rice/install.sh --all" {
		t.Errorf("Display = %q", got)
	}
}

func TestSnapshotRecordsParent(t *testing.T) {
	m := newManager(t)
	write(t, m.Home, ".zshrc", "a\n")
	g1, _, _ := m.Snapshot("", OriginManual)
	write(t, m.Home, ".zshrc", "b\n")
	g2, _, _ := m.Snapshot("", OriginManual)
	if g1.Parent != 0 || g2.Parent != g1.ID {
		t.Fatalf("parents: %d %d", g1.Parent, g2.Parent)
	}
	if _, err := m.Rollback(g1.ID, false); err != nil {
		t.Fatal(err)
	}
	g4, _, _ := m.Snapshot("after rollback", OriginManual)
	if g4.Parent != g1.ID {
		t.Fatalf("after rollback to #1 the parent must be #1, got %d", g4.Parent)
	}
}

func TestFileHistoryAttributesRunsAndIgnoresRollbacks(t *testing.T) {
	m := newManager(t)
	write(t, m.Home, ".zshrc", "mine\n")
	g1, _, _ := m.Snapshot("init", OriginInit)
	// A run adds waybar config.
	write(t, m.Home, ".config/waybar/config", "{}")
	m.SnapshotRun("rice", OriginRun, &RunInfo{Command: []string{"./install.sh"}, Before: g1.ID})
	// Rollback to #1 and snapshot: the removal is the rollback, not an event.
	m.Rollback(g1.ID, false)
	m.Snapshot("after", OriginManual)

	evs, err := m.FileHistory(".config/waybar/config")
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) != 1 || evs[0].Kind != Added || !strings.Contains(evs[0].Cause(), "./install.sh") {
		t.Fatalf("events: %+v", evs)
	}
	z, _ := m.FileHistory(".zshrc")
	if len(z) != 1 || !strings.Contains(z[0].Cause(), "initialized") {
		t.Fatalf(".zshrc events: %+v", z)
	}
}

func TestPackageKeptAfterRollback(t *testing.T) {
	m := newManager(t)
	pk := []string{"hyprland"}
	m.Collector = func(string, []scan.Entry) (Extra, []scan.Warning) {
		return Extra{Packages: &Packages{Explicit: append([]string(nil), pk...)}}, nil
	}
	g1, _, _ := m.Snapshot("", OriginInit)
	pk = append(pk, "waybar")
	m.SnapshotRun("rice", OriginRun, &RunInfo{Command: []string{"./install.sh"}, Before: g1.ID})
	m.Rollback(g1.ID, false) // packages are not touched: waybar stays
	m.Snapshot("after", OriginManual)

	evs, _ := m.PackageHistory("waybar")
	if len(evs) != 2 {
		t.Fatalf("events: %+v", evs)
	}
	if !strings.Contains(evs[0].Cause(), "./install.sh") {
		t.Errorf("first event cause: %s", evs[0].Cause())
	}
	if evs[1].Kept != evs[0].Gen.ID || !strings.Contains(evs[1].Cause(), "kept after going back") {
		t.Errorf("second event should be marked as kept: %+v / %s", evs[1], evs[1].Cause())
	}
}

func TestStartupHistory(t *testing.T) {
	m := newManager(t)
	items := []StartupItem{{Kind: "hypr-autostart", Command: "dunst", Source: "h"}}
	m.Collector = func(string, []scan.Entry) (Extra, []scan.Warning) {
		return Extra{Startup: append([]StartupItem(nil), items...)}, nil
	}
	m.Snapshot("", OriginInit)
	items = append(items, StartupItem{Kind: "hypr-autostart", Command: "waybar", Source: "h"})
	m.SnapshotRun("rice", OriginRun, &RunInfo{Command: []string{"./install.sh"}})
	evs, _ := m.StartupHistory("waybar")
	if len(evs) != 1 || evs[0].Kind != Added || evs[0].Gen.Run == nil {
		t.Fatalf("events: %+v", evs)
	}
}
