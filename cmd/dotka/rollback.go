package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/iYeremy/dotka/internal/gen"
	"github.com/iYeremy/dotka/internal/ui"
)

func cmdRollback(e env, args []string) error {
	fs := flag.NewFlagSet("rollback", flag.ContinueOnError)
	dry := fs.Bool("dry-run", false, "only show the plan")
	yes := fs.Bool("yes", false, "do not ask for confirmation")
	fs.BoolVar(yes, "y", false, "short for --yes")
	keepNew := fs.Bool("keep-new", false, "keep files that did not exist in the target generation")
	pos, err := interspersed(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 1 {
		return fmt.Errorf("usage: dotka rollback N [--dry-run] [--yes] [--keep-new]")
	}
	id, err := strconv.Atoi(strings.TrimPrefix(pos[0], "#"))
	if err != nil {
		return fmt.Errorf("invalid generation %q", pos[0])
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

	plan, cur, err := m.Preview(id, *keepNew)
	if err != nil {
		return err
	}
	ed := gen.DiffExtra(cur.Extra, plan.Target.Extra)
	renderPlan(plan, *keepNew)
	renderRollbackExtra(ed)

	if plan.Empty() {
		fmt.Println("\nThe tracked files already match this generation.")
		return nil
	}
	if *dry {
		fmt.Println(ui.Dim("\n(dry run: nothing was changed)"))
		return nil
	}
	if !*yes && !confirm("\nProceed?") {
		fmt.Println("Aborted. Nothing was changed.")
		return nil
	}

	res, err := m.Rollback(id, *keepNew)
	if err != nil {
		return err
	}
	fmt.Printf("\n%s Restored generation #%d (%d actions).\n", ui.Green("✔"), id, len(res.Plan.Actions)-len(res.Failures))
	fmt.Printf("  Previous state saved as %s — undo with `dotka rollback %d`.\n", ui.Bold(fmt.Sprintf("#%d", res.Pre.ID)), res.Pre.ID)
	if len(res.Failures) > 0 {
		fmt.Printf("\n%s %d action(s) could not be applied:\n", ui.Yellow("⚠"), len(res.Failures))
		for _, f := range res.Failures {
			fmt.Printf("  %s %s: %v\n", f.Action.Kind, f.Action.Path, f.Err)
		}
	}
	return nil
}

func renderPlan(p gen.Plan, keepNew bool) {
	title := fmt.Sprintf("Plan to restore generation #%d", p.Target.ID)
	if p.Target.Message != "" {
		title += fmt.Sprintf(" %q", p.Target.Message)
	}
	fmt.Println(ui.Bold(title))
	fmt.Println(ui.Dim("  The current state will be saved first, so this rollback can be undone."))
	counts := map[string]int{}
	for _, a := range p.Actions {
		counts[a.Kind]++
	}
	fmt.Printf("  restore %d file(s), %d symlink(s), create %d dir(s), remove %d file(s)/link(s) and %d dir(s), %d permission fix(es)\n\n",
		counts[gen.ActWrite], counts[gen.ActSymlink], counts[gen.ActMkdir], counts[gen.ActRemove], counts[gen.ActRmdir], counts[gen.ActChmod])

	const limit = 40
	for i, a := range p.Actions {
		if i == limit {
			fmt.Printf("  %s\n", ui.Dim(fmt.Sprintf("… %d more", len(p.Actions)-limit)))
			break
		}
		var sym string
		switch a.Kind {
		case gen.ActRemove, gen.ActRmdir:
			sym = ui.Red("-")
		case gen.ActMkdir:
			sym = ui.Green("+")
		default:
			sym = ui.Yellow("↺")
		}
		path := a.Path
		if a.Kind == gen.ActRmdir || a.Kind == gen.ActMkdir {
			path += "/"
		}
		if a.Kind == gen.ActSymlink {
			path += " -> " + a.Want.Target
		}
		fmt.Printf("  %s %-8s %s %s\n", sym, a.Kind, path, ui.Dim("("+a.Reason+")"))
	}
	if len(p.Skipped) > 0 {
		fmt.Printf("\n  %s Not touched:\n", ui.Yellow("⚠"))
		for _, s := range p.Skipped {
			fmt.Printf("    %s %s\n", s.Path, ui.Dim("("+s.Reason+")"))
		}
	}
	if keepNew {
		fmt.Println(ui.Dim("  --keep-new: files created after the target generation are kept."))
	}
}

// renderRollbackExtra shows package and startup consequences. Dotka never
// runs package managers; it only prints commands for the user to review.
func renderRollbackExtra(d gen.ExtraDiff) {
	if len(d.PkgAdded)+len(d.PkgRemoved) > 0 {
		fmt.Printf("\n%s Packages are NOT changed by Dotka. Review and run yourself:\n", ui.Bold("Packages"))
		// d is computed current → target: "added" means the target has
		// packages that are missing now; "removed" means installed since.
		if len(d.PkgRemoved) > 0 {
			fmt.Printf("  %s installed since the target generation:\n", ui.Dim("#"))
			fmt.Printf("  sudo pacman -Rns %s\n", strings.Join(d.PkgRemoved, " "))
		}
		if len(d.PkgAdded) > 0 {
			native, foreign := splitForeign(d.PkgAdded, d.ForeignAdded)
			fmt.Printf("  %s present in the target generation but missing now:\n", ui.Dim("#"))
			if len(native) > 0 {
				fmt.Printf("  sudo pacman -S --needed %s\n", strings.Join(native, " "))
			}
			if len(foreign) > 0 {
				fmt.Printf("  %s -S %s   %s\n", aurHelper(), strings.Join(foreign, " "), ui.Dim("# AUR"))
			}
		}
	}
	if len(d.StartupAdded)+len(d.StartupRemoved) > 0 {
		fmt.Printf("\n%s after the rollback:\n", ui.Bold("Startup"))
		for _, s := range d.StartupRemoved {
			fmt.Printf("  %s %s\n", ui.Red("- will no longer run:"), formatStartup(s))
		}
		for _, s := range d.StartupAdded {
			fmt.Printf("  %s %s\n", ui.Green("+ will run again:"), formatStartup(s))
		}
	}
}

func splitForeign(all, foreign []string) (native, aur []string) {
	f := map[string]bool{}
	for _, x := range foreign {
		f[x] = true
	}
	for _, x := range all {
		if f[x] {
			aur = append(aur, x)
		} else {
			native = append(native, x)
		}
	}
	return
}

func aurHelper() string {
	for _, h := range []string{"paru", "yay"} {
		if _, err := lookPath(h); err == nil {
			return h
		}
	}
	return "<aur-helper>"
}

func confirm(q string) bool {
	fmt.Printf("%s [y/N] ", q)
	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes", "s", "si", "sí":
		return true
	}
	return false
}
