// Command dotka is a time machine for your Linux user environment.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/iYeremy/dotka/internal/config"
	"github.com/iYeremy/dotka/internal/gen"
	"github.com/iYeremy/dotka/internal/store"
)

var version = "0.1.0-dev"

const usage = `dotka - a time machine for your Linux user environment

Usage:
  dotka [--home DIR] [--store DIR] <command> [args]

Commands:
  init                     create the store and generation #1
  snap [-m MSG]            save a new generation
  status                   what changed since the current generation
  diff [A [B]] [--path P] [--patch]
                           compare generations (default: HEAD vs now)
  run [-m MSG] -- CMD...    snapshot, run CMD, snapshot, and show what CMD changed
  why THING                where a file, package or startup item came from
  rollback N [--dry-run] [--yes] [--keep-new]
                           restore generation N (reversible)
  log                      list generations
  show N                   details of a generation
  startup                  everything that runs at login / shell start
  doctor                   health check: missing programs, risky startup
                           scripts, broken links, exposed secrets, leftovers
  track [ls|add|rm] [PATH] manage tracked roots
  secrets [ls|allow|deny] [PATH]
                           files excluded because they look like secrets
  forget N [N...]          delete save points you no longer need (frees space)
  prune [--keep 10]        forget all but the most recent generations
  gc                       maintenance: delete stored copies no generation uses
  version

Environment:
  DOTKA_PACKAGES         pacman (default) | off | file:PATH (fake list for demos)
  DOTKA_ALLOW_REAL_HOME  set to 1 to allow write commands on your real home
                         (blocked by default while dotka is a prototype)

Global options (also DOTKA_HOME / DOTKA_STORE):
  --home DIR    home directory to manage (default: $HOME)
  --store DIR   store directory (default: $XDG_DATA_HOME/dotka or HOME/.local/share/dotka)
`

// env is the resolved environment for a command.
type env struct {
	home     string
	storeDir string
	// realHome is true when home is the user's actual home directory.
	realHome bool
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "dotka:", err)
		var ee exitError
		if errors.As(err, &ee) {
			os.Exit(ee.code)
		}
		os.Exit(1)
	}
}

func run(args []string) error {
	g := flag.NewFlagSet("dotka", flag.ContinueOnError)
	g.Usage = func() { fmt.Fprint(os.Stderr, usage) }
	homeFlag := g.String("home", os.Getenv("DOTKA_HOME"), "")
	storeFlag := g.String("store", os.Getenv("DOTKA_STORE"), "")
	if err := g.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	rest := g.Args()
	if len(rest) == 0 {
		fmt.Print(usage)
		return nil
	}
	e, err := resolveEnv(*homeFlag, *storeFlag)
	if err != nil {
		return err
	}
	cmd, cargs := rest[0], rest[1:]
	if err := guardRealHome(e, cmd, cargs); err != nil {
		return err
	}
	switch cmd {
	case "init":
		return cmdInit(e, cargs)
	case "snap", "snapshot":
		return cmdSnap(e, cargs)
	case "status", "st":
		return cmdStatus(e, cargs)
	case "diff":
		return cmdDiff(e, cargs)
	case "rollback":
		return cmdRollback(e, cargs)
	case "run":
		return cmdRun(e, cargs)
	case "why":
		return cmdWhy(e, cargs)
	case "log":
		return cmdLog(e, cargs)
	case "show":
		return cmdShow(e, cargs)
	case "startup":
		return cmdStartup(e, cargs)
	case "doctor":
		return cmdDoctor(e, cargs)
	case "track":
		return cmdTrack(e, cargs)
	case "secrets":
		return cmdSecrets(e, cargs)
	case "forget":
		return cmdForget(e, cargs)
	case "prune":
		return cmdPrune(e, cargs)
	case "gc":
		return cmdGC(e, cargs)
	case "version", "--version":
		fmt.Println("dotka", version)
		return nil
	case "help", "-h", "--help":
		fmt.Print(usage)
		return nil
	}
	return fmt.Errorf("unknown command %q (see `dotka help`)", cmd)
}

