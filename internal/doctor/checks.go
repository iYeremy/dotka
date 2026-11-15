package doctor

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/iYeremy/dotka/internal/scan"
	"github.com/iYeremy/dotka/internal/startup"
)

// missingPrograms: startup entries whose program is not installed.
func missingPrograms(in Input) []Finding {
	var out []Finding
	seen := map[string]bool{}
	for _, it := range in.Startup {
		cmd, ok := execPart(it)
		if !ok {
			continue
		}
		prog, _, ok := program(cmd)
		if !ok {
			continue
		}
		if _, found := resolve(in, prog); found {
			continue
		}
		src, line := location(it)
		key := prog + "\x00" + src
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, Finding{
			Severity: Problem, Check: "missing-program", Path: src, Line: line,
			Message: fmt.Sprintf("starts %q, but that program is not installed", prog),
			Hint:    "install it, or remove the line if you no longer use it",
		})
	}
	return out
}

var sudoRe = regexp.MustCompile(`(^|[\s;&|])sudo\s`)

// startupScripts: local scripts run at startup that download and run code
// or call sudo. The script is read, never executed.
func startupScripts(in Input) []Finding {
	var out []Finding
	done := map[string]bool{}
	for _, it := range in.Startup {
		cmd, ok := execPart(it)
		if !ok {
			continue
		}
		prog, args, ok := program(cmd)
		if !ok {
			continue
		}
		// "bash ~/x.sh" runs the script given as argument.
		if (prog == "sh" || prog == "bash" || prog == "zsh") && len(args) > 0 {
			prog = args[0]
		}
		path := expand(in.Home, prog)
		if !strings.HasPrefix(path, in.Home+"/") || done[path] {
			continue
		}
		done[path] = true
		lines, ok := readText(path)
		if !ok {
			continue
		}
		src, line := location(it)
		for i, l := range lines {
			t := strings.TrimSpace(l)
			if t == "" || strings.HasPrefix(t, "#") {
				continue
			}
			switch {
			case startup.Risk(t) == startup.RiskRemoteExec:
				out = append(out, Finding{
					Severity: Problem, Check: "startup-script-remote", Path: rel(in.Home, path), Line: i + 1,
					Message: fmt.Sprintf("downloads and runs code at every login (started from %s:%d): %s", src, line, t),
					Hint:    "read the script; remove it from startup if you do not trust the source",
				})
			case sudoRe.MatchString(t):
				out = append(out, Finding{
					Severity: Problem, Check: "startup-script-sudo", Path: rel(in.Home, path), Line: i + 1,
					Message: fmt.Sprintf("uses sudo at startup (started from %s:%d): %s", src, line, t),
					Hint:    "a login script should not need root; review it",
				})
			}
		}
	}
	return out
}

// readText returns the lines of a small text file.
func readText(path string) ([]string, bool) {
	f, err := os.Open(path)
	if err != nil {
		return nil, false
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, 1<<20))
	if err != nil || bytes.IndexByte(b, 0) >= 0 {
		return nil, false
	}
	return strings.Split(string(b), "\n"), true
}

// brokenSymlinks in tracked roots.
func brokenSymlinks(in Input) []Finding {
	var out []Finding
	for _, e := range in.Entries {
		if e.Type != scan.TypeSymlink {
			continue
		}
		t := e.Target
		if !filepath.IsAbs(t) {
			t = filepath.Join(in.Home, filepath.Dir(e.Path), t)
		}
		if _, err := os.Stat(t); err == nil {
			continue
		}
		out = append(out, Finding{
			Severity: Problem, Check: "broken-symlink", Path: e.Path,
			Message: fmt.Sprintf("broken symlink: points to %s, which does not exist", e.Target),
			Hint:    "remove it, or restore the file it pointed to (a deleted dotfiles repo?)",
		})
	}
	return out
}

