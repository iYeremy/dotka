// Package startup builds the "startup map": everything that runs
// automatically in the user's session or shell, with the file and line it
// comes from. It only reads files; nothing is ever executed.
package startup

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/iYeremy/dotka/internal/gen"
)

// Confidence levels.
const (
	Confirmed = "confirmed" // known syntax with clear semantics
	Heuristic = "heuristic" // pattern match in code (Lua, shell)
)

// Risks.
const (
	RiskRemoteExec  = "remote-exec"  // downloads and runs code
	RiskLocalScript = "local-script" // runs a script from $HOME
)

// Warning is a non-fatal extraction problem.
type Warning struct {
	Path string
	Err  error
}

type extractor func(home string) ([]gen.StartupItem, []Warning)

var extractors = []extractor{hyprConf, hyprLua, xdgAutostart, systemdUser, shellRC}

// Extract runs every extractor against home.
func Extract(home string) ([]gen.StartupItem, []Warning) {
	var items []gen.StartupItem
	var warns []Warning
	seen := map[string]bool{}
	for _, ex := range extractors {
		it, w := ex(home)
		warns = append(warns, w...)
		for _, i := range it {
			k := i.Key()
			if seen[k] {
				continue
			}
			seen[k] = true
			items = append(items, i)
		}
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Source != items[j].Source {
			return items[i].Source < items[j].Source
		}
		return items[i].Line < items[j].Line
	})
	return items, warns
}

var (
	remotePipe   = regexp.MustCompile(`\b(curl|wget|fetch)\b[^|]*\|\s*(sudo\s+)?(env\s+)?(ba|z|da|fi)?sh\b`)
	remoteSubst  = regexp.MustCompile(`(\$\(|<\(|` + "`" + `)\s*(curl|wget)\b`)
	remoteEval   = regexp.MustCompile(`\b(eval|source|\.)\s+.*\b(curl|wget)\b`)
	localScriptR = regexp.MustCompile(`^(sh\s+|bash\s+|zsh\s+)?["']?(~/|\$HOME/|\$\{HOME\}/|%h/|/home/|\./)`)
)

// Risk classifies a command string.
func Risk(cmd string) string {
	c := strings.TrimSpace(cmd)
	if remotePipe.MatchString(c) || remoteSubst.MatchString(c) || remoteEval.MatchString(c) {
		return RiskRemoteExec
	}
	if localScriptR.MatchString(c) {
		return RiskLocalScript
	}
	return ""
}

// rel returns a home-relative path when p is inside home.
func rel(home, p string) string {
	if r, err := filepath.Rel(home, p); err == nil && !strings.HasPrefix(r, "..") {
		return r
	}
	return p
}

// expandHome expands ~ and $HOME in a path.
func expandHome(home, p string) string {
	p = strings.TrimSpace(strings.Trim(strings.TrimSpace(p), `"'`))
	switch {
	case p == "~":
		return home
	case strings.HasPrefix(p, "~/"):
		return filepath.Join(home, p[2:])
	}
	p = strings.ReplaceAll(p, "${HOME}", home)
	return strings.ReplaceAll(p, "$HOME", home)
}

func readLines(path string) ([]string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return strings.Split(string(b), "\n"), nil
}
