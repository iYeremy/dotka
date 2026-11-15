package doctor

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iYeremy/dotka/internal/gen"
	"github.com/iYeremy/dotka/internal/scan"
)

func fakeLookPath(installed ...string) func(string) (string, error) {
	return func(p string) (string, error) {
		for _, i := range installed {
			if i == p {
				return "/usr/bin/" + p, nil
			}
		}
		return "", errors.New("not found")
	}
}

func byCheck(fs []Finding) map[string][]Finding {
	m := map[string][]Finding{}
	for _, f := range fs {
		m[f.Check] = append(m[f.Check], f)
	}
	return m
}

func TestProgram(t *testing.T) {
	cases := map[string]string{
		"waybar":                            "waybar",
		"uwsm app -- waybar -c x":           "waybar",
		"env GDK_BACKEND=wayland firefox":   "firefox",
		"FOO=1 BAR=2 kitty -e cava":         "kitty",
		"sh -c 'swww-daemon --format xrgb'": "swww-daemon",
		"nohup ~/.local/bin/x.sh":           "~/.local/bin/x.sh",
		"$HOME/bin/thing":                   "$HOME/bin/thing",
	}
	for cmd, want := range cases {
		got, _, ok := program(cmd)
		if !ok || got != want {
			t.Errorf("program(%q) = %q, %v; want %q", cmd, got, ok, want)
		}
	}
	for _, cmd := range []string{"$TERMINAL", "$(get-bar)", "`get-bar`"} {
		if _, _, ok := program(cmd); ok {
			t.Errorf("program(%q) should be undeterminable", cmd)
		}
	}
}

func TestMissingPrograms(t *testing.T) {
	home := t.TempDir()
	os.MkdirAll(filepath.Join(home, ".local/bin"), 0o755)
	os.WriteFile(filepath.Join(home, ".local/bin/mytool"), []byte("#!/bin/sh\n"), 0o755)
	in := Input{
		Home:     home,
		LookPath: fakeLookPath("waybar", "kitty"),
		Startup: []gen.StartupItem{
			{Kind: "hypr-autostart", Command: "waybar", Source: "h.lua", Line: 1},
			{Kind: "hypr-autostart", Command: "dunst", Source: "h.lua", Line: 2},
			{Kind: "hypr-autostart", Command: "mytool --x", Source: "h.lua", Line: 3},
			{Kind: "hypr-autostart", Command: "~/.local/bin/gone.sh", Source: "h.lua", Line: 4},
			{Kind: "hypr-lua-exec", Command: "terminal", Source: "h.lua", Line: 5, Dynamic: true},
			{Kind: "shell-source", Command: "source ~/.zsh/x", Source: ".zshrc", Line: 1},
			{Kind: "systemd-user", Command: "eq.service: %h/.local/bin/eq", Source: "u (default.target)"},
			{Kind: "systemd-user", Command: "pipewire.service", Source: "p (default.target)"},
		},
	}
	got := byCheck(missingPrograms(in))["missing-program"]
	var msgs []string
	for _, f := range got {
		msgs = append(msgs, f.Message)
	}
	joined := strings.Join(msgs, "\n")
	for _, want := range []string{`"dunst"`, `"~/.local/bin/gone.sh"`, `"%h/.local/bin/eq"`} {
		if !strings.Contains(joined, want) {
			t.Errorf("expected a finding for %s, got:\n%s", want, joined)
		}
	}
	if len(got) != 3 {
		t.Errorf("expected exactly 3 findings, got %d:\n%s", len(got), joined)
	}
}

