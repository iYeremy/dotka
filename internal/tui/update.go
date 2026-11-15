package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/iYeremy/dotka/internal/gen"
	"github.com/iYeremy/dotka/internal/scan"
)

// Update handles a message and returns the new state.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil

	case loadedMsg:
		m.loading, m.busy = false, ""
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.err, m.data = nil, msg.data
		m.patches = map[string][]string{}
		m.compare = map[int][3]int{}
		m.buildRows()
		for t := tab(0); t < numTabs; t++ {
			m.cursor[t] = m.clampCursor(t, m.cursor[t], 1)
			m.offset[t] = 0
		}
		return m, m.afterMove()

	case patchMsg:
		m.patches[msg.path] = msg.lines
		return m, nil

	case previewMsg:
		m.busy = ""
		if msg.err != nil {
			m.setStatus(msg.err.Error(), false)
			return m, nil
		}
		m.plan, m.planX, m.target, m.modal = &msg.plan, msg.extra, msg.id, modalRollback
		return m, nil

	case actionMsg:
		m.busy = ""
		if msg.err != nil {
			m.setStatus(msg.err.Error(), false)
			return m, nil
		}
		m.setStatus(msg.text, true)
		m.loading = true
		return m, loadCmd(m.be)

	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m *Model) setStatus(s string, ok bool) { m.status, m.statusOK = s, ok }

func (m Model) handleKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	// A lone Esc followed by another key is reported by terminals as
	// Alt+key. Nothing here is bound to Alt, so treat it as Esc, then key.
	if k.Alt && k.Type == tea.KeyRunes {
		nm, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyEsc})
		k.Alt = false
		return nm.(Model).handleKey(k)
	}
	key := k.String()
	if key == "ctrl+c" {
		return m, tea.Quit
	}
	if m.busy != "" {
		return m, nil // an action is running; only ctrl+c works
	}
	if m.modal != modalNone {
		return m.handleModal(k)
	}
	m.status = ""

	switch key {
	case "q":
		return m, tea.Quit
	case "?":
		m.modal = modalHelp
		return m, nil
	case "r":
		if !m.loading {
			m.loading = true
			return m, loadCmd(m.be)
		}
		return m, nil
	case "1", "2", "3", "4", "5":
		return m.switchTab(tab(key[0] - '1'))
	case "right", "l":
		return m.switchTab((m.tab + 1) % numTabs)
	case "left", "h":
		return m.switchTab((m.tab + numTabs - 1) % numTabs)
	}
	if m.data == nil {
		return m, nil
	}

	switch key {
	case "tab", "enter":
		if m.tab != tabOverview && len(m.rows[m.tab]) > 0 {
			m.focusDetail = !m.focusDetail || key == "enter"
			m.detailOff = 0
		}
		return m, nil
	case "esc":
		m.focusDetail = false
		return m, nil
	case "up", "k":
		return m.move(-1)
	case "down", "j":
		return m.move(1)
	case "pgup":
		return m.move(-m.bodyHeight() / 2)
	case "pgdown", " ":
		return m.move(m.bodyHeight() / 2)
	case "g", "home":
		return m.move(-1 << 30)
	case "G", "end":
		return m.move(1 << 30)
	}
	return m.handleAction(key)
}

func (m Model) switchTab(t tab) (tea.Model, tea.Cmd) {
	m.tab, m.focusDetail, m.detailOff = t, false, 0
	return m, m.afterMove()
}

func (m Model) move(delta int) (tea.Model, tea.Cmd) {
	if m.focusDetail {
		m.detailOff += delta
		if m.detailOff < 0 {
			m.detailOff = 0
		}
		return m, nil // clamped against content height in View
	}
	if m.tab == tabOverview {
		return m, nil
	}
	m.cursor[m.tab] = m.clampCursor(m.tab, m.cursor[m.tab]+delta, sign(delta))
	m.offset[m.tab] = window(m.cursor[m.tab], m.offset[m.tab], len(m.rows[m.tab]), m.listHeight())
	m.detailOff = 0
	return m, m.afterMove()
}

func sign(n int) int {
	if n < 0 {
		return -1
	}
	return 1
}

// clampCursor keeps the cursor in range and off header rows, moving in dir.
func (m Model) clampCursor(t tab, c, dir int) int {
	rows := m.rows[t]
	if len(rows) == 0 {
		return 0
	}
	if c < 0 {
		c, dir = 0, 1
	}
	if c >= len(rows) {
		c, dir = len(rows)-1, -1
	}
	for i := c; i >= 0 && i < len(rows); i += dir {
		if !rows[i].header {
			return i
		}
	}
	for i := c; i >= 0 && i < len(rows); i -= dir {
		if !rows[i].header {
			return i
		}
	}
	return c
}

