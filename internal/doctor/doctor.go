// Package doctor runs read-only health checks on a user environment:
// startup commands whose program is missing, startup scripts that download
// and run code, broken symlinks, exposed secrets, duplicated startup entries
// and leftover configs of apps that seem uninstalled.
package doctor

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/iYeremy/dotka/internal/gen"
	"github.com/iYeremy/dotka/internal/scan"
)

// Severities.
const (
	Problem = "problem"
	Note    = "note"
)

// Finding is one result of a check.
type Finding struct {
	Severity string
	Check    string // short id, e.g. "missing-program"
	Path     string // file the finding is about (home-relative when possible)
	Line     int
	Message  string
	Hint     string // what the user can do
}

// Input is everything the checks need. It is gathered by the caller so the
// checks stay pure and testable.
type Input struct {
	Home    string
	Entries []scan.Entry      // tracked files (metadata only is enough)
	Startup []gen.StartupItem // startup map
	// Packages lists every installed package; nil when unknown.
	Packages []string
	// LookPath finds a program in $PATH (exec.LookPath in production).
	LookPath func(string) (string, error)
	// XDGConfigHome is the value of $XDG_CONFIG_HOME ("" if unset).
	XDGConfigHome string
}

type check func(Input) []Finding

var checks = []check{missingPrograms, startupScripts, brokenSymlinks, exposedSecrets, duplicateStartup, leftoverConfigs, unsupportedWMs, xdgConfigHome}

// Run executes every check and returns findings, problems first.
func Run(in Input) []Finding {
	var out []Finding
	for _, c := range checks {
		out = append(out, c(in)...)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Severity != out[j].Severity {
			return out[i].Severity == Problem
		}
		return false
	})
	return out
}

// ---- helpers --------------------------------------------------------------

// splitArgs splits a command line into words, honoring simple quotes.
func splitArgs(s string) []string {
	var out []string
	var cur strings.Builder
	var quote rune
	inWord := false
	for _, r := range s {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			} else {
				cur.WriteRune(r)
			}
		case r == '"' || r == '\'':
			quote, inWord = r, true
		case r == ' ' || r == '\t':
			if inWord {
				out = append(out, cur.String())
				cur.Reset()
				inWord = false
			}
		default:
			cur.WriteRune(r)
			inWord = true
		}
	}
	if inWord {
		out = append(out, cur.String())
	}
	return out
}

// wrappers are launchers whose real program is a later argument.
var wrappers = map[string]bool{
	"exec": true, "nohup": true, "setsid": true, "env": true, "uwsm-app": true,
	"app2unit": true, "runapp": true,
}

// program extracts the program a startup command runs. ok is false when it
// cannot be determined statically (variables, subshells, Lua expressions).
func program(cmd string) (prog string, args []string, ok bool) {
	w := splitArgs(cmd)
	for len(w) > 0 {
		head := w[0]
		switch {
		case strings.Contains(head, "=") && !strings.HasPrefix(head, "/") && !strings.HasPrefix(head, "~"):
			w = w[1:] // VAR=value
			continue
		case wrappers[head]:
			w = w[1:]
			for len(w) > 0 && strings.HasPrefix(w[0], "-") {
				w = w[1:]
			}
			continue
		case head == "uwsm" && len(w) > 2 && w[1] == "app":
			w = w[2:]
			if len(w) > 0 && w[0] == "--" {
				w = w[1:]
			}
			continue
		case (head == "sh" || head == "bash" || head == "zsh") && len(w) > 2 && w[1] == "-c":
			return program(w[2])
		}
		break
	}
	if len(w) == 0 {
		return "", nil, false
	}
	p := w[0]
	if strings.ContainsAny(p, "$`(){}|;&<>*") && !strings.HasPrefix(p, "$HOME/") && !strings.HasPrefix(p, "${HOME}/") {
		return "", nil, false
	}
	return p, w[1:], true
}

// expand resolves ~, $HOME and the systemd %h specifier.
func expand(home, p string) string {
	switch {
	case strings.HasPrefix(p, "~/"):
		return filepath.Join(home, p[2:])
	case strings.HasPrefix(p, "$HOME/"):
		return filepath.Join(home, p[6:])
	case strings.HasPrefix(p, "${HOME}/"):
		return filepath.Join(home, p[8:])
	case strings.HasPrefix(p, "%h/"):
		return filepath.Join(home, p[3:])
	}
	return p
}

// resolve reports whether prog exists. Paths are checked on disk; bare names
// in $PATH and in the user's ~/.local/bin and ~/bin.
func resolve(in Input, prog string) (string, bool) {
	if strings.Contains(prog, "/") {
		p := expand(in.Home, prog)
		if !filepath.IsAbs(p) {
			return p, true // relative to an unknown cwd: do not guess
		}
		fi, err := os.Stat(p)
		return p, err == nil && !fi.IsDir()
	}
	for _, dir := range []string{".local/bin", "bin"} {
		p := filepath.Join(in.Home, dir, prog)
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			return p, true
		}
	}
	if in.LookPath != nil {
		if p, err := in.LookPath(prog); err == nil {
			return p, true
		}
	}
	return prog, false
}

// execPart returns the command of a startup item that actually runs.
func execPart(it gen.StartupItem) (string, bool) {
	switch {
	case it.Dynamic:
		return "", false
	case it.Kind == "systemd-user":
		_, cmd, ok := strings.Cut(it.Command, ": ")
		return cmd, ok
	case strings.HasPrefix(it.Kind, "hypr"), it.Kind == "xdg-autostart", it.Kind == "x11-exec", it.Kind == "shell-exec":
		return it.Command, true
	}
	return "", false // shell source/eval/plugins: not a program launch
}

func rel(home, p string) string {
	if r, err := filepath.Rel(home, p); err == nil && !strings.HasPrefix(r, "..") {
		return r
	}
	return p
}

func location(it gen.StartupItem) (string, int) {
	src := it.Source
	if i := strings.Index(src, " ("); i > 0 {
		src = src[:i]
	}
	return src, it.Line
}
