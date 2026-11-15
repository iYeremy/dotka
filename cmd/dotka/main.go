// Command dotka is a time machine for your Linux user environment.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/iYeremy/dotka/internal/config"
	"github.com/iYeremy/dotka/internal/gen"
	"github.com/iYeremy/dotka/internal/store"
)

var version = "0.1.0-dev"

const usage = `dotka — a time machine for your Linux user environment

Usage:
  dotka [--home DIR] [--store DIR] <command> [args]

Commands:
  init                     create the store and generation #1
  snap [-m MSG]            save a new generation
  status                   what changed since the current generation
  diff [A [B]] [--path P] [--patch]
                           compare generations (default: HEAD vs now)
  log                      list generations
  show N                   details of a generation
  track [ls|add|rm] [PATH] manage tracked roots
  secrets [ls|allow|deny] [PATH]
                           files excluded because they look like secrets
  gc                       remove unreferenced objects from the store
  version

Global options (also DOTKA_HOME / DOTKA_STORE):
  --home DIR    home directory to manage (default: $HOME)
  --store DIR   store directory (default: $XDG_DATA_HOME/dotka or HOME/.local/share/dotka)
`

// env is the resolved environment for a command.
type env struct {
	home     string
	storeDir string
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "dotka:", err)
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
	switch cmd {
	case "init":
		return cmdInit(e, cargs)
	case "snap", "snapshot":
		return cmdSnap(e, cargs)
	case "status", "st":
		return cmdStatus(e, cargs)
	case "diff":
		return cmdDiff(e, cargs)
	case "log":
		return cmdLog(e, cargs)
	case "show":
		return cmdShow(e, cargs)
	case "track":
		return cmdTrack(e, cargs)
	case "secrets":
		return cmdSecrets(e, cargs)
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
	return env{home: home, storeDir: storeDir}, nil
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

// collector gathers packages and the startup map. Set in collect.go.
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
