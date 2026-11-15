package main

import (
	"fmt"
	"strings"

	"github.com/iYeremy/dotka/internal/gen"
	"github.com/iYeremy/dotka/internal/scan"
	"github.com/iYeremy/dotka/internal/ui"
)

// renderChanges prints file changes, folding added/removed directories into
// one line ("+ .config/waybar/ (14 files)").
func renderChanges(changes []gen.Change, limit int) {
	if len(changes) == 0 {
		return
	}
	var add, mod, del int
	for _, c := range changes {
		if c.New != nil && c.New.Type == scan.TypeDir && c.Kind == gen.Added ||
			c.Old != nil && c.Old.Type == scan.TypeDir && c.Kind == gen.Removed {
			continue
		}
		switch c.Kind {
		case gen.Added:
			add++
		case gen.Removed:
			del++
		default:
			mod++
		}
	}
	fmt.Printf("%s  %s  %s  %s\n", ui.Bold("Files   "), ui.Green(fmt.Sprintf("+%d", add)),
		ui.Yellow(fmt.Sprintf("~%d", mod)), ui.Red(fmt.Sprintf("-%d", del)))

	shown := 0
	for i := 0; i < len(changes); i++ {
		c := changes[i]
		if shown == limit {
			fmt.Printf("  %s\n", ui.Dim(fmt.Sprintf("… %d more (use `dotka diff`)", len(changes)-i)))
			break
		}
		// Fold whole added/removed directories.
		if (c.Kind == gen.Added && c.New.Type == scan.TypeDir) || (c.Kind == gen.Removed && c.Old.Type == scan.TypeDir) {
			n := 0
			j := i + 1
			for ; j < len(changes) && strings.HasPrefix(changes[j].Path, c.Path+"/") && changes[j].Kind == c.Kind; j++ {
				if t := entryOf(changes[j]); t.Type != scan.TypeDir {
					n++
				}
			}
			sign, color := "+", ui.Green
			if c.Kind == gen.Removed {
				sign, color = "-", ui.Red
			}
			fmt.Printf("  %s %s/ %s\n", color(sign), c.Path, ui.Dim(fmt.Sprintf("(%d files)", n)))
			i = j - 1
			shown++
			continue
		}
		fmt.Printf("  %s\n", formatChange(c))
		shown++
	}
}

func entryOf(c gen.Change) scan.Entry {
	if c.New != nil {
		return *c.New
	}
	return *c.Old
}

func formatChange(c gen.Change) string {
	note := ""
	if en := entryOf(c); en.Skip == scan.SkipSecret {
		note = " " + ui.Dim("🔒 secret, not stored")
	} else if en.Skip == scan.SkipLarge {
		note = " " + ui.Dim("(too large to restore)")
	}
	switch c.Kind {
	case gen.Added:
		s := c.Path
		if c.New.Type == scan.TypeSymlink {
			s += " -> " + c.New.Target
		}
		return ui.Green("+ ") + s + note
	case gen.Removed:
		return ui.Red("- ") + c.Path + note
	case gen.TypeChg:
		msg := fmt.Sprintf("%s → %s", c.Old.Type, c.New.Type)
		warn := ""
		if c.Old.Type == scan.TypeFile && c.New.Type == scan.TypeSymlink {
			msg += " -> " + c.New.Target
			warn = " " + ui.Yellow("⚠ your file was replaced by a symlink")
		}
		return ui.Yellow("~ ") + c.Path + " " + ui.Dim("("+msg+")") + warn + note
	case gen.ModeChg:
		return ui.Yellow("~ ") + c.Path + " " + ui.Dim(fmt.Sprintf("(mode %o → %o)", c.Old.Mode, c.New.Mode))
	default:
		s := c.Path
		if c.New.Type == scan.TypeSymlink {
			s += ui.Dim(fmt.Sprintf(" (-> %s ⇒ %s)", c.Old.Target, c.New.Target))
		}
		return ui.Yellow("~ ") + s + note
	}
}

func renderExtra(d gen.ExtraDiff) {
	if len(d.PkgAdded)+len(d.PkgRemoved) > 0 {
		fmt.Printf("%s  %s  %s", ui.Bold("Packages"), ui.Green(fmt.Sprintf("+%d", len(d.PkgAdded))), ui.Red(fmt.Sprintf("-%d", len(d.PkgRemoved))))
		if len(d.ForeignAdded) > 0 {
			fmt.Printf(" %s", ui.Dim(fmt.Sprintf("(%d AUR/foreign added)", len(d.ForeignAdded))))
		}
		fmt.Println()
		for _, p := range d.PkgAdded {
			fmt.Printf("  %s %s\n", ui.Green("+"), p)
		}
		for _, p := range d.PkgRemoved {
			fmt.Printf("  %s %s\n", ui.Red("-"), p)
		}
	}
	if len(d.StartupAdded)+len(d.StartupRemoved) > 0 {
		fmt.Printf("%s  %s  %s\n", ui.Bold("Startup "), ui.Green(fmt.Sprintf("+%d", len(d.StartupAdded))), ui.Red(fmt.Sprintf("-%d", len(d.StartupRemoved))))
		for _, s := range d.StartupAdded {
			fmt.Printf("  %s %s\n", ui.Green("+"), formatStartup(s))
		}
		for _, s := range d.StartupRemoved {
			fmt.Printf("  %s %s\n", ui.Red("-"), formatStartup(s))
		}
	}
}

func formatStartup(s gen.StartupItem) string {
	loc := s.Source
	if s.Line > 0 {
		loc = fmt.Sprintf("%s:%d", s.Source, s.Line)
	}
	out := fmt.Sprintf("%-14s %s  %s", s.Kind, s.Command, ui.Dim(loc))
	if s.Confidence == "heuristic" {
		out += ui.Dim(" [heuristic]")
	}
	switch s.Risk {
	case "remote-exec":
		out += " " + ui.Red("⚠ downloads and runs code")
	case "local-script":
		out += " " + ui.Yellow("⚠ local script")
	}
	return out
}
