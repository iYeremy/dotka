package main

import (
	"fmt"

	"github.com/iYeremy/dotka/internal/config"
	"github.com/iYeremy/dotka/internal/doctor"
	"github.com/iYeremy/dotka/internal/ui"
)

// cmdDoctor runs read-only health checks. It works without `dotka init`.
func cmdDoctor(e env, args []string) error {
	cfg := config.Default()
	if c, err := config.Load(e.storeDir); err == nil {
		cfg = c
	}
	in, err := doctor.Collect(e.home, cfg)
	if err != nil {
		return err
	}
	findings := doctor.Run(in)

	fmt.Printf("%s %s\n", ui.Bold("Checking"), e.home)
	fmt.Println(ui.Dim(fmt.Sprintf("  %d tracked entries, %d startup items. Read-only: nothing is changed.", len(in.Entries), len(in.Startup))))
	if len(findings) == 0 {
		fmt.Println("\nNo problems found.")
		return nil
	}
	var problems, notes int
	last := ""
	for _, f := range findings {
		if f.Severity != last {
			if f.Severity == doctor.Problem {
				fmt.Printf("\n%s\n", ui.Bold("Problems"))
			} else {
				fmt.Printf("\n%s\n", ui.Bold("Notes"))
			}
			last = f.Severity
		}
		loc := f.Path
		if f.Line > 0 {
			loc = fmt.Sprintf("%s:%d", f.Path, f.Line)
		}
		mark := ui.Red("[problem]")
		if f.Severity == doctor.Note {
			mark = ui.Yellow("[note]")
			notes++
		} else {
			problems++
		}
		fmt.Printf("  %s %s\n      %s\n", mark, loc, f.Message)
		if f.Hint != "" {
			fmt.Printf("      %s\n", ui.Dim("-> "+f.Hint))
		}
	}
	fmt.Printf("\n%d problem(s), %d note(s). %s\n", problems, notes,
		ui.Dim("Static checks only: this is not a security audit."))
	return nil
}
