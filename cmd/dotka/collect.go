package main

import (
	"fmt"
	"sort"

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
	if len(items) == 0 {
		fmt.Println("Nothing found that runs automatically.")
		return nil
	}
	groups := map[string][]gen.StartupItem{}
	for _, it := range items {
		groups[group(it.Kind)] = append(groups[group(it.Kind)], it)
	}
	names := make([]string, 0, len(groups))
	for g := range groups {
		names = append(names, g)
	}
	sort.Strings(names)
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

func group(kind string) string {
	switch {
	case len(kind) >= 4 && kind[:4] == "hypr":
		return "Hyprland"
	case kind == "xdg-autostart":
		return "XDG autostart"
	case kind == "systemd-user":
		return "systemd --user"
	case kind == "x11-exec":
		return "X11 session"
	}
	return "Shell"
}