func resolveEnv(home, storeDir string) (env, error) {
	explicitHome := home != ""
	if home == "" {
		h, err := os.UserHomeDir()
		if err != nil {
			return env{}, err
		}
		home = h
	}
	home, err := filepath.Abs(home)
	if err != nil {
		return env{}, err
	}
	if fi, err := os.Stat(home); err != nil || !fi.IsDir() {
		return env{}, fmt.Errorf("home %q is not a directory", home)
	}
	if storeDir == "" {
		if x := os.Getenv("XDG_DATA_HOME"); x != "" && !explicitHome {
			storeDir = filepath.Join(x, "dotka")
		} else {
			storeDir = filepath.Join(home, ".local/share/dotka")
		}
	}
	storeDir, err = filepath.Abs(storeDir)
	if err != nil {
		return env{}, err
	}
	real := false
	if h, err := os.UserHomeDir(); err == nil {
		real = sameDir(h, home)
	}
	return env{home: home, storeDir: storeDir, realHome: real}, nil
}

func sameDir(a, b string) bool {
	ra, err1 := filepath.EvalSymlinks(a)
	rb, err2 := filepath.EvalSymlinks(b)
	if err1 != nil || err2 != nil {
		return filepath.Clean(a) == filepath.Clean(b)
	}
	return ra == rb
}

// writes reports whether a command writes to the store or the home.
func writes(cmd string, args []string) bool {
	switch cmd {
	case "init", "snap", "snapshot", "rollback", "gc", "forget", "prune", "run":
		return true
	case "track", "secrets":
		return len(args) > 0 && args[0] != "ls" && args[0] != "list"
	}
	return false
}

// guardRealHome stops write commands on the user's real home while Dotka is
// a prototype, unless explicitly allowed. It protects against forgetting to
// point dotka at the sandbox.
func guardRealHome(e env, cmd string, args []string) error {
	if !e.realHome || !writes(cmd, args) || os.Getenv("DOTKA_ALLOW_REAL_HOME") == "1" {
		return nil
	}
	return fmt.Errorf(`refusing to run %q on your real home (%s).

Dotka is a prototype. Try it on the sandbox instead:
  scripts/sandbox.sh setup && eval "$(scripts/sandbox.sh env)"

Read-only commands (startup, status, log, show, diff) work anywhere.
To use your real home anyway, set DOTKA_ALLOW_REAL_HOME=1.`, cmd, e.home)
}

// manager opens the store and config and returns a ready Manager.
func (e env) manager() (*gen.Manager, error) {
	s, err := store.Open(e.storeDir)
	if err != nil {
		return nil, err
	}
	cfg, err := config.Load(e.storeDir)
	if err != nil {
		return nil, err
	}
	return e.managerWith(s, cfg), nil
}

func (e env) managerWith(s *store.Store, cfg config.Config) *gen.Manager {
	// Never scan our own store, wherever it lives.
	if rel, err := filepath.Rel(e.home, e.storeDir); err == nil && !strings.HasPrefix(rel, "..") {
		cfg.Exclude = append(cfg.Exclude, rel)
	}
	return &gen.Manager{Home: e.home, Store: s, Config: cfg, Collector: collector}
}

var lookPath = exec.LookPath

// collector gathers packages and the startup map (see collect.go).
var collector gen.Collector

// homeRel converts a user-supplied path (absolute, ~/..., or home-relative)
// into a path relative to home.
func (e env) homeRel(p string) (string, error) {
	switch {
	case p == "~":
		return "", fmt.Errorf("refusing to track the whole home")
	case strings.HasPrefix(p, "~/"):
		p = p[2:]
	case filepath.IsAbs(p):
		r, err := filepath.Rel(e.home, p)
		if err != nil || strings.HasPrefix(r, "..") {
			return "", fmt.Errorf("%s is outside %s", p, e.home)
		}
		p = r
	}
	return filepath.Clean(p), nil
}

// interspersed parses flags that may appear after positional arguments.
func interspersed(fs *flag.FlagSet, args []string) ([]string, error) {
	var pos []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		args = fs.Args()
		if len(args) == 0 {
			return pos, nil
		}
		pos = append(pos, args[0])
		args = args[1:]
	}
}
