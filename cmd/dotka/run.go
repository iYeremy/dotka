package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/iYeremy/dotka/internal/gen"
	"github.com/iYeremy/dotka/internal/scan"
	"github.com/iYeremy/dotka/internal/ui"
)

// exitError makes dotka exit with the status of a command it ran.
type exitError struct{ code int }

func (e exitError) Error() string { return fmt.Sprintf("command exited with status %d", e.code) }

// cmdRun snapshots, runs a command chosen by the user, snapshots again and
// shows what the command changed. The command runs for real (no sandbox).
func cmdRun(e env, args []string) error {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	msg := fs.String("m", "", "message for the resulting generation")
	if err := fs.Parse(args); err != nil {
		return err
	}
	command := fs.Args()
	if len(command) == 0 {
		return fmt.Errorf("usage: dotka run [-m MSG] -- COMMAND [ARGS...]")
	}
	m, err := e.manager()
	if err != nil {
		return err
	}
	unlock, err := m.Store.Lock()
	if err != nil {
		return err
	}
	defer unlock()

	dir, _ := os.Getwd()
	cmdline := (gen.RunInfo{Command: command, Dir: dir}).Display()
	before, err := m.Store.Head()
	if err != nil {
		return err
	}
	unsaved, err := m.Unsaved()
	if err != nil {
		return err
	}
	if unsaved {
		g, _, err := m.Snapshot("before: "+cmdline, gen.OriginPreRun)
		if err != nil {
			return err
		}
		before = g.ID
		fmt.Printf("%s unsaved changes saved as #%d before running.\n", ui.Dim("note:"), g.ID)
	}
	base, err := m.Load(before)
	if err != nil {
		return err
	}

	fmt.Printf("%s %s\n", ui.Bold("Running:"), strings.Join(command, " "))
	if !e.realHome {
		fmt.Printf("%s HOME=%s\n", ui.Dim("with"), e.home)
	}
	fmt.Println(ui.Dim("---------------------------------------------------------------"))
	c := exec.Command(command[0], command[1:]...)
	c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
	c.Env = childEnv(e)
	code := 0
	if err := c.Run(); err != nil {
		var ee *exec.ExitError
		if !errors.As(err, &ee) {
			return fmt.Errorf("could not run %q: %w", command[0], err)
		}
		code = ee.ExitCode()
	}
	fmt.Println(ui.Dim("---------------------------------------------------------------"))

	message := *msg
	if message == "" {
		message = cmdline
	}
	after, warns, err := m.SnapshotRun(message, gen.OriginRun,
		&gen.RunInfo{Command: command, Dir: dir, ExitCode: code, Before: before})
	if err != nil {
		return err
	}

	changes := gen.FilterRoots(gen.Diff(base.Entries, after.Entries), base.Roots, after.Roots)
	ed := gen.DiffExtra(base.Extra, after.Extra)
	fmt.Printf("\n%s %s\n\n", ui.Bold(fmt.Sprintf("What `%s` changed", cmdline)),
		ui.Dim(fmt.Sprintf("(#%d -> #%d)", before, after.ID)))
	if len(changes) == 0 && ed.Empty() {
		fmt.Println("Nothing in the tracked files, packages or startup changed.")
	} else {
		renderChanges(changes, 25)
		renderExtra(ed)
		fmt.Printf("\nSaved as generation #%d. Undo with `dotka rollback %d`.\n", after.ID, before)
	}
	printWarnings(warns)
	if code != 0 {
		return exitError{code}
	}
	return nil
}

// childEnv points the command at the home dotka manages. When that is not
// the real home (e.g. the sandbox), HOME and the XDG dirs are redirected so
// the command cannot write into the real configuration by accident.
func childEnv(e env) []string {
	if e.realHome {
		return os.Environ()
	}
	override := map[string]string{
		"HOME":            e.home,
		"XDG_CONFIG_HOME": filepath.Join(e.home, ".config"),
		"XDG_DATA_HOME":   filepath.Join(e.home, ".local/share"),
		"XDG_STATE_HOME":  filepath.Join(e.home, ".local/state"),
		"XDG_CACHE_HOME":  filepath.Join(e.home, ".cache"),
	}
	var out []string
	for _, kv := range os.Environ() {
		k, _, _ := strings.Cut(kv, "=")
		if _, ok := override[k]; !ok {
			out = append(out, kv)
		}
	}
	for k, v := range override {
		out = append(out, k+"="+v)
	}
	return out
}

// cmdWhy explains where a file, package or startup item came from.
func cmdWhy(e env, args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: dotka why <path | package | startup command>")
	}
	q := args[0]
	m, err := e.manager()
	if err != nil {
		return err
	}
	found := false
	head, _ := m.Store.Head()
	headGen, _ := m.Load(head)
	now := func(present bool) {
		if present {
			fmt.Printf("  %s present in the current generation #%d\n", ui.Dim("now:"), head)
		} else {
			fmt.Printf("  %s not present in the current generation #%d\n", ui.Dim("now:"), head)
		}
	}

	// File: accept ~/x, absolute paths inside home, home-relative paths, and
	// shorthand relative to ~/.config.
	if rel, err := e.homeRel(q); err == nil {
		for _, p := range []string{rel, ".config/" + rel} {
			evs, err := m.FileHistory(p)
			if err != nil {
				return err
			}
			if len(evs) == 0 {
				continue
			}
			found = true
			fmt.Printf("%s ~/%s\n", ui.Bold("File"), p)
			printEvents(evs, func(ev gen.Event) string {
				if ev.Detail != "" && ev.Kind != gen.Added {
					return ev.Kind + " (" + ev.Detail + ")"
				}
				return ev.Kind
			})
			_, present := scan.Index(headGen.Entries)[p]
			now(present)
			fmt.Println()
			break
		}
	}
	if evs, err := m.PackageHistory(q); err == nil && len(evs) > 0 {
		found = true
		fmt.Printf("%s %s\n", ui.Bold("Package"), q)
		printEvents(evs, func(ev gen.Event) string {
			switch {
			case ev.Kept != 0:
				return "still installed"
			case ev.Kind == gen.Added:
				return "installed"
			}
			return "uninstalled"
		})
		now(headGen.Packages != nil && slices.Contains(headGen.Packages.Explicit, q))
		fmt.Println()
	}
	if evs, err := m.StartupHistory(q); err == nil && len(evs) > 0 {
		found = true
		fmt.Printf("%s %q\n", ui.Bold("Startup items matching"), q)
		printEvents(evs, func(ev gen.Event) string { return ev.Kind + ": " + ev.Detail })
		fmt.Println()
	}
	if !found {
		fmt.Printf("Dotka has no record of %q in any generation.\n", q)
		return nil
	}
	fmt.Println(ui.Dim("Changes made after the current generation are not shown here (see `dotka status`)."))
	return nil
}

func printEvents(evs []gen.Event, what func(gen.Event) string) {
	for _, ev := range evs {
		fmt.Printf("  #%-3d %s  %-14s %s\n", ev.Gen.ID, ev.Gen.CreatedAt.Local().Format("2006-01-02 15:04"),
			what(ev), ui.Dim(ev.Cause()))
	}
}
