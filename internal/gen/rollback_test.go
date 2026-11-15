package gen

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iYeremy/dotka/internal/scan"
)

func read(t *testing.T, home, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(home, rel))
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	return string(b)
}

func exists(home, rel string) bool {
	_, err := os.Lstat(filepath.Join(home, rel))
	return err == nil
}

// fakeInstall simulates a typical dotfiles installer.
func fakeInstall(t *testing.T, home string) {
	t.Helper()
	write(t, home, ".config/hypr/hyprland.conf", "exec-once = swww-daemon\n")
	os.Remove(filepath.Join(home, ".config/kitty/kitty.conf"))
	if err := os.Symlink("/dots/kitty.conf", filepath.Join(home, ".config/kitty/kitty.conf")); err != nil {
		t.Fatal(err)
	}
	write(t, home, ".config/waybar/config.jsonc", "{}")
	write(t, home, ".config/waybar/modules/clock.json", "{}")
	os.Remove(filepath.Join(home, ".zshrc"))
	os.Chmod(filepath.Join(home, ".config/hypr/scripts/run.sh"), 0o644)
}

func setupBase(t *testing.T) *Manager {
	t.Helper()
	m := newManager(t)
	write(t, m.Home, ".config/hypr/hyprland.conf", "exec-once = waybar\n")
	write(t, m.Home, ".config/hypr/scripts/run.sh", "#!/bin/sh\n")
	os.Chmod(filepath.Join(m.Home, ".config/hypr/scripts/run.sh"), 0o755)
	write(t, m.Home, ".config/kitty/kitty.conf", "font_size 11\n")
	write(t, m.Home, ".zshrc", "echo hi\n")
	return m
}

func TestRollbackRestoresExactState(t *testing.T) {
	m := setupBase(t)
	g1, _, err := m.Snapshot("before", OriginManual)
	if err != nil {
		t.Fatal(err)
	}
	fakeInstall(t, m.Home)

	res, err := m.Rollback(g1.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Failures) > 0 {
		t.Fatalf("failures: %+v", res.Failures)
	}
	if got := read(t, m.Home, ".config/hypr/hyprland.conf"); got != "exec-once = waybar\n" {
		t.Errorf("hyprland.conf = %q", got)
	}
	fi, err := os.Lstat(filepath.Join(m.Home, ".config/kitty/kitty.conf"))
	if err != nil || !fi.Mode().IsRegular() || read(t, m.Home, ".config/kitty/kitty.conf") != "font_size 11\n" {
		t.Errorf("kitty.conf not restored as regular file: %v %v", fi, err)
	}
	if read(t, m.Home, ".zshrc") != "echo hi\n" {
		t.Error(".zshrc not restored")
	}
	if exists(m.Home, ".config/waybar") {
		t.Error("added waybar directory should be removed")
	}
	if fi, _ := os.Stat(filepath.Join(m.Home, ".config/hypr/scripts/run.sh")); fi.Mode().Perm() != 0o755 {
		t.Errorf("mode not restored: %v", fi.Mode())
	}
	// The home now matches generation 1 exactly.
	cur, _ := m.Current()
	if d := Diff(g1.Entries, cur.Entries); len(d) != 0 {
		t.Errorf("home differs from target after rollback: %v", kinds(d))
	}
	if head, _ := m.Store.Head(); head != g1.ID {
		t.Errorf("HEAD = %d, want %d", head, g1.ID)
	}
}

