package tui

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/iYeremy/dotka/internal/gen"
)

// Layout (h lines):
//
//	header       1   dotka  <home> [tag]              save point #N - X unsaved
//	tabs         1    1 Overview   2 Changes 11   3 History ...
//	rule         1   -------------------------------------------------------
//	body       h-5   overview | list + detail | modal
//	status rule  1   ---- last action result ----------------------------------
//	footer       1   contextual key hints
func (m Model) View() string {
	if m.width == 0 {
		return "" // waiting for the first WindowSizeMsg
	}
	if m.width < minWidth || m.height < minHeight {
		return m.tooSmall()
	}
	lines := []string{m.header(), m.tabs(), rule(m.width)}
	lines = append(lines, m.body()...)
	lines = append(lines, m.statusLine(), m.footer())
	for i, l := range lines {
		lines[i] = fit(l, m.width)
	}
	return strings.Join(lines, "\n")
}

func (m Model) bodyHeight() int { return m.height - 5 }

// listHeight is the number of rows visible in a list pane: body minus the
// title and a blank line, minus one line for the "more" indicator when the
// list overflows.
func (m Model) listHeight() int {
	h := m.bodyHeight() - 2
	if len(m.rows[m.tab]) > h {
		h--
	}
	return h
}

func (m Model) tooSmall() string {
	var msg []string
	for _, p := range []string{
		"Terminal too small",
		"",
		fmt.Sprintf("dotka needs at least %dx%d, this one is %dx%d.", minWidth, minHeight, m.width, m.height),
		"Make the window bigger, or use the dotka commands (dotka help).",
		"",
		"q quit",
	} {
		msg = append(msg, wrap(p, m.width-2)...)
	}
	lines := center(m.height, m.width, msg)
	for len(lines) < m.height {
		lines = append(lines, "")
	}
	for i := range lines {
		lines[i] = fit(lines[i], m.width)
	}
	return strings.Join(lines[:m.height], "\n")
}

func (m Model) header() string {
	brand := sBold.Render(" dotka") + "  "
	if m.data == nil {
		return brand
	}
	d := m.data
	right := sMuted.Render("not set up yet")
	if d.Initialized {
		right = fmt.Sprintf("save point #%d", d.Head.ID)
		if n := d.Unsaved(); n > 0 {
			right += "  " + sModified.Render(fmt.Sprintf("%d unsaved", n))
		} else {
			right += "  " + sMuted.Render("no unsaved changes")
		}
	}
	right += " "

	tag := ""
	switch {
	case d.Sandbox:
		tag = "  " + sAccent.Render("[sandbox]")
	case d.ReadOnly:
		tag = "  " + sModified.Render("[read-only]")
	}
	home := d.Home
	if h, err := os.UserHomeDir(); err == nil && !d.Sandbox && strings.HasPrefix(home, h) {
		home = "~" + strings.TrimPrefix(home, h)
	}
	// The path gives way first (trimmed from the left) so the tag and the
	// save point info always stay visible.
	budget := m.width - lipgloss.Width(brand) - lipgloss.Width(tag) - lipgloss.Width(right) - 2
	home = trimLeft(home, budget)
	left := brand + home + tag
	return fit(left, m.width-lipgloss.Width(right)) + right
}

// trimLeft shortens a path from the left: ".../Dotka/.sandbox/home".
func trimLeft(p string, w int) string {
	if w <= 0 {
		return ""
	}
	if len(p) <= w {
		return p
	}
	if w <= 4 {
		return p[len(p)-w:]
	}
	return "..." + p[len(p)-(w-3):]
}

func (m Model) tabs() string {
	labels := [numTabs]string{"Overview", "Changes", "History", "Startup", "Health"}
	if d := m.data; d != nil {
		if n := d.Unsaved(); n > 0 {
			labels[tabChanges] += fmt.Sprintf(" %d", n)
		}
		if d.Initialized {
			labels[tabHistory] += fmt.Sprintf(" %d", len(d.Gens))
		}
		labels[tabStartup] += fmt.Sprintf(" %d", len(d.Startup))
		if p := d.Problems(); p > 0 {
			labels[tabHealth] += fmt.Sprintf(" %d!", p)
		}
	}
	var parts []string
	for i, l := range labels {
		s := fmt.Sprintf(" %d %s ", i+1, l)
		if name, count, ok := strings.Cut(l, " "); ok {
			s = fmt.Sprintf(" %d %s (%s) ", i+1, name, count)
		}
		if tab(i) == m.tab {
			s = sTabOn.Render(s)
		}
		parts = append(parts, s)
	}
	return " " + strings.Join(parts, " ")
}