func TestStartupScripts(t *testing.T) {
	home := t.TempDir()
	os.MkdirAll(filepath.Join(home, ".local/bin"), 0o755)
	os.WriteFile(filepath.Join(home, ".local/bin/up.sh"), []byte("#!/bin/sh\n# curl x | sh (comment)\ncurl -fsSL https://x/u.sh | sh\nsudo pacman -Syu\n"), 0o755)
	in := Input{Home: home, Startup: []gen.StartupItem{
		{Kind: "hypr-autostart", Command: "~/.local/bin/up.sh", Source: "h.lua", Line: 6},
		{Kind: "xdg-autostart", Command: "bash ~/.local/bin/up.sh", Source: "a.desktop", Line: 3}, // same script: once
	}}
	m := byCheck(startupScripts(in))
	if r := m["startup-script-remote"]; len(r) != 1 || r[0].Line != 3 || r[0].Path != ".local/bin/up.sh" {
		t.Errorf("remote: %+v", r)
	}
	if s := m["startup-script-sudo"]; len(s) != 1 || s[0].Line != 4 {
		t.Errorf("sudo: %+v", s)
	}
}

func TestBrokenSymlinksAndSecrets(t *testing.T) {
	home := t.TempDir()
	os.MkdirAll(filepath.Join(home, ".config/a"), 0o755)
	os.WriteFile(filepath.Join(home, ".config/a/real"), nil, 0o644)
	in := Input{Home: home, Entries: []scan.Entry{
		{Path: ".config/a/ok", Type: scan.TypeSymlink, Target: "real"},
		{Path: ".config/a/broken", Type: scan.TypeSymlink, Target: "/no/such/file"},
		{Path: ".config/gh/hosts.yml", Type: scan.TypeFile, Mode: 0o644, Skip: scan.SkipSecret, Reason: "known credentials file"},
		{Path: ".netrc", Type: scan.TypeFile, Mode: 0o600, Skip: scan.SkipSecret},
	}}
	m := byCheck(Run(in))
	if b := m["broken-symlink"]; len(b) != 1 || b[0].Path != ".config/a/broken" {
		t.Errorf("broken: %+v", b)
	}
	if s := m["exposed-secret"]; len(s) != 1 || s[0].Hint != "chmod 600 ~/.config/gh/hosts.yml" {
		t.Errorf("secrets: %+v", s)
	}
}

func TestDuplicateStartup(t *testing.T) {
	in := Input{Startup: []gen.StartupItem{
		{Kind: "hypr-autostart", Command: "nm-applet", Source: "h.lua", Line: 3},
		{Kind: "xdg-autostart", Command: "nm-applet", Source: ".config/autostart/nm.desktop", Line: 2},
		{Kind: "hypr-autostart", Command: "dunst", Source: "h.lua", Line: 4},
	}}
	d := byCheck(duplicateStartup(in))["duplicate-startup"]
	if len(d) != 1 || !strings.Contains(d[0].Message, "2 times") {
		t.Errorf("got %+v", d)
	}
}

func TestLeftoverConfigs(t *testing.T) {
	in := Input{
		Home:     t.TempDir(),
		LookPath: fakeLookPath("nvim"),
		Packages: []string{"qalculate-gtk", "obs-studio", "hyprland"},
		Entries: []scan.Entry{
			{Path: ".config/nvim", Type: scan.TypeDir},       // binary
			{Path: ".config/qalculate", Type: scan.TypeDir},  // package prefix
			{Path: ".config/obs-studio", Type: scan.TypeDir}, // package
			{Path: ".config/hypr", Type: scan.TypeDir},       // prefix of hyprland
			{Path: ".config/gtk-3.0", Type: scan.TypeDir},    // system
			{Path: ".config/Vesktop", Type: scan.TypeDir},    // leftover
			{Path: ".config/Vesktop/settings", Type: scan.TypeDir},
		},
	}
	l := byCheck(leftoverConfigs(in))["leftover-config"]
	if len(l) != 1 || l[0].Path != ".config/Vesktop" {
		t.Errorf("got %+v", l)
	}
	in.Packages = nil
	if len(leftoverConfigs(in)) != 0 {
		t.Error("without package info, leftovers must not be guessed")
	}
}