// selected returns the row under the cursor of the current tab.
func (m Model) selected() *row {
	rows := m.rows[m.tab]
	c := m.cursor[m.tab]
	if c < 0 || c >= len(rows) || rows[c].header {
		return nil
	}
	return &rows[c]
}

// afterMove requests data the newly selected row needs (file diffs) and
// computes cheap in-memory derived values (history comparisons).
func (m *Model) afterMove() tea.Cmd {
	r := m.selected()
	if r == nil {
		return nil
	}
	if r.gen != nil {
		if _, ok := m.compare[r.gen.ID]; !ok {
			m.compare[r.gen.ID] = m.compareWithNow(*r.gen)
		}
	}
	if r.change != nil {
		if _, ok := m.patches[r.change.Path]; !ok {
			e := r.change.New
			if e == nil {
				e = r.change.Old
			}
			if e.Type == scan.TypeFile {
				m.patches[r.change.Path] = nil // requested
				return patchCmd(m.be, *r.change)
			}
		}
	}
	return nil
}

// compareWithNow counts the differences between a generation and the
// current home, using the Head entries plus the unsaved changes.
func (m Model) compareWithNow(g gen.Generation) [3]int {
	cur := scan.Index(m.data.Head.Entries)
	for _, c := range m.data.Changes {
		if c.New == nil {
			delete(cur, c.Path)
		} else {
			cur[c.Path] = *c.New
		}
	}
	now := make([]scan.Entry, 0, len(cur))
	for _, e := range cur {
		now = append(now, e)
	}
	var out [3]int
	for _, c := range gen.FilterRoots(gen.Diff(g.Entries, now), g.Roots, m.data.Head.Roots) {
		e := c.New
		if e == nil {
			e = c.Old
		}
		if e.Type == scan.TypeDir {
			continue
		}
		switch c.Kind {
		case gen.Added:
			out[0]++
		case gen.Removed:
			out[2]++
		default:
			out[1]++
		}
	}
	return out
}

// ---- actions ------------------------------------------------------------------

func (m Model) handleAction(key string) (tea.Model, tea.Cmd) {
	d := m.data
	needWrite := func() bool {
		if !d.Initialized {
			m.setStatus("Dotka is not set up here yet: press i to create your first save point.", false)
			return false
		}
		return true
	}
	switch key {
	case "i":
		if d.Initialized {
			return m, nil
		}
		m.modal = modalInit
	case "s":
		if needWrite() {
			m.modal, m.input = modalSave, nil
		}
	case "u":
		if !needWrite() {
			return m, nil
		}
		if d.Unsaved() == 0 {
			m.setStatus("Nothing to discard: everything matches your current save point.", true)
			return m, nil
		}
		m.discard, m.busy = true, "Preparing the plan..."
		return m, previewCmd(m.be, d.Head.ID)
	case "b":
		r := m.selected()
		if m.tab != tabHistory || r == nil || r.gen == nil || !needWrite() {
			return m, nil
		}
		m.discard, m.busy = false, "Preparing the plan..."
		return m, previewCmd(m.be, r.gen.ID)
	case "x":
		r := m.selected()
		if m.tab != tabHistory || r == nil || r.gen == nil || !needWrite() {
			return m, nil
		}
		if r.gen.ID == d.Head.ID {
			m.setStatus("This is your current save point; it cannot be forgotten.", false)
			return m, nil
		}
		m.target, m.modal = r.gen.ID, modalForget
	}
	return m, nil
}

func (m Model) handleModal(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := k.String()
	switch m.modal {
	case modalHelp:
		m.modal = modalNone
		return m, nil
	case modalSave:
		switch k.Type {
		case tea.KeyEsc:
			m.modal = modalNone
		case tea.KeyEnter:
			msg := strings.TrimSpace(string(m.input))
			m.modal, m.busy = modalNone, "Saving..."
			return m, actionCmd(func() (string, error) { return m.be.Snap(msg) })
		case tea.KeyBackspace:
			if len(m.input) > 0 {
				m.input = m.input[:len(m.input)-1]
			}
		case tea.KeyRunes, tea.KeySpace:
			if len(m.input) < 120 {
				m.input = append(m.input, k.Runes...)
			}
		}
		return m, nil
	}
	// Confirmation modals: y confirms, anything else cancels.
	confirmed := key == "y" || key == "Y"
	kind, target := m.modal, m.target
	m.modal, m.plan = modalNone, nil
	if !confirmed {
		m.setStatus("Cancelled. Nothing was changed.", true)
		return m, nil
	}
	switch kind {
	case modalInit:
		m.busy = "Setting up dotka..."
		return m, actionCmd(m.be.Init)
	case modalRollback:
		m.busy = "Going back..."
		return m, actionCmd(func() (string, error) { return m.be.Rollback(target) })
	case modalForget:
		m.busy = "Forgetting..."
		return m, actionCmd(func() (string, error) { return m.be.Forget(target) })
	}
	return m, nil
}
