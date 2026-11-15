package tui

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/iYeremy/dotka/internal/doctor"
	"github.com/iYeremy/dotka/internal/gen"
	"github.com/iYeremy/dotka/internal/scan"
	"github.com/iYeremy/dotka/internal/startup"
)

// buildRows derives the list rows of every tab from the data. Pure.
func (m *Model) buildRows() {
	d := m.data
	for t := range m.rows {
		m.rows[t] = nil
	}

	// Changes.
	var ch []row
	files := 0
	for _, c := range d.Changes {
		if !isFoldedChild(d.Changes, c) {
			files++
		}
	}
	if files > 0 {
		ch = append(ch, row{header: true, text: fmt.Sprintf("Files (%d)", files)})
		for i := range d.Changes {
			c := &d.Changes[i]
			if isFoldedChild(d.Changes, *c) {
				continue
			}
			ch = append(ch, row{change: c, text: changeLine(d.Changes, *c)})
		}
	}
	if n := len(d.Extra.PkgAdded) + len(d.Extra.PkgRemoved); n > 0 {
		ch = append(ch, row{header: true, text: fmt.Sprintf("Packages (%d)", n)})
		for _, p := range d.Extra.PkgAdded {
			ch = append(ch, row{pkg: p, added: true, text: sAdded.Render("+") + " " + p})
		}
		for _, p := range d.Extra.PkgRemoved {
			ch = append(ch, row{pkg: p, text: sRemoved.Render("-") + " " + p})
		}
	}
	if n := len(d.Extra.StartupAdded) + len(d.Extra.StartupRemoved); n > 0 {
		ch = append(ch, row{header: true, text: fmt.Sprintf("Runs at startup (%d)", n)})
		for i := range d.Extra.StartupAdded {
			s := &d.Extra.StartupAdded[i]
			ch = append(ch, row{startup: s, added: true, text: sAdded.Render("+") + " " + startupLine(*s)})
		}
		for i := range d.Extra.StartupRemoved {
			s := &d.Extra.StartupRemoved[i]
			ch = append(ch, row{startup: s, text: sRemoved.Render("-") + " " + startupLine(*s)})
		}
	}
	m.rows[tabChanges] = ch

	// History.
	for i := range d.Gens {
		g := &d.Gens[i]
		mark := "  "
		if g.ID == d.Head.ID {
			mark = sAccent.Render("* ")
		}
		label := g.Message
		if label == "" {
			label = sMuted.Render("(" + g.Origin + ")")
		}
		m.rows[tabHistory] = append(m.rows[tabHistory], row{gen: g,
			text: fmt.Sprintf("%s#%-3d %s  %s", mark, g.ID, g.CreatedAt.Local().Format("Jan 02 15:04"), label)})
	}

	// Startup, grouped.
	groups := startup.Groups()
	byGroup := map[string][]row{}
	for i := range d.Startup {
		s := &d.Startup[i]
		g := startup.Group(s.Kind)
		byGroup[g] = append(byGroup[g], row{startup: s, text: startupLine(*s)})
	}
	for _, g := range groups {
		if rs := byGroup[g]; len(rs) > 0 {
			m.rows[tabStartup] = append(m.rows[tabStartup], row{header: true, text: fmt.Sprintf("%s (%d)", g, len(rs))})
			m.rows[tabStartup] = append(m.rows[tabStartup], rs...)
		}
	}

	// Health.
	var probs, notes []row
	for i := range d.Findings {
		f := &d.Findings[i]
		r := row{finding: f, text: findingLine(*f)}
		if f.Severity == doctor.Problem {
			probs = append(probs, r)
		} else {
			notes = append(notes, r)
		}
	}
	if len(probs) > 0 {
		m.rows[tabHealth] = append(m.rows[tabHealth], row{header: true, text: fmt.Sprintf("Problems (%d)", len(probs))})
		m.rows[tabHealth] = append(m.rows[tabHealth], probs...)
	}
	if len(notes) > 0 {
		m.rows[tabHealth] = append(m.rows[tabHealth], row{header: true, text: fmt.Sprintf("Notes (%d)", len(notes))})
		m.rows[tabHealth] = append(m.rows[tabHealth], notes...)
	}
}