func TestRollbackIsReversible(t *testing.T) {
	m := setupBase(t)
	g1, _, _ := m.Snapshot("before", OriginManual)
	fakeInstall(t, m.Home)
	after, _ := m.Current()

	res, err := m.Rollback(g1.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	// Undo the rollback using the automatic snapshot.
	res2, err := m.Rollback(res.Pre.ID, false)
	if err != nil || len(res2.Failures) > 0 {
		t.Fatalf("undo failed: %v %+v", err, res2.Failures)
	}
	cur, _ := m.Current()
	if d := Diff(after.Entries, cur.Entries); len(d) != 0 {
		t.Fatalf("undo did not restore the post-install state: %v", kinds(d))
	}
	if read(t, m.Home, ".config/waybar/modules/clock.json") != "{}" {
		t.Error("waybar file not back")
	}
}

func TestRollbackKeepNew(t *testing.T) {
	m := setupBase(t)
	g1, _, _ := m.Snapshot("", OriginManual)
	fakeInstall(t, m.Home)
	if _, err := m.Rollback(g1.ID, true); err != nil {
		t.Fatal(err)
	}
	if !exists(m.Home, ".config/waybar/config.jsonc") {
		t.Error("--keep-new must keep added files")
	}
	if read(t, m.Home, ".config/hypr/hyprland.conf") != "exec-once = waybar\n" {
		t.Error("modified files must still be restored with --keep-new")
	}
}

func TestRollbackNeverDeletesUnstoredFiles(t *testing.T) {
	m := setupBase(t)
	m.Config.MaxFileSize = 64
	g1, _, _ := m.Snapshot("", OriginManual)
	// Added after g1: a secret and a large file, both impossible to store.
	write(t, m.Home, ".config/gh/hosts.yml", "oauth_token: x\n")
	write(t, m.Home, ".config/big/blob.bin", string(make([]byte, 128)))
	res, err := m.Rollback(g1.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if !exists(m.Home, ".config/gh/hosts.yml") || !exists(m.Home, ".config/big/blob.bin") {
		t.Fatal("rollback deleted a file it could not restore later")
	}
	if len(res.Plan.Skipped) < 2 {
		t.Errorf("expected skipped entries, got %+v", res.Plan.Skipped)
	}
	// Their parent dirs must not be planned for removal.
	for _, a := range res.Plan.Actions {
		if a.Path == ".config/gh" || a.Path == ".config/big" {
			t.Errorf("parent of a kept file planned for %s", a.Kind)
		}
	}
}

func TestRollbackLeavesExcludedContentAlone(t *testing.T) {
	m := setupBase(t)
	g1, _, _ := m.Snapshot("", OriginManual)
	// An app dir appears with a cache subdir (excluded by name).
	write(t, m.Home, ".config/someapp/settings.json", "{}")
	write(t, m.Home, ".config/someapp/Cache/data", "cache")
	res, err := m.Rollback(g1.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if exists(m.Home, ".config/someapp/settings.json") {
		t.Error("tracked added file should be removed")
	}
	if !exists(m.Home, ".config/someapp/Cache/data") {
		t.Error("excluded content must never be touched")
	}
	// rmdir of someapp fails (not empty) and is reported, not fatal.
	if len(res.Failures) != 1 || res.Failures[0].Action.Path != ".config/someapp" {
		t.Errorf("expected one rmdir failure for someapp, got %+v", res.Failures)
	}
}

func TestApplyRefusesPathTraversal(t *testing.T) {
	m := newManager(t)
	outside := t.TempDir()
	evil := scan.Entry{Path: "../" + filepath.Base(outside) + "/pwned", Type: scan.TypeSymlink, Target: "x"}
	plan := Plan{Actions: []Action{{Kind: ActSymlink, Path: evil.Path, Want: &evil}}}
	fails := Apply(m.Home, m.Store, plan)
	if len(fails) != 1 {
		t.Fatalf("expected failure, got %v", fails)
	}
	if exists(outside, "pwned") {
		t.Fatal("wrote outside home")
	}
}

func TestApplyRefusesSymlinkedParent(t *testing.T) {
	m := newManager(t)
	outside := t.TempDir()
	os.MkdirAll(filepath.Join(m.Home, ".config"), 0o755)
	os.Symlink(outside, filepath.Join(m.Home, ".config/evil"))
	e := scan.Entry{Path: ".config/evil/pwned", Type: scan.TypeSymlink, Target: "x"}
	fails := Apply(m.Home, m.Store, Plan{Actions: []Action{{Kind: ActSymlink, Path: e.Path, Want: &e}}})
	if len(fails) != 1 || exists(outside, "pwned") {
		t.Fatalf("followed a symlinked parent: %v", fails)
	}
}

func TestRemoveVerifiesFileUnchanged(t *testing.T) {
	m := setupBase(t)
	g1, _, _ := m.Snapshot("", OriginManual)
	write(t, m.Home, ".config/new.conf", "v1")
	plan, _, err := m.Preview(g1.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	// The user edits the file between preview and apply.
	write(t, m.Home, ".config/new.conf", "v2 edited")
	fails := Apply(m.Home, m.Store, plan)
	if len(fails) != 1 || read(t, m.Home, ".config/new.conf") != "v2 edited" {
		t.Fatalf("changed file was removed: %v", fails)
	}
}

func TestPreviewWritesNothing(t *testing.T) {
	m := setupBase(t)
	g1, _, _ := m.Snapshot("", OriginManual)
	fakeInstall(t, m.Home)
	before, _ := m.Store.GenerationIDs()
	plan, _, err := m.Preview(g1.ID, false)
	if err != nil || plan.Empty() {
		t.Fatalf("preview: %v empty=%v", err, plan.Empty())
	}
	after, _ := m.Store.GenerationIDs()
	if len(before) != len(after) || !exists(m.Home, ".config/waybar/config.jsonc") {
		t.Fatal("preview modified the store or the home")
	}
}

func TestRollbackIgnoresRootsNotTrackedInTarget(t *testing.T) {
	m := setupBase(t)
	g1, _, _ := m.Snapshot("", OriginManual)
	write(t, m.Home, ".local/share/nvim/init.lua", "x")
	m.Config.Roots = append(m.Config.Roots, ".local/share/nvim")
	if _, err := m.Rollback(g1.ID, false); err != nil {
		t.Fatal(err)
	}
	if !exists(m.Home, ".local/share/nvim/init.lua") {
		t.Fatal("rollback removed files from a root that was not tracked in the target")
	}
}

// On NixOS/home-manager, links into /nix/store belong to Nix: rollback must
// never recreate, retarget or remove them.
func TestRollbackLeavesNixStoreLinksAlone(t *testing.T) {
	m := setupBase(t)
	os.Symlink("/nix/store/aaa-hm-files/kitty.conf", filepath.Join(m.Home, ".config/kitty/nix.conf"))
	g1, _, _ := m.Snapshot("", OriginManual)
	// home-manager switch: retargets one link and adds another.
	os.Remove(filepath.Join(m.Home, ".config/kitty/nix.conf"))
	os.Symlink("/nix/store/bbb-hm-files/kitty.conf", filepath.Join(m.Home, ".config/kitty/nix.conf"))
	os.Symlink("/nix/store/bbb-hm-files/foot.ini", filepath.Join(m.Home, ".config/foot.ini"))
	write(t, m.Home, ".zshrc", "changed\n") // an imperative change: restored as usual

	res, err := m.Rollback(g1.ID, false)
	if err != nil || len(res.Failures) > 0 {
		t.Fatalf("rollback: %v %+v", err, res.Failures)
	}
	if tgt, _ := os.Readlink(filepath.Join(m.Home, ".config/kitty/nix.conf")); tgt != "/nix/store/bbb-hm-files/kitty.conf" {
		t.Errorf("nix link was retargeted to %q", tgt)
	}
	if !exists(m.Home, ".config/foot.ini") {
		t.Error("nix link added by home-manager was removed")
	}
	if read(t, m.Home, ".zshrc") != "echo hi\n" {
		t.Error("imperative file not restored")
	}
	n := 0
	for _, s := range res.Plan.Skipped {
		if strings.Contains(s.Reason, "Nix") {
			n++
		}
	}
	if n != 2 {
		t.Errorf("expected 2 nix skips, got %+v", res.Plan.Skipped)
	}
}
