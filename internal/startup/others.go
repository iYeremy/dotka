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

// ---- XDG autostart --------------------------------------------------------

func xdgAutostart(home string) ([]gen.StartupItem, []Warning) {
	dir := filepath.Join(home, ".config/autostart")
	files, _ := filepath.Glob(filepath.Join(dir, "*.desktop"))
	sort.Strings(files)
	var items []gen.StartupItem
	var warns []Warning
	for _, f := range files {
		lines, err := readLines(f)
		if err != nil {
			warns = append(warns, Warning{rel(home, f), err})
			continue
		}
		var exec string
		line := 0
		hidden, inEntry := false, false
		for i, l := range lines {
			l = strings.TrimSpace(l)
			if strings.HasPrefix(l, "[") {
				inEntry = l == "[Desktop Entry]"
				continue
			}
			if !inEntry {
				continue
			}
			k, v, ok := strings.Cut(l, "=")
			if !ok {
				continue
			}
			switch strings.TrimSpace(k) {
			case "Exec":
				exec, line = strings.TrimSpace(v), i+1
			case "Hidden":
				hidden = hidden || strings.EqualFold(strings.TrimSpace(v), "true")
			case "X-GNOME-Autostart-enabled":
				hidden = hidden || strings.EqualFold(strings.TrimSpace(v), "false")
			}
		}
		if exec == "" || hidden {
			continue
		}
		items = append(items, gen.StartupItem{
			Kind: "xdg-autostart", Command: exec, Source: rel(home, f), Line: line,
			Confidence: Confirmed, Risk: Risk(exec),
		})
	}
	return items, warns
}

// ---- systemd --user -------------------------------------------------------

// systemdUser lists units enabled by the user: symlinks in
// ~/.config/systemd/user/*.wants/. Reading them needs no systemctl call.
func systemdUser(home string) ([]gen.StartupItem, []Warning) {
	base := filepath.Join(home, ".config/systemd/user")
	links, _ := filepath.Glob(filepath.Join(base, "*.wants", "*"))
	sort.Strings(links)
	var items []gen.StartupItem
	for _, l := range links {
		unit := filepath.Base(l)
		wants := strings.TrimSuffix(filepath.Base(filepath.Dir(l)), ".wants")
		cmd := unit
		risk := ""
		if exec := unitExecStart(home, base, l); exec != "" {
			cmd = unit + ": " + exec
			risk = Risk(exec)
		}
		items = append(items, gen.StartupItem{
			Kind: "systemd-user", Command: cmd, Source: rel(home, l) + " (" + wants + ")",
			Confidence: Confirmed, Risk: risk,
		})
	}
	return items, nil
}

// unitExecStart reads ExecStart= from a unit file defined in the user's home.
func unitExecStart(home, base, link string) string {
	candidates := []string{filepath.Join(base, filepath.Base(link))}
	if t, err := os.Readlink(link); err == nil {
		if !filepath.IsAbs(t) {
			t = filepath.Join(filepath.Dir(link), t)
		}
		candidates = append([]string{t}, candidates...)
	}
	for _, c := range candidates {
		if !strings.HasPrefix(c, home+"/") {
			continue // system units: not the user's concern here
		}
		lines, err := readLines(c)
		if err != nil {
			continue
		}
		for _, l := range lines {
			if v, ok := strings.CutPrefix(strings.TrimSpace(l), "ExecStart="); ok {
				return strings.TrimLeft(strings.TrimSpace(v), "-@:+!")
			}
		}
	}
	return ""
}

// ---- Shell rc files -------------------------------------------------------

var shellFiles = []string{".zshenv", ".zprofile", ".zshrc", ".profile", ".bash_profile", ".bashrc", ".xprofile", ".xinitrc"}

var (
	shSource   = regexp.MustCompile(`^(source|\.)\s+\S`)
	shEval     = regexp.MustCompile(`^eval\s`)
	shPlugins  = regexp.MustCompile(`\b(zinit|antigen|zplug|antidote|zplugin|sheldon)\b|oh-my-zsh\.sh|\$ZSH/`)
	shBgOrExec = regexp.MustCompile(`(^exec\s)|(&\s*$)`)
)

func shellRC(home string) ([]gen.StartupItem, []Warning) {
	var items []gen.StartupItem
	var warns []Warning
	for _, f := range shellFiles {
		p := filepath.Join(home, f)
		lines, err := readLines(p)
		if err != nil {
			if !errors.Is(err, fs.ErrNotExist) {
				warns = append(warns, Warning{f, err})
			}
			continue
		}
		isX := f == ".xinitrc" || f == ".xprofile"
		for i, l := range lines {
			t := strings.TrimSpace(l)
			if t == "" || strings.HasPrefix(t, "#") {
				continue
			}
			risk := Risk(t)
			var kind string
			switch {
			case risk == RiskRemoteExec:
				kind = "shell-remote"
			case shPlugins.MatchString(t):
				kind = "shell-plugins"
			case shEval.MatchString(t):
				kind = "shell-eval"
			case shSource.MatchString(t):
				kind = "shell-source"
			case isX && shBgOrExec.MatchString(t):
				kind = "x11-exec"
			case risk == RiskLocalScript:
				kind = "shell-exec"
			default:
				continue
			}
			items = append(items, gen.StartupItem{
				Kind: kind, Command: t, Source: f, Line: i + 1,
				Confidence: Heuristic, Risk: risk,
			})
		}
	}
	return items, warns
}