func changeSymbol(kind string) string {
	switch kind {
	case gen.Added:
		return sAdded.Render("+")
	case gen.Removed:
		return sRemoved.Render("-")
	}
	return sModified.Render("~")
}

func changeLine(all []gen.Change, c gen.Change) string {
	e := c.New
	if e == nil {
		e = c.Old
	}
	s := changeSymbol(c.Kind) + " " + c.Path
	switch {
	case e.Type == scan.TypeDir && (c.Kind == gen.Added || c.Kind == gen.Removed):
		n := len(foldedChildren(all, c))
		s += "/ " + sMuted.Render(plural(n, "file"))
	case c.Kind == gen.TypeChg && c.Old.Type == scan.TypeFile && c.New.Type == scan.TypeSymlink:
		s += " " + sModified.Render("[!] now a link")
	case e.Skip == scan.SkipSecret:
		s += " " + sMuted.Render("[secret]")
	case e.NixManaged() || (c.Old != nil && c.Old.NixManaged()):
		s += " " + sMuted.Render("[nix]")
	}
	return s
}

func startupLine(s gen.StartupItem) string {
	cmd := s.Command
	if s.Kind == "systemd-user" {
		cmd, _, _ = strings.Cut(cmd, ": ")
	}
	switch s.Risk {
	case "remote-exec":
		return sDanger.Render("[!]") + " " + cmd
	case "local-script":
		return sModified.Render("[?]") + " " + cmd
	}
	return "    " + cmd
}

func findingLine(f doctor.Finding) string {
	where := filepath.Base(f.Path)
	if f.Line > 0 {
		where = fmt.Sprintf("%s:%d", where, f.Line)
	}
	mark := sModified.Render("[-]")
	if f.Severity == doctor.Problem {
		mark = sDanger.Render("[x]")
	}
	return mark + " " + fmt.Sprintf("%-18s", checkTitle(f.Check)) + " " + sMuted.Render(where)
}

func plural(n int, w string) string {
	if n == 1 {
		return "1 " + w
	}
	return fmt.Sprintf("%d %ss", n, w)
}

// ---- detail panes (pure: data + caches -> lines) ------------------------------

