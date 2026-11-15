package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/iYeremy/dotka/internal/config"
	"github.com/iYeremy/dotka/internal/gen"
	"github.com/iYeremy/dotka/internal/scan"
	"github.com/iYeremy/dotka/internal/store"
	"github.com/iYeremy/dotka/internal/textdiff"
	"github.com/iYeremy/dotka/internal/ui"
)

func cmdInit(e env, args []string) error {
	s, err := store.Create(e.storeDir)
	if err != nil {
		return err
	}
	cfg := config.Default()
	if err := cfg.Save(e.storeDir); err != nil {
		return err
	}
	m := e.managerWith(s, cfg)
	unlock, err := s.Lock()
	if err != nil {
		return err
	}
	defer unlock()
	fmt.Printf("Store created at %s\nScanning %s …\n", e.storeDir, e.home)
	g, warns, err := m.Snapshot("initial state", gen.OriginInit)
	if err != nil {
		return err
	}
	printSnapSummary(g, warns)
	return nil
}

func cmdSnap(e env, args []string) error {
	fs := flag.NewFlagSet("snap", flag.ContinueOnError)
	msg := fs.String("m", "", "message")
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
	g, warns, err := m.Snapshot(*msg, gen.OriginManual)
	if err != nil {
		return err
	}
	printSnapSummary(g, warns)
	return nil
}

func printSnapSummary(g gen.Generation, warns []scan.Warning) {
	var files, secrets, large int
	for _, en := range g.Entries {
		switch {
		case en.Skip == scan.SkipSecret:
			secrets++
		case en.Skip == scan.SkipLarge:
			large++
		case en.Type == scan.TypeFile:
			files++
		}
	}
	fmt.Printf("%s %s (%d files", ui.Green("Generation"), ui.Bold(fmt.Sprintf("#%d", g.ID)), files)
	if g.Packages != nil {
		fmt.Printf(", %d packages", len(g.Packages.Explicit))
	}
	if len(g.Startup) > 0 {
		fmt.Printf(", %d startup items", len(g.Startup))
	}
	fmt.Println(") saved.")
	if secrets > 0 {
		fmt.Printf("  %s %d file(s) look like secrets and were NOT stored (see `dotka secrets`)\n", ui.Yellow("⚠"), secrets)
	}
	if large > 0 {
		fmt.Printf("  %s %d file(s) exceed max_file_size and cannot be restored\n", ui.Yellow("⚠"), large)
	}
	printWarnings(warns)
}

func printWarnings(warns []scan.Warning) {
	for i, w := range warns {
		if i == 5 {
			fmt.Printf("  %s … and %d more warnings\n", ui.Yellow("⚠"), len(warns)-5)
			break
		}
		fmt.Printf("  %s %s: %v\n", ui.Yellow("⚠"), w.Path, w.Err)
	}
}

func cmdStatus(e env, args []string) error {
	m, err := e.manager()
	if err != nil {
		return err
	}
	head, err := m.Store.Head()
	if err != nil {
		return err
	}
	base, err := m.Load(head)
	if err != nil {
		return err
	}
	cur, err := m.Current()
	if err != nil {
		return err
	}
	changes := gen.FilterRoots(gen.Diff(base.Entries, cur.Entries), base.Roots, cur.Roots)
	ed := gen.DiffExtra(base.Extra, cur.Extra)
	title := fmt.Sprintf("Changes since generation #%d", base.ID)
	if base.Message != "" {
		title += fmt.Sprintf(" %q", base.Message)
	}
	fmt.Printf("%s %s\n\n", ui.Bold(title), ui.Dim("("+ui.Ago(base.CreatedAt)+")"))
	if len(changes) == 0 && ed.Empty() {
		fmt.Println("Nothing changed.")
		return nil
	}
	renderChanges(changes, 25)
	renderExtra(ed)
	fmt.Printf("\nRun `dotka snap -m \"…\"` to keep this, or `dotka rollback %d` to undo.\n", base.ID)
	printWarnings(cur.Warnings)
	return nil
}

