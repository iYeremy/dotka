package startup

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/iYeremy/dotka/internal/gen"
)

func kinds(t *testing.T, f func(string) ([]gen.StartupItem, []Warning), home string) map[string]string {
	t.Helper()
	items, warns := f(home)
	if len(warns) > 0 {
		t.Fatalf("warnings: %v", warns)
	}
	m := map[string]string{}
	for _, i := range items {
		m[i.Command] = i.Kind
	}
	return m
}

func expectKinds(t *testing.T, got, want map[string]string) {
	t.Helper()
	for c, k := range want {
		if got[c] != k {
			t.Errorf("%q: got kind %q, want %q", c, got[c], k)
		}
	}
	if len(got) != len(want) {
		t.Errorf("got %d items, want %d: %v", len(got), len(want), got)
	}
}

func TestSwayAndI3(t *testing.T) {
	home := t.TempDir()
	put(t, home, ".config/sway/config", `
exec waybar
exec_always --no-startup-id autotiling
# exec disabled
bindsym $mod+Return exec foot
include ~/.config/sway/config.d/*
`)
	put(t, home, ".config/sway/config.d/50-apps", "exec mako # notifications\n")
	expectKinds(t, kinds(t, sway, home), map[string]string{
		"waybar": "sway-exec", "autotiling": "sway-exec-always", "mako": "sway-exec",
	})
	put(t, home, ".i3/config", "exec --no-startup-id nm-applet\n")
	expectKinds(t, kinds(t, i3, home), map[string]string{"nm-applet": "i3-exec"})
}

func TestNiri(t *testing.T) {
	home := t.TempDir()
	put(t, home, ".config/niri/config.kdl", `
spawn-at-startup "waybar"
spawn-at-startup "sh" "-c" "swaybg -i ~/wall.png"
spawn-sh-at-startup "qs -c noctalia"
// spawn-at-startup "commented"
/-spawn-at-startup "slashdashed"
/*
spawn-at-startup "in block comment"
*/
binds {
    Mod+T { spawn "alacritty"; }
}
`)
	expectKinds(t, kinds(t, niri, home), map[string]string{
		"waybar": "niri-startup", "sh -c swaybg -i ~/wall.png": "niri-startup", "qs -c noctalia": "niri-startup",
	})
}

func TestRiverWayfireScriptsKDE(t *testing.T) {
	home := t.TempDir()
	put(t, home, ".config/river/init", "#!/bin/sh\nriverctl map normal Super Return spawn foot\nriverctl spawn waybar\nriverctl spawn 'swaybg -i x'\n")
	expectKinds(t, kinds(t, river, home), map[string]string{"waybar": "river-spawn", "swaybg -i x": "river-spawn"})

	put(t, home, ".config/wayfire.ini", "[core]\nplugins = autostart\n[autostart]\nautostart_wf_shell = false\nbar = waybar\nportal = /usr/libexec/xdg-desktop-portal\n")
	expectKinds(t, kinds(t, wayfire, home), map[string]string{"waybar": "wayfire-autostart", "/usr/libexec/xdg-desktop-portal": "wayfire-autostart"})

	put(t, home, ".config/bspwm/bspwmrc", "#!/bin/sh\nsxhkd &\nbspc monitor -d I II III\npolybar main &\n# picom &\n")
	put(t, home, ".config/openbox/autostart", "tint2 &\n")
	expectKinds(t, kinds(t, scripts, home), map[string]string{"sxhkd": "bspwm-exec", "polybar main": "bspwm-exec", "tint2": "openbox-autostart"})

	put(t, home, ".config/plasma-workspace/env/ssh-agent.sh", "eval $(ssh-agent)\n")
	expectKinds(t, kinds(t, kde, home), map[string]string{"~/.config/plasma-workspace/env/ssh-agent.sh": "kde-env-script"})
}

func TestDetect(t *testing.T) {
	home := t.TempDir()
	put(t, home, ".config/hypr/hyprland.lua", "")
	put(t, home, ".config/hypr/hyprland.conf", "")
	put(t, home, ".config/qtile/config.py", "")
	os.MkdirAll(filepath.Join(home, ".config/niri"), 0o755)
	got := Detect(home)
	names := map[string]bool{}
	for _, d := range got {
		names[d.Name] = d.Supported
	}
	if sup, ok := names["Hyprland (Lua)"]; !ok || !sup {
		t.Errorf("Hyprland Lua not detected as supported: %+v", got)
	}
	if sup, ok := names["Qtile"]; !ok || sup {
		t.Errorf("Qtile must be detected as unsupported: %+v", got)
	}
	if _, ok := names["niri"]; ok {
		t.Error("an empty niri dir without config.kdl is not a niri setup")
	}
}
