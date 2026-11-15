package startup

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/iYeremy/dotka/internal/gen"
)

// ---- sway / i3 ---------------------------------------------------------------

var (
	i3Exec    = regexp.MustCompile(`^\s*(exec_always|exec)\s+(?:--no-startup-id\s+)?(.+?)\s*$`)
	i3Include = regexp.MustCompile(`^\s*include\s+(.+?)\s*$`)
)

func sway(home string) ([]gen.StartupItem, []Warning) {
	return i3Like(home, "sway", []string{".config/sway/config", ".sway/config"})
}

func i3(home string) ([]gen.StartupItem, []Warning) {
	return i3Like(home, "i3", []string{".config/i3/config", ".i3/config"})
}

// i3Like reads the sway/i3 config format: "exec" runs once at startup,
// "exec_always" on every reload. Keybindings ("bindsym ... exec") are not
// startup items: only lines that start with exec count.
func i3Like(home, wm string, candidates []string) ([]gen.StartupItem, []Warning) {
	var items []gen.StartupItem
	var warns []Warning
	visited := map[string]bool{}
	var walk func(path string, depth int)
	walk = func(path string, depth int) {
		if visited[path] || depth > maxSourceDepth {
			return
		}
		visited[path] = true
		lines, err := readLines(path)
		if err != nil {
			if !errors.Is(err, fs.ErrNotExist) {
				warns = append(warns, Warning{rel(home, path), err})
			}
			return
		}
		for i, line := range lines {
			line = stripHashComment(line)
			if m := i3Exec.FindStringSubmatch(line); m != nil {
				kind := wm + "-exec"
				if m[1] == "exec_always" {
					kind = wm + "-exec-always"
				}
				items = append(items, gen.StartupItem{Kind: kind, Command: m[2], Source: rel(home, path),
					Line: i + 1, Confidence: Confirmed, Risk: Risk(m[2])})
				continue
			}
			if m := i3Include.FindStringSubmatch(line); m != nil {
				p := expandHome(home, m[1])
				if !filepath.IsAbs(p) {
					p = filepath.Join(filepath.Dir(path), p)
				}
				matches, _ := filepath.Glob(p)
				for _, mp := range matches {
					walk(mp, depth+1)
				}
			}
		}
	}
	for _, c := range candidates {
		p := filepath.Join(home, c)
		if _, err := os.Stat(p); err == nil {
			walk(p, 0)
			break // the first existing config wins, like the WM itself
		}
	}
	return items, warns
}

// ---- niri ------------------------------------------------------------------------

var (
	niriSpawn   = regexp.MustCompile(`^\s*spawn-at-startup\s+(.+?)\s*$`)
	niriSpawnSh = regexp.MustCompile(`^\s*spawn-sh-at-startup\s+(.+?)\s*$`)
	kdlString   = regexp.MustCompile(`"((?:[^"\\]|\\.)*)"`)
)

// niri: spawn-at-startup "prog" "arg"...  /  spawn-sh-at-startup "cmd line"
func niri(home string) ([]gen.StartupItem, []Warning) {
	path := filepath.Join(home, ".config/niri/config.kdl")
	lines, err := readLines(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, []Warning{{rel(home, path), err}}
	}
	var items []gen.StartupItem
	inBlockComment := false
	for i, line := range lines {
		t := strings.TrimSpace(line)
		if inBlockComment {
			if strings.Contains(t, "*/") {
				inBlockComment = false
			}
			continue
		}
		if strings.HasPrefix(t, "/*") {
			inBlockComment = !strings.Contains(t, "*/")
			continue
		}
		if strings.HasPrefix(t, "//") || strings.HasPrefix(t, "/-") {
			continue // comment, or KDL "slashdash" (commented-out node)
		}
		var args []string
		if m := niriSpawn.FindStringSubmatch(line); m != nil {
			for _, s := range kdlString.FindAllStringSubmatch(m[1], -1) {
				args = append(args, s[1])
			}
		} else if m := niriSpawnSh.FindStringSubmatch(line); m != nil {
			if s := kdlString.FindStringSubmatch(m[1]); s != nil {
				args = []string{s[1]}
			}
		}
		if len(args) == 0 {
			continue
		}
		cmd := strings.Join(args, " ")
		items = append(items, gen.StartupItem{Kind: "niri-startup", Command: cmd, Source: rel(home, path),
			Line: i + 1, Confidence: Confirmed, Risk: Risk(cmd)})
	}
	return items, nil
}