func cmdDiff(e env, args []string) error {
	fs := flag.NewFlagSet("diff", flag.ContinueOnError)
	path := fs.String("path", "", "only show changes under this path")
	patch := fs.Bool("patch", false, "show line diffs of text files")
	fs.BoolVar(patch, "p", false, "short for --patch")
	pos, err := interspersed(fs, args)
	if err != nil {
		return err
	}
	m, err := e.manager()
	if err != nil {
		return err
	}
	var aE, bE []scan.Entry
	var aR, bR []string
	var aX, bX gen.Extra
	aName, bName := "", "now"
	switch len(pos) {
	case 0, 1:
		id, err := m.Store.Head()
		if err != nil {
			return err
		}
		if len(pos) == 1 {
			if id, err = strconv.Atoi(strings.TrimPrefix(pos[0], "#")); err != nil {
				return fmt.Errorf("invalid generation %q", pos[0])
			}
		}
		a, err := m.Load(id)
		if err != nil {
			return err
		}
		cur, err := m.Current()
		if err != nil {
			return err
		}
		aE, aR, aX, aName = a.Entries, a.Roots, a.Extra, fmt.Sprintf("#%d", id)
		bE, bR, bX = cur.Entries, cur.Roots, cur.Extra
	case 2:
		ids := [2]int{}
		for i, p := range pos {
			if ids[i], err = strconv.Atoi(strings.TrimPrefix(p, "#")); err != nil {
				return fmt.Errorf("invalid generation %q", p)
			}
		}
		a, err := m.Load(ids[0])
		if err != nil {
			return err
		}
		b, err := m.Load(ids[1])
		if err != nil {
			return err
		}
		aE, aR, aX, aName = a.Entries, a.Roots, a.Extra, fmt.Sprintf("#%d", a.ID)
		bE, bR, bX, bName = b.Entries, b.Roots, b.Extra, fmt.Sprintf("#%d", b.ID)
	default:
		return fmt.Errorf("usage: dotka diff [A [B]] [--path P] [--patch]")
	}
	changes := gen.FilterRoots(gen.Diff(aE, bE), aR, bR)
	if *path != "" {
		p, err := e.homeRel(*path)
		if err != nil {
			return err
		}
		var f []gen.Change
		for _, c := range changes {
			if underPath(c.Path, p) || underPath(c.Path, ".config/"+p) { // "hypr" works as ".config/hypr"
				f = append(f, c)
			}
		}
		changes = f
	}
	fmt.Printf("%s %s → %s\n\n", ui.Bold("Diff"), aName, bName)
	if *patch {
		for _, c := range changes {
			printPatch(m, e, c, aName, bName, bName == "now")
		}
		return nil
	}
	renderChanges(changes, 200)
	if *path == "" {
		renderExtra(gen.DiffExtra(aX, bX))
	}
	return nil
}

// content returns the bytes of an entry, from the store or (for "now") disk.
func content(m *gen.Manager, e env, en *scan.Entry, fromDisk bool) ([]byte, bool) {
	if en == nil {
		return nil, true
	}
	if en.Type != scan.TypeFile || en.Skip == scan.SkipSecret {
		return nil, false
	}
	if fromDisk {
		b, err := os.ReadFile(filepath.Join(e.home, en.Path))
		return b, err == nil
	}
	if !en.Stored {
		return nil, false
	}
	b, err := m.Store.ReadObject(en.Hash)
	return b, err == nil
}

