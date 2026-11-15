package main

import (
	"fmt"
	"os/exec"

	"github.com/iYeremy/dotka/internal/config"
	"github.com/iYeremy/dotka/internal/doctor"
	"github.com/iYeremy/dotka/internal/pkgs"
	"github.com/iYeremy/dotka/internal/scan"
	"github.com/iYeremy/dotka/internal/startup"
	"github.com/iYeremy/dotka/internal/ui"
)

// cmdDoctor runs read-only health checks. It works without `dotka init`.
func cmdDoctor(e env, args []string) error {
	cfg := config.Default()
	if c, err := config.Load(e.storeDir); err == nil {
		cfg = c
	}
	res, err := scan.Scan(scan.Options{Home: e.home, Config: cfg, NoHash: true})
	if err != nil {
		return err
	}
	items, _ := startup.Extract(e.home)
	in := doctor.Input{Home: e.home, Entries: res.Entries, Startup: items, LookPath: exec.LookPath}
	if src := pkgs.FromEnv(); src != nil {
		if all, err := src.All(); err == nil {
			in.Packages = all
		}
	}
	findings := doctor.Run(in)

	fmt.Printf("%s %s\n", ui.Bold("Checking"), e.home)
	fmt.Println(ui.Dim(fmt.Sprintf("  %d tracked entries, %d startup items. Read-only: nothing is changed.", len(res.Entries), len(items))))
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