// detail returns the title and lines of the right pane for the selection.
func (m Model) detail(w int) (string, []string) {
	r := m.selected()
	if r == nil {
		return "", nil
	}
	p := func(s string) []string { return wrap(s, w) }
	var out []string
	add := func(lines ...string) { out = append(out, lines...) }
	blank := func() { out = append(out, "") }

	switch {
	case r.change != nil:
		c := *r.change
		add(sBold.Render(strings.ToUpper(c.Kind[:1]) + c.Kind[1:]))
		add(p(explainChange(c))...)
		blank()
		e := c.New
		if e == nil {
			e = c.Old
		}
		switch {
		case e.Type == scan.TypeDir && (c.Kind == gen.Added || c.Kind == gen.Removed):
			add(sMuted.Render("Files inside:"))
			for _, f := range foldedChildren(m.data.Changes, c) {
				add("  " + f)
			}
		case e.Type == scan.TypeSymlink || (c.New != nil && c.New.Type == scan.TypeSymlink):
			if c.Old != nil && c.Old.Type == scan.TypeSymlink {
				add("was a link to: " + c.Old.Target)
			}
			if c.New != nil && c.New.Type == scan.TypeSymlink {
				add("now links to:  " + c.New.Target)
			}
		default:
			lines, ok := m.patches[c.Path]
			switch {
			case !ok || lines == nil:
				add(sMuted.Render("Loading..."))
			default:
				for _, l := range lines {
					add(styleDiffLine(l))
				}
			}
		}
		return c.Path, out

	case r.pkg != "":
		if r.added {
			add(sBold.Render("Installed since your save point"))
			add(p("Dotka never installs or removes packages. Going back restores your files, not this package. To remove it yourself:")...)
			blank()
			add("  sudo pacman -Rns " + r.pkg)
		} else {
			add(sBold.Render("Uninstalled since your save point"))
			add(p("Dotka will not reinstall it. To get it back yourself:")...)
			blank()
			add("  sudo pacman -S " + r.pkg)
		}
		blank()
		add(sMuted.Render("Tip: `dotka why " + r.pkg + "` tells you which command installed it."))
		return "package " + r.pkg, out

	case r.startup != nil:
		s := *r.startup
		if m.tab == tabChanges {
			if r.added {
				add(sBold.Render("New: this will now run automatically"))
			} else {
				add(sBold.Render("Removed: this no longer runs automatically"))
			}
			blank()
		}
		add(sMuted.Render("Command"))
		add(p(s.Command)...)
		blank()
		add(sMuted.Render("What it is"))
		add(p(explainKind(s.Kind))...)
		blank()
		where := s.Source
		if s.Line > 0 {
			where = fmt.Sprintf("%s:%d", s.Source, s.Line)
		}
		add(sMuted.Render("Defined in"))
		add(p(where)...)
		if rk := explainRisk(s.Risk); rk != "" {
			blank()
			if s.Risk == "remote-exec" {
				add(sDanger.Render("[!] Downloads and runs code"))
			} else {
				add(sModified.Render("[?] Runs a local script"))
			}
			add(p(rk)...)
		}
		if s.Confidence == "heuristic" {
			blank()
			add(sMuted.Render("Detected by pattern matching: it may be wrong."))
		}
		return kindTitle(s.Kind), out

	case r.gen != nil:
		g := *r.gen
		add(field("Created", g.CreatedAt.Local().Format("2006-01-02 15:04"), w)...)
		add(field("How", explainOrigin(g), w)...)
		var files int
		for _, e := range g.Entries {
			if e.Type == scan.TypeFile {
				files++
			}
		}
		contains := plural(files, "file")
		if g.Packages != nil {
			contains += ", " + plural(len(g.Packages.Explicit), "package")
		}
		contains += ", " + plural(len(g.Startup), "startup item")
		add(field("Contains", contains, w)...)
		blank()
		if g.ID == m.data.Head.ID {
			add(sAccent.Render("* This is your current save point."))
			add(p("Unsaved changes are compared against it (tab 2).")...)
		} else {
			c := m.compare[g.ID]
			add(sBold.Render("Going back here would"))
			if c == [3]int{} {
				add("  change nothing in your tracked files")
			}
			if c[1] > 0 {
				add("  " + sModified.Render("~") + " revert " + plural(c[1], "file"))
			}
			if c[2] > 0 {
				add("  " + sAdded.Render("+") + " restore " + plural(c[2], "deleted file"))
			}
			if c[0] > 0 {
				add("  " + sRemoved.Render("-") + " remove " + plural(c[0], "newer file"))
			}
			blank()
			add(keyHint("b", "go back here", "x", "forget this save point"))
		}
		return fmt.Sprintf("#%d %s", g.ID, g.Message), out

	case r.finding != nil:
		f := *r.finding
		if f.Severity == doctor.Problem {
			add(sDanger.Render("[x] Problem"))
		} else {
			add(sModified.Render("[-] Note (worth a look)"))
		}
		add(p(f.Message)...)
		blank()
		where := f.Path
		if f.Line > 0 {
			where = fmt.Sprintf("%s:%d", f.Path, f.Line)
		}
		add(sMuted.Render("Where"))
		add(p(where)...)
		blank()
		if ex := explainCheck(f.Check); ex != "" {
			add(sMuted.Render("What it means"))
			add(p(ex)...)
			blank()
		}
		if f.Hint != "" {
			add(sMuted.Render("What you can do"))
			add(p(f.Hint)...)
		}
		return checkTitle(f.Check), out
	}
	return "", nil
}

// field renders "Label     value" with the value wrapped and indented.
func field(label, value string, w int) []string {
	const lw = 10
	lines := wrap(value, w-lw)
	if len(lines) == 0 {
		lines = []string{""}
	}
	out := []string{sMuted.Render(fmt.Sprintf("%-*s", lw, label)) + lines[0]}
	for _, l := range lines[1:] {
		out = append(out, strings.Repeat(" ", lw)+l)
	}
	return out
}

func styleDiffLine(l string) string {
	switch {
	case strings.HasPrefix(l, "+++"), strings.HasPrefix(l, "---"):
		return sMuted.Render(l)
	case strings.HasPrefix(l, "@@"):
		return sAccent.Render(l)
	case strings.HasPrefix(l, "+"):
		return sAdded.Render(l)
	case strings.HasPrefix(l, "-"):
		return sRemoved.Render(l)
	}
	return l
}