func (m Model) body() []string {
	h := m.bodyHeight()
	var lines []string
	switch {
	case m.modal != modalNone:
		lines = m.modalView(h)
	case m.data == nil && m.err != nil:
		lines = center(h, m.width, []string{
			sDanger.Render("Could not read your configuration"), "", m.err.Error(), "", keyHint("r", "retry", "q", "quit")})
	case m.data == nil:
		lines = center(h, m.width, []string{"Scanning your home...", "", sMuted.Render("The first time can take a few seconds.")})
	case m.tab == tabOverview:
		lines = m.overview()
	default:
		lines = m.split(h)
	}
	for len(lines) < h {
		lines = append(lines, "")
	}
	return lines[:h]
}

func center(h, w int, msg []string) []string {
	out := make([]string, 0, h)
	top := (h - len(msg)) / 2
	for i := 0; i < top; i++ {
		out = append(out, "")
	}
	for _, l := range msg {
		pad := (w - lipgloss.Width(l)) / 2
		if pad < 0 {
			pad = 0
		}
		out = append(out, strings.Repeat(" ", pad)+l)
	}
	return out
}

// ---- overview -------------------------------------------------------------------

func (m Model) overview() []string {
	d := m.data
	var out []string
	add := func(s ...string) { out = append(out, s...) }
	in := "  "
	w := m.width - 4

	add("")
	if !d.Initialized {
		add(in + sBold.Render("Welcome to dotka"))
		for _, l := range wrap("Dotka keeps save points of your Linux configuration. Before trying someone's dotfiles, save a point; afterwards dotka shows exactly what changed (files, packages, what runs at login) and lets you go back.", w) {
			add(in + l)
		}
		add("")
		if d.ReadOnly {
			for _, l := range wrap("This is your real home and dotka is still a prototype, so it is read-only here. You can still explore tabs 4 (Startup) and 5 (Health).", w) {
				add(in + sModified.Render(l))
			}
		} else {
			add(in + keyHint("i", "set up dotka here (saves your current state as save point #1)"))
		}
		add("")
	} else {
		title := fmt.Sprintf("Unsaved changes since save point #%d", d.Head.ID)
		if d.Head.Message != "" {
			title += fmt.Sprintf(" %q", d.Head.Message)
		}
		add(in+sBold.Render(title)+"  "+sMuted.Render(ago(d.Head)), "")
		if d.Unsaved() == 0 {
			add(in + "  Nothing changed. Your configuration matches this save point.")
		} else {
			var a, mo, r int
			for _, c := range d.Changes {
				if isFoldedChild(d.Changes, c) {
					continue
				}
				switch c.Kind {
				case gen.Added:
					a++
				case gen.Removed:
					r++
				default:
					mo++
				}
			}
			if a+mo+r > 0 {
				add(in + "  " + counts("Files", a, mo, r))
			}
			if pa, pr := len(d.Extra.PkgAdded), len(d.Extra.PkgRemoved); pa+pr > 0 {
				add(in + "  " + counts("Packages", pa, -1, pr))
			}
			risky := 0
			for _, s := range d.Extra.StartupAdded {
				if s.Risk == "remote-exec" {
					risky++
				}
			}
			if sa, sr := len(d.Extra.StartupAdded), len(d.Extra.StartupRemoved); sa+sr > 0 {
				line := counts("Startup", sa, -1, sr)
				if risky > 0 {
					line += "   " + sDanger.Render(fmt.Sprintf("[!] %d new item(s) download and run code", risky))
				}
				add(in + "  " + line)
			}
		}
		add("")
	}

	add(in + sBold.Render("Health"))
	p, n := d.Problems(), len(d.Findings)-d.Problems()
	switch {
	case len(d.Findings) == 0:
		add(in + "  No problems found.")
	default:
		add(in + fmt.Sprintf("  %s, %s   %s", plural(p, "problem"), plural(n, "note"), sMuted.Render("see tab 5")))
	}
	add("")

	add(in + sBold.Render("What you can do"))
	switch {
	case !d.Initialized:
		add(in + "  " + keyHint("4", "see what runs automatically when you log in"))
		add(in + "  " + keyHint("5", "check your configuration for problems"))
	case d.ReadOnly:
		add(in + "  " + keyHint("2", "review the changes", "4", "startup", "5", "health"))
		add(in + "  " + sMuted.Render("Saving and going back are disabled on your real home (read-only)."))
	case d.Unsaved() > 0:
		add(in + "  " + keyHint("s", "save the current state as a new save point"))
		add(in + "  " + keyHint("u", fmt.Sprintf("discard these changes and go back to #%d", d.Head.ID)))
		add(in + "  " + keyHint("2", "review the changes one by one"))
	default:
		add(in + "  " + keyHint("s", "save a point now, before trying new dotfiles"))
		add(in + "  " + sMuted.Render("Then run the installer with: dotka run -- ./install.sh"))
		add(in + "  " + keyHint("3", "browse your save points"))
	}
	return out
}