// ---- river -------------------------------------------------------------------------

var riverSpawn = regexp.MustCompile(`^\s*riverctl\s+spawn\s+(.+?)\s*$`)

// river: the init file is a script; "riverctl spawn <cmd>" at top level
// starts programs ("riverctl map ... spawn" is a keybinding, not matched).
func river(home string) ([]gen.StartupItem, []Warning) {
	path := filepath.Join(home, ".config/river/init")
	lines, err := readLines(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, []Warning{{rel(home, path), err}}
	}
	var items []gen.StartupItem
	for i, line := range lines {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "#") {
			continue
		}
		m := riverSpawn.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		cmd := strings.Trim(m[1], `"'`)
		items = append(items, gen.StartupItem{Kind: "river-spawn", Command: cmd, Source: rel(home, path),
			Line: i + 1, Confidence: Confirmed, Risk: Risk(cmd)})
	}
	return items, nil
}

// ---- Wayfire -------------------------------------------------------------------------

// wayfire: [autostart] section of wayfire.ini, "name = command".
func wayfire(home string) ([]gen.StartupItem, []Warning) {
	path := filepath.Join(home, ".config/wayfire.ini")
	lines, err := readLines(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, []Warning{{rel(home, path), err}}
	}
	var items []gen.StartupItem
	in := false
	for i, line := range lines {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "[") {
			in = t == "[autostart]"
			continue
		}
		if !in || t == "" || strings.HasPrefix(t, "#") {
			continue
		}
		k, v, ok := strings.Cut(t, "=")
		if !ok {
			continue
		}
		v = strings.TrimSpace(v)
		if strings.HasPrefix(strings.TrimSpace(k), "autostart_") || v == "true" || v == "false" {
			continue // options, not commands
		}
		items = append(items, gen.StartupItem{Kind: "wayfire-autostart", Command: v, Source: rel(home, path),
			Line: i + 1, Confidence: Confirmed, Risk: Risk(v)})
	}
	return items, nil
}

// ---- autostart shell scripts (bspwm, Openbox, labwc, herbstluftwm) -------------

var autostartScripts = []struct{ kind, path string }{
	{"bspwm-exec", ".config/bspwm/bspwmrc"},
	{"openbox-autostart", ".config/openbox/autostart"},
	{"labwc-autostart", ".config/labwc/autostart"},
	{"herbstluft-exec", ".config/herbstluftwm/autostart"},
}

var bgOrExec = regexp.MustCompile(`(^exec\s)|(&\s*$)`)

// scripts: lines that start something in the background ("cmd &") or
// "exec cmd". WM control commands (bspc, herbstclient) are skipped.
func scripts(home string) ([]gen.StartupItem, []Warning) {
	var items []gen.StartupItem
	var warns []Warning
	for _, s := range autostartScripts {
		path := filepath.Join(home, s.path)
		lines, err := readLines(path)
		if err != nil {
			if !errors.Is(err, fs.ErrNotExist) {
				warns = append(warns, Warning{s.path, err})
			}
			continue
		}
		for i, line := range lines {
			t := strings.TrimSpace(line)
			if t == "" || strings.HasPrefix(t, "#") || !bgOrExec.MatchString(t) {
				continue
			}
			if strings.HasPrefix(t, "bspc ") || strings.HasPrefix(t, "herbstclient ") || strings.HasPrefix(t, "hc ") {
				continue
			}
			cmd := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(t, "exec "), "&"))
			items = append(items, gen.StartupItem{Kind: s.kind, Command: cmd, Source: s.path,
				Line: i + 1, Confidence: Heuristic, Risk: Risk(cmd)})
		}
	}
	return items, warns
}

