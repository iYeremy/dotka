package main

import (
	"fmt"
	"strings"

	"github.com/iYeremy/dotka/internal/collect"
	"github.com/iYeremy/dotka/internal/gen"
	"github.com/iYeremy/dotka/internal/startup"
	"github.com/iYeremy/dotka/internal/ui"
)

func init() {
	collector = collect.Extra
}

func cmdStartup(e env, args []string) error {
	items, warns := startup.Extract(e.home)
	var found, unsupported []string
	for _, d := range startup.Detect(e.home) {
		if d.Supported {
			found = append(found, d.Name)
		} else {
			unsupported = append(unsupported, d.Name+" (~/"+d.Path+")")
		}
	}
	if len(found) > 0 {
		fmt.Println(ui.Dim("Window managers / desktops found: " + strings.Join(found, ", ")))
	}
	for _, u := range unsupported {
		fmt.Printf("%s %s: its config is code dotka can't read yet, so its autostart isn't listed.\n", ui.Yellow("warning:"), u)
	}
	if len(found)+len(unsupported) > 0 {
		fmt.Println()
	}
	if len(items) == 0 {
		fmt.Println("Nothing found that runs automatically.")
		return nil
	}
	groups := map[string][]gen.StartupItem{}
	for _, it := range items {
		groups[startup.Group(it.Kind)] = append(groups[startup.Group(it.Kind)], it)
	}
	var names []string
	for _, g := range startup.Groups() {
		if len(groups[g]) > 0 {
			names = append(names, g)
		}
	}
	risky := 0
	for _, g := range names {
		fmt.Println(ui.Bold(g))
		for _, it := range groups[g] {
			fmt.Printf("  %s\n", formatStartup(it))
			if it.Risk == startup.RiskRemoteExec {
				risky++
			}
		}
		fmt.Println()
	}
	fmt.Printf("%d item(s)", len(items))
	if risky > 0 {
		fmt.Printf(", %s", ui.Red(fmt.Sprintf("%d download and run remote code", risky)))
	}
	fmt.Println(ui.Dim(" - static analysis only, this is not a security audit."))
	for _, w := range warns {
		fmt.Printf("  %s %s: %v\n", ui.Yellow("warning:"), w.Path, w.Err)
	}
	return nil
}