func counts(label string, a, mo, r int) string {
	s := fmt.Sprintf("%-10s", label) + sAdded.Render(fmt.Sprintf("+%-4d", a))
	if mo >= 0 {
		s += sModified.Render(fmt.Sprintf("~%-4d", mo))
	} else {
		s += "     "
	}
	return s + sRemoved.Render(fmt.Sprintf("-%d", r))
}

func ago(g gen.Generation) string {
	return "saved " + g.CreatedAt.Local().Format("Jan 02 15:04")
}

// ---- list + detail ----------------------------------------------------------------

func (m Model) listWidth() int {
	lw := m.width * 9 / 20
	if lw < 32 {
		lw = 32
	}
	if lw > 60 {
		lw = 60
	}
	return lw
}

func (m Model) split(h int) []string {
	lw := m.listWidth()
	rw := m.width - lw - 3
	rows := m.rows[m.tab]

	listTitle := map[tab]string{
		tabChanges: "Unsaved changes", tabHistory: "Save points (newest first)",
		tabStartup: "Runs automatically", tabHealth: "Health check",
	}[m.tab]
	dTitle, dLines := m.detail(rw)
	if m.focusDetail {
		listTitle, dTitle = sMuted.Render(listTitle), sAccent.Render("> "+dTitle)
	} else {
		listTitle, dTitle = sAccent.Render("> "+listTitle), sBold.Render(dTitle)
	}

	if len(rows) == 0 {
		return append([]string{" " + listTitle, ""}, m.emptyState(m.width-2)...)
	}

	// Left: list window around the cursor (offset kept in Update).
	lh := m.listHeight()
	off := window(m.cursor[m.tab], m.offset[m.tab], len(rows), lh)
	left := []string{listTitle, ""}
	for i := off; i < off+lh && i < len(rows); i++ {
		r := rows[i]
		switch {
		case r.header:
			left = append(left, sBold.Render(r.text))
		case i == m.cursor[m.tab] && !m.focusDetail:
			left = append(left, sSelected.Render(fit("> "+stripStyle(r.text), lw-1)))
		case i == m.cursor[m.tab]:
			left = append(left, "> "+r.text)
		default:
			left = append(left, "  "+r.text)
		}
	}
	if more := len(rows) - (off + lh); more > 0 {
		left = append(left, sMuted.Render(fmt.Sprintf("  ... %d more below", more)))
	} else if off > 0 {
		left = append(left, sMuted.Render(fmt.Sprintf("  ... %d more above", off)))
	}

	// Right: detail with its own scroll.
	right := []string{dTitle, ""}
	dh := h - 2
	doff := m.detailOff
	if max := len(dLines) - dh; doff > max {
		doff = max
	}
	if doff < 0 {
		doff = 0
	}
	end := doff + dh
	if end > len(dLines) {
		end = len(dLines)
	}
	right = append(right, dLines[doff:end]...)
	if end < len(dLines) {
		right[len(right)-1] = sMuted.Render("... more below (tab, then j/k to scroll)")
	}

	sep := sMuted.Render(" | ")
	out := make([]string, h)
	for i := range out {
		l, r := "", ""
		if i < len(left) {
			l = left[i]
		}
		if i < len(right) {
			r = right[i]
		}
		out[i] = fit(" "+l, lw) + sep + fit(r, rw)
	}
	return out
}