// ---- KDE Plasma ----------------------------------------------------------------------

// kde: scripts in plasma-workspace/env are sourced at the start of every
// Plasma session. (Regular autostart apps are XDG entries, handled apart.)
func kde(home string) ([]gen.StartupItem, []Warning) {
	files, _ := filepath.Glob(filepath.Join(home, ".config/plasma-workspace/env/*.sh"))
	sort.Strings(files)
	var items []gen.StartupItem
	for _, f := range files {
		items = append(items, gen.StartupItem{Kind: "kde-env-script", Command: "~/" + rel(home, f),
			Source: rel(home, f), Confidence: Confirmed, Risk: RiskLocalScript})
	}
	return items, nil
}

// ---- detection --------------------------------------------------------------------------

// Desktop is a window manager / desktop config found in the home.
type Desktop struct {
	Name      string
	Path      string // home-relative config path
	Supported bool   // dotka can read its autostart
}

var knownDesktops = []Desktop{
	{"Hyprland (Lua)", ".config/hypr/hyprland.lua", true},
	{"Hyprland", ".config/hypr/hyprland.conf", true},
	{"sway", ".config/sway/config", true},
	{"i3", ".config/i3/config", true},
	{"i3", ".i3/config", true},
	{"niri", ".config/niri/config.kdl", true},
	{"river", ".config/river/init", true},
	{"Wayfire", ".config/wayfire.ini", true},
	{"bspwm", ".config/bspwm/bspwmrc", true},
	{"Openbox", ".config/openbox/autostart", true},
	{"labwc", ".config/labwc/autostart", true},
	{"herbstluftwm", ".config/herbstluftwm/autostart", true},
	{"KDE Plasma", ".config/plasma-workspace", true},
	// Configs that are real code: their autostart is not understood yet.
	{"Qtile", ".config/qtile/config.py", false},
	{"awesome", ".config/awesome/rc.lua", false},
	{"xmonad", ".config/xmonad/xmonad.hs", false},
	{"xmonad", ".xmonad/xmonad.hs", false},
	{"dwl/dwm", ".config/dwl", false},
}

// Detect lists the window managers / desktops whose config exists.
func Detect(home string) []Desktop {
	var out []Desktop
	seen := map[string]bool{}
	for _, d := range knownDesktops {
		if seen[d.Name] {
			continue
		}
		if _, err := os.Stat(filepath.Join(home, d.Path)); err == nil {
			seen[d.Name] = true
			out = append(out, d)
		}
	}
	return out
}

// groupOrder is the display order of startup groups.
var groupOrder = []string{
	"Hyprland", "sway", "i3", "niri", "river", "Wayfire", "bspwm", "Openbox", "labwc", "herbstluftwm", "KDE Plasma",
	"Desktop autostart", "User services", "Shell", "X11 session",
}

// Group returns the display group of a startup item kind.
func Group(kind string) string {
	prefixes := []struct{ p, g string }{
		{"hypr", "Hyprland"}, {"sway", "sway"}, {"i3", "i3"}, {"niri", "niri"}, {"river", "river"},
		{"wayfire", "Wayfire"}, {"bspwm", "bspwm"}, {"openbox", "Openbox"}, {"labwc", "labwc"},
		{"herbstluft", "herbstluftwm"}, {"kde", "KDE Plasma"}, {"xdg", "Desktop autostart"},
		{"systemd", "User services"}, {"x11", "X11 session"},
	}
	for _, x := range prefixes {
		if strings.HasPrefix(kind, x.p) {
			return x.g
		}
	}
	return "Shell"
}

// Groups returns the group names in display order.
func Groups() []string { return groupOrder }