// exposedSecrets: credential files readable by other users.
func exposedSecrets(in Input) []Finding {
	var out []Finding
	for _, e := range in.Entries {
		if e.Skip != scan.SkipSecret || e.Mode&0o077 == 0 {
			continue
		}
		out = append(out, Finding{
			Severity: Problem, Check: "exposed-secret", Path: e.Path,
			Message: fmt.Sprintf("looks like a secret (%s) but is readable by other users (mode %o)", e.Reason, e.Mode),
			Hint:    "chmod 600 ~/" + e.Path,
		})
	}
	return out
}

// duplicateStartup: the same program started from several places.
func duplicateStartup(in Input) []Finding {
	byProg := map[string][]string{}
	var order []string
	for _, it := range in.Startup {
		cmd, ok := execPart(it)
		if !ok || strings.HasPrefix(it.Kind, "hypr-on:") {
			continue
		}
		if it.Kind == "hypr-exec" || it.Kind == "hypr-execr" {
			continue // runs on every reload by design
		}
		key := strings.Join(strings.Fields(cmd), " ")
		src, line := location(it)
		where := src
		if line > 0 {
			where = fmt.Sprintf("%s:%d", src, line)
		}
		if _, ok := byProg[key]; !ok {
			order = append(order, key)
		}
		byProg[key] = append(byProg[key], where)
	}
	var out []Finding
	for _, k := range order {
		if w := byProg[k]; len(w) > 1 {
			out = append(out, Finding{
				Severity: Note, Check: "duplicate-startup", Path: w[0],
				Message: fmt.Sprintf("%q is started %d times: %s", k, len(w), strings.Join(w, ", ")),
				Hint:    "keep only one of them, or the program may run twice",
			})
		}
	}
	return out
}

// systemConfigDirs are ~/.config entries that belong to the desktop or to
// libraries rather than to an app you install by name.
var systemConfigDirs = map[string]bool{
	"autostart": true, "systemd": true, "environment.d": true, "fontconfig": true,
	"gtk-2.0": true, "gtk-3.0": true, "gtk-4.0": true, "dconf": true, "pulse": true,
	"menus": true, "xsettingsd": true, "procps": true, "ibus": true, "enchant": true,
	"user-tmpfiles.d": true, "dbus-1": true, "gnome-session": true, "trolltech": true,
	"session": true, "configstore": true, "go": true, "ui": true, "xdg-desktop-portal": true,
	"mimeapps.list": true, "qt5ct": true, "qt6ct": true, "dotka": true, "bash": true, "zsh": true,
	"shell": true, "nautilus": true, "electron": true, "matplotlib": true, "pip": true,
	"xkb": true, "unknown organization": true, "kdedefaults": true, "kde.org": true,
}

// leftoverConfigs: ~/.config/<name> dirs with no matching program or
// package. Heuristic, reported as notes.
func leftoverConfigs(in Input) []Finding {
	if in.Packages == nil {
		return nil // without the package list the guess is too noisy
	}
	pkgs := make([]string, 0, len(in.Packages))
	for _, p := range in.Packages {
		pkgs = append(pkgs, strings.ToLower(p))
	}
	var names []string
	for _, e := range in.Entries {
		if e.Type != scan.TypeDir || !strings.HasPrefix(e.Path, ".config/") || strings.Count(e.Path, "/") != 1 {
			continue
		}
		names = append(names, strings.TrimPrefix(e.Path, ".config/"))
	}
	sort.Strings(names)
	var out []Finding
	for _, n := range names {
		l := strings.ToLower(n)
		if systemConfigDirs[l] || strings.HasPrefix(l, ".") {
			continue
		}
		if _, found := resolve(in, l); found {
			continue
		}
		if matchesPackage(l, pkgs) {
			continue
		}
		out = append(out, Finding{
			Severity: Note, Check: "leftover-config", Path: ".config/" + n,
			Message: "no installed program or package matches this config (left over from an uninstalled app?)",
			Hint:    "if you no longer use it, it can be removed (heuristic: double-check first)",
		})
	}
	return out
}

func matchesPackage(name string, pkgs []string) bool {
	for _, p := range pkgs {
		if p == name || strings.HasPrefix(p, name) || strings.HasPrefix(name, p+"-") ||
			strings.TrimSuffix(strings.TrimSuffix(p, "-git"), "-bin") == name {
			return true
		}
	}
	return false
}