func (m Model) emptyState(w int) []string {
	d := m.data
	var msg string
	switch m.tab {
	case tabChanges:
		if !d.Initialized {
			msg = "Dotka is not set up here yet, so there is nothing to compare against. Go to tab 1 to set it up."
		} else {
			msg = fmt.Sprintf("No unsaved changes: your configuration matches save point #%d.", d.Head.ID)
		}
	case tabHistory:
		msg = "No save points yet. Go to tab 1 to set dotka up."
	case tabStartup:
		msg = "Nothing found that runs automatically (Hyprland, autostart, user services, shell)."
	case tabHealth:
		msg = "No problems found. Your startup programs exist, links are fine and secrets are private."
	}
	var out []string
	for _, l := range wrap(msg, w-2) {
		out = append(out, "   "+l)
	}
	return out
}

// stripStyle removes ANSI styling so a selected row is uniformly reversed.
func stripStyle(s string) string {
	var b strings.Builder
	in := false
	for _, r := range s {
		switch {
		case r == '\x1b':
			in = true
		case in && r == 'm':
			in = false
		case !in:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// ---- modals --------------------------------------------------------------------------

func (m Model) modalView(h int) []string {
	bw := m.width - 8
	if bw > 72 {
		bw = 72
	}
	iw := bw - 4
	var title string
	var body []string
	add := func(s ...string) { body = append(body, s...) }
	p := func(s string) { add(wrap(s, iw)...) }

	switch m.modal {
	case modalHelp:
		title = "Keys"
		rowsK := [][2]string{
			{"1-5  left/right", "switch tab"},
			{"j/k  up/down", "move in the list (or scroll the detail)"},
			{"tab  enter", "move between the list and the detail"},
			{"esc", "back to the list / close"},
			{"s", "save the current state as a save point"},
			{"u", "discard unsaved changes"},
			{"b", "History: go back to the selected save point"},
			{"x", "History: forget the selected save point"},
			{"i", "set dotka up (first time)"},
			{"r", "refresh"},
			{"q  ctrl+c", "quit"},
		}
		for _, r := range rowsK {
			add(sKey.Render(fmt.Sprintf("%-16s", r[0])) + " " + r[1])
		}
		add("", sMuted.Render("Every change asks first, and going back can always be undone."))
	case modalSave:
		title = "Save a point"
		p("Describe this save point (optional), for example: before trying X's dots")
		add("", "> "+string(m.input)+"_", "")
		add(keyHint("enter", "save", "esc", "cancel"))
	case modalInit:
		title = "Set up dotka"
		p("Dotka will create its private store in:")
		add("  "+sMuted.Render(m.data.Home+"/.local/share/dotka"), "")
		p("and save your current configuration as save point #1. Nothing in your configuration is changed.")
		add("", keyHint("y", "set up", "n", "cancel"))
	case modalForget:
		title = fmt.Sprintf("Forget save point #%d?", m.target)
		p("You will no longer be able to go back to it, and its stored copies are deleted if no other save point uses them. Your files are not touched.")
		add("", keyHint("y", "forget", "n", "cancel"))
	case modalRollback:
		title = m.rollbackTitle()
		body = m.rollbackBody(iw)
	}

	box := []string{"+" + strings.Repeat("-", bw-2) + "+"}
	box = append(box, "| "+fit(sBold.Render(title), iw)+" |", "| "+strings.Repeat(" ", iw)+" |")
	maxBody := h - 4
	if len(body) > maxBody {
		// Keep the last line: it holds the keys to confirm or cancel.
		last := body[len(body)-1]
		body = append(body[:maxBody-2], sMuted.Render("..."), last)
	}
	for _, l := range body {
		box = append(box, "| "+fit(l, iw)+" |")
	}
	box = append(box, "+"+strings.Repeat("-", bw-2)+"+")
	return center(h, m.width, box)
}

func (m Model) rollbackTitle() string {
	if m.discard {
		return fmt.Sprintf("Discard unsaved changes (back to #%d)?", m.target)
	}
	t := fmt.Sprintf("Go back to save point #%d", m.target)
	if m.plan != nil && m.plan.Target.Message != "" {
		t += fmt.Sprintf(" %q", m.plan.Target.Message)
	}
	return t + "?"
}

func (m Model) rollbackBody(iw int) []string {
	var out []string
	add := func(s ...string) { out = append(out, s...) }
	p := func(s string) { add(wrap(s, iw)...) }
	if m.plan == nil {
		return out
	}
	add(sMuted.Render("Your current state is saved first, so this can be undone."))
	add("")
	cnt := map[string]int{}
	for _, a := range m.plan.Actions {
		cnt[a.Kind]++
	}
	add(sBold.Render("Files"))
	if len(m.plan.Actions) == 0 {
		add("  nothing to change in your tracked files")
	} else {
		if n := cnt[gen.ActWrite] + cnt[gen.ActSymlink] + cnt[gen.ActChmod]; n > 0 {
			add("  " + sModified.Render("~") + " restore " + plural(n, "file"))
		}
		if n := cnt[gen.ActRemove]; n > 0 {
			add("  " + sRemoved.Render("-") + " remove " + plural(n, "file") + " added since then")
		}
	}
	if n := len(m.plan.Skipped); n > 0 {
		add(sMuted.Render(fmt.Sprintf("  %s left untouched (secrets or files dotka could not restore later)", plural(n, "item"))))
	}
	x := m.planX
	if len(x.StartupAdded)+len(x.StartupRemoved) > 0 {
		add("", sBold.Render("Runs at startup"))
		if n := len(x.StartupRemoved); n > 0 {
			add(fmt.Sprintf("  %s will no longer run", plural(n, "item")))
		}
		if n := len(x.StartupAdded); n > 0 {
			add(fmt.Sprintf("  %s will run again", plural(n, "item")))
		}
	}
	if len(x.PkgRemoved)+len(x.PkgAdded) > 0 {
		add("", sBold.Render("Packages"))
		p("Dotka never changes packages. If you want, remove the ones installed since then yourself:")
		if len(x.PkgRemoved) > 0 {
			add(fit("  sudo pacman -Rns "+strings.Join(x.PkgRemoved, " "), iw))
		}
	}
	add("", keyHint("y", "go back", "n", "cancel"))
	return out
}

// ---- status + footer -----------------------------------------------------------------

func (m Model) statusLine() string {
	var s string
	switch {
	case m.busy != "":
		s = sAccent.Render(m.busy)
	case m.loading && m.data != nil:
		s = sMuted.Render("Refreshing...")
	case m.status != "" && m.statusOK:
		s = sAdded.Render(m.status)
	case m.status != "":
		s = sDanger.Render(m.status)
	}
	if s == "" {
		return rule(m.width)
	}
	return sMuted.Render("-- ") + s + " " + rule(m.width)
}

func (m Model) footer() string {
	switch {
	case m.modal == modalSave:
		return " " + keyHint("enter", "save", "esc", "cancel")
	case m.modal == modalHelp:
		return " " + keyHint("any key", "close")
	case m.modal != modalNone:
		return " " + keyHint("y", "confirm", "n", "cancel")
	case m.data == nil:
		return " " + keyHint("q", "quit")
	}
	var hints []string
	switch {
	case m.focusDetail:
		hints = []string{"j/k", "scroll", "tab/esc", "back to list"}
	case m.tab == tabOverview:
		hints = []string{"1-5", "tabs"}
		if m.data.ReadOnly {
			// no write actions to advertise
		} else if m.data.Initialized {
			hints = append(hints, "s", "save", "u", "discard changes")
		} else if !m.data.ReadOnly {
			hints = append(hints, "i", "set up")
		}
	case m.tab == tabHistory && !m.data.ReadOnly:
		hints = []string{"j/k", "move", "b", "go back here", "x", "forget"}
	case m.tab == tabChanges && !m.data.ReadOnly:
		hints = []string{"j/k", "move", "tab", "detail", "s", "save", "u", "discard all"}
	default:
		hints = []string{"j/k", "move", "tab", "detail"}
	}
	hints = append(hints, "r", "refresh", "?", "help", "q", "quit")
	return " " + keyHint(hints...)
}