func printPatch(m *gen.Manager, e env, c gen.Change, aName, bName string, bOnDisk bool) {
	if (c.Old != nil && c.Old.Type != scan.TypeFile) || (c.New != nil && c.New.Type != scan.TypeFile) {
		renderChanges([]gen.Change{c}, 1)
		return
	}
	a, okA := content(m, e, c.Old, false)
	b, okB := content(m, e, c.New, bOnDisk)
	if !okA || !okB {
		fmt.Printf("%s %s %s\n", ui.Yellow("?"), c.Path, ui.Dim("(content not available: secret or not stored)"))
		return
	}
	out := textdiff.Unified(aName+"/"+c.Path, bName+"/"+c.Path, a, b, 3)
	for _, line := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
		switch {
		case strings.HasPrefix(line, "+++"), strings.HasPrefix(line, "---"):
			fmt.Println(ui.Bold(line))
		case strings.HasPrefix(line, "@@"):
			fmt.Println(ui.Cyan(line))
		case strings.HasPrefix(line, "+"):
			fmt.Println(ui.Green(line))
		case strings.HasPrefix(line, "-"):
			fmt.Println(ui.Red(line))
		default:
			fmt.Println(line)
		}
	}
	fmt.Println()
}

func cmdLog(e env, args []string) error {
	m, err := e.manager()
	if err != nil {
		return err
	}
	gens, err := m.List()
	if err != nil {
		return err
	}
	head, _ := m.Store.Head()
	for i := len(gens) - 1; i >= 0; i-- {
		g := gens[i]
		mark := "  "
		if g.ID == head {
			mark = ui.Green("● ")
		}
		fmt.Printf("%s%s  %s  %-22s %s\n", mark, ui.Bold(fmt.Sprintf("#%-3d", g.ID)),
			g.CreatedAt.Local().Format("2006-01-02 15:04"), ui.Dim(g.Origin), g.Message)
	}
	return nil
}

func cmdShow(e env, args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: dotka show N")
	}
	id, err := strconv.Atoi(strings.TrimPrefix(args[0], "#"))
	if err != nil {
		return fmt.Errorf("invalid generation %q", args[0])
	}
	m, err := e.manager()
	if err != nil {
		return err
	}
	g, err := m.Load(id)
	if err != nil {
		return err
	}
	var files, dirs, links, secrets, large int
	var size int64
	for _, en := range g.Entries {
		switch en.Type {
		case scan.TypeFile:
			files++
			size += en.Size
		case scan.TypeDir:
			dirs++
		case scan.TypeSymlink:
			links++
		}
		switch en.Skip {
		case scan.SkipSecret:
			secrets++
		case scan.SkipLarge:
			large++
		}
	}
	fmt.Printf("%s %s\n", ui.Bold(fmt.Sprintf("Generation #%d", g.ID)), g.Message)
	fmt.Printf("  created   %s (%s)\n", g.CreatedAt.Local().Format("2006-01-02 15:04:05"), ui.Ago(g.CreatedAt))
	fmt.Printf("  origin    %s\n", g.Origin)
	fmt.Printf("  files     %d (%s), %d dirs, %d symlinks\n", files, humanSize(size), dirs, links)
	if secrets+large > 0 {
		fmt.Printf("  not stored: %d secret(s), %d too large\n", secrets, large)
	}
	if g.Packages != nil {
		fmt.Printf("  packages  %d explicit (%d foreign/AUR) via %s\n", len(g.Packages.Explicit), len(g.Packages.Foreign), g.Packages.Manager)
	}
	if len(g.Startup) > 0 {
		fmt.Printf("  startup   %d items\n", len(g.Startup))
		for _, s := range g.Startup {
			fmt.Printf("    %s\n", formatStartup(s))
		}
	}
	return nil
}

