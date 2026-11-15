package main

import (
	"flag"
	"fmt"
	"strconv"
	"strings"

	"github.com/iYeremy/dotka/internal/gen"
	"github.com/iYeremy/dotka/internal/ui"
)

func cmdForget(e env, args []string) error {
	fs := flag.NewFlagSet("forget", flag.ContinueOnError)
	yes := fs.Bool("yes", false, "do not ask for confirmation")
	fs.BoolVar(yes, "y", false, "short for --yes")
	dry := fs.Bool("dry-run", false, "only show what would be forgotten")
	pos, err := interspersed(fs, args)
	if err != nil {
		return err
	}
	if len(pos) == 0 {
		return fmt.Errorf("usage: dotka forget N [N...] [--dry-run] [--yes]")
	}
	var ids []int
	for _, p := range pos {
		id, err := strconv.Atoi(strings.TrimPrefix(p, "#"))
		if err != nil {
			return fmt.Errorf("invalid generation %q", p)
		}
		ids = append(ids, id)
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
	plan, err := m.PlanForget(ids)
	if err != nil {
		return err
	}
	return runForget(m, plan, *dry, *yes)
}

func cmdPrune(e env, args []string) error {
	fs := flag.NewFlagSet("prune", flag.ContinueOnError)
	keep := fs.Int("keep", 10, "number of most recent generations to keep")
	yes := fs.Bool("yes", false, "do not ask for confirmation")
	fs.BoolVar(yes, "y", false, "short for --yes")
	dry := fs.Bool("dry-run", false, "only show what would be forgotten")
	if _, err := interspersed(fs, args); err != nil {
		return err
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
	plan, err := m.PlanPrune(*keep)
	if err != nil {
		return err
	}
	if len(plan.IDs) == 0 {
		fmt.Printf("Nothing to prune: %d or fewer generations besides the current one.\n", *keep)
		return nil
	}
	return runForget(m, plan, *dry, *yes)
}

func runForget(m *gen.Manager, plan gen.ForgetPlan, dry, yes bool) error {
	fmt.Println(ui.Bold("Generations to forget:"))
	for _, id := range plan.IDs {
		g, err := m.Load(id)
		if err != nil {
			return err
		}
		fmt.Printf("  #%-3d %s  %-22s %s\n", g.ID, g.CreatedAt.Local().Format("2006-01-02 15:04"), ui.Dim(g.Origin), g.Message)
	}
	fmt.Printf("\nThis frees %s (%d stored file version(s) used by no other generation).\n",
		humanSize(plan.FreedBytes), plan.FreedObjects)
	fmt.Println(ui.Dim("Your files in home are not touched; you just can no longer go back to these points."))
	if dry {
		fmt.Println(ui.Dim("(dry run: nothing was changed)"))
		return nil
	}
	if !yes && !confirm("Forget them?") {
		fmt.Println("Aborted. Nothing was changed.")
		return nil
	}
	n, b, err := m.Forget(plan)
	if err != nil {
		return err
	}
	fmt.Printf("%s Forgot %d generation(s), freed %s (%d object(s)).\n", ui.Green("Done."), len(plan.IDs), humanSize(b), n)
	return nil
}