func cmdTrack(e env, args []string) error {
	sub := "ls"
	if len(args) > 0 {
		sub = args[0]
	}
	cfg, err := config.Load(e.storeDir)
	if err != nil {
		if os.IsNotExist(err) {
			return store.ErrNotInitialized
		}
		return err
	}
	switch sub {
	case "ls", "list":
		for _, r := range cfg.Roots {
			mark := ui.Dim("(missing)")
			if _, err := os.Lstat(filepath.Join(e.home, r)); err == nil {
				mark = ""
			}
			fmt.Printf("~/%s %s\n", r, mark)
		}
		return nil
	case "add", "rm", "remove":
		if len(args) != 2 {
			return fmt.Errorf("usage: dotka track %s PATH", sub)
		}
		rel, err := e.homeRel(args[1])
		if err != nil {
			return err
		}
		if sub == "add" {
			ok, err := cfg.AddRoot(rel)
			if err != nil {
				return err
			}
			if !ok {
				fmt.Printf("~/%s is already tracked\n", rel)
				return nil
			}
			fmt.Printf("Now tracking ~/%s (included from the next `dotka snap`)\n", rel)
		} else {
			if !cfg.RemoveRoot(rel) {
				return fmt.Errorf("~/%s is not a tracked root", rel)
			}
			fmt.Printf("Stopped tracking ~/%s (existing generations keep it)\n", rel)
		}
		return cfg.Save(e.storeDir)
	}
	return fmt.Errorf("unknown track subcommand %q", sub)
}

func cmdSecrets(e env, args []string) error {
	sub := "ls"
	if len(args) > 0 {
		sub = args[0]
	}
	m, err := e.manager()
	if err != nil {
		return err
	}
	switch sub {
	case "ls", "list":
		cur, err := m.Current()
		if err != nil {
			return err
		}
		n := 0
		for _, en := range cur.Entries {
			if en.Skip == scan.SkipSecret {
				fmt.Printf("%s ~/%s %s\n", ui.Yellow("🔒"), en.Path, ui.Dim("("+en.Reason+")"))
				n++
			}
		}
		if n == 0 {
			fmt.Println("No secrets detected in tracked roots.")
		}
		if len(m.Config.AllowSecrets) > 0 {
			fmt.Println("\nExplicitly allowed (stored):")
			for _, p := range m.Config.AllowSecrets {
				fmt.Printf("  ~/%s\n", p)
			}
		}
		return nil
	case "allow", "deny":
		if len(args) != 2 {
			return fmt.Errorf("usage: dotka secrets %s PATH", sub)
		}
		rel, err := e.homeRel(args[1])
		if err != nil {
			return err
		}
		cfg, err := config.Load(e.storeDir)
		if err != nil {
			return err
		}
		if sub == "allow" {
			if !cfg.SecretAllowed(rel) {
				cfg.AllowSecrets = append(cfg.AllowSecrets, rel)
			}
			fmt.Printf("~/%s will be stored from the next snapshot. The store is private (0700) but NOT encrypted.\n", rel)
		} else {
			var keep []string
			for _, p := range cfg.AllowSecrets {
				if p != rel {
					keep = append(keep, p)
				}
			}
			cfg.AllowSecrets = keep
			fmt.Printf("~/%s will no longer be stored (older generations still contain it until `dotka gc` after removing them).\n", rel)
		}
		return cfg.Save(e.storeDir)
	}
	return fmt.Errorf("unknown secrets subcommand %q", sub)
}

func cmdGC(e env, args []string) error {
	m, err := e.manager()
	if err != nil {
		return err
	}
	unlock, err := m.Store.Lock()
	if err != nil {
		return err
	}
	defer unlock()
	keep, err := m.LiveObjects()
	if err != nil {
		return err
	}
	n, b, err := m.Store.GC(keep)
	if err != nil {
		return err
	}
	m.Store.AppendLog(store.LogEntry{Op: "gc", Result: "ok", Details: fmt.Sprintf("%d objects", n)})
	fmt.Printf("Removed %d unreferenced object(s), %s freed.\n", n, humanSize(b))
	return nil
}

func underPath(path, prefix string) bool {
	return path == prefix || strings.HasPrefix(path, prefix+"/")
}

func humanSize(b int64) string {
	const k = 1024
	switch {
	case b < k:
		return fmt.Sprintf("%d B", b)
	case b < k*k:
		return fmt.Sprintf("%.1f KiB", float64(b)/k)
	case b < k*k*k:
		return fmt.Sprintf("%.1f MiB", float64(b)/(k*k))
	}
	return fmt.Sprintf("%.1f GiB", float64(b)/(k*k*k))
}
