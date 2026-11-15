// Package tui is the interactive terminal interface of dotka.
//
// Architecture: Backend (all I/O) -> tea.Cmd -> messages -> Model.Update
// (pure state changes) -> Model.View (pure rendering from state + size).
package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/iYeremy/dotka/internal/doctor"
	"github.com/iYeremy/dotka/internal/gen"
)

// Minimum terminal size; below it a polite message is shown instead.
const (
	minWidth  = 70
	minHeight = 20
)

type tab int

const (
	tabOverview tab = iota
	tabChanges
	tabHistory
	tabStartup
	tabHealth
	numTabs
)

type modal int

const (
	modalNone modal = iota
	modalHelp
	modalSave
	modalInit
	modalRollback
	modalForget
)

// row is one line of a list pane. Headers are not selectable.
type row struct {
	text    string
	header  bool
	change  *gen.Change
	pkg     string
	added   bool // package/startup: added (true) or removed (false)
	startup *gen.StartupItem
	gen     *gen.Generation
	finding *doctor.Finding
}

// Model is the whole UI state.
type Model struct {
	be Backend

	width, height int
	tab           tab
	focusDetail   bool
	cursor        [numTabs]int
	offset        [numTabs]int
	detailOff     int

	data    *Data
	rows    [numTabs][]row
	loading bool
	err     error

	patches map[string][]string // file change details, keyed by path
	compare map[int][3]int      // history: +added ~modified -removed vs now

	modal    modal
	input    []rune
	plan     *gen.Plan
	planX    gen.ExtraDiff
	target   int  // generation of a pending rollback/forget
	discard  bool // the pending rollback discards unsaved changes
	busy     string
	status   string
	statusOK bool
}

// New creates the model. Call Init through tea.NewProgram.
func New(be Backend) Model {
	return Model{be: be, loading: true, patches: map[string][]string{}, compare: map[int][3]int{}}
}

// ---- messages ---------------------------------------------------------------

type loadedMsg struct {
	data *Data
	err  error
}

type patchMsg struct {
	path  string
	lines []string
}

type previewMsg struct {
	id    int
	plan  gen.Plan
	extra gen.ExtraDiff
	err   error
}

type actionMsg struct {
	text string
	err  error
}

// ---- commands (all I/O happens here, never in Update or View) ---------------

func loadCmd(be Backend) tea.Cmd {
	return func() tea.Msg {
		d, err := be.Load()
		return loadedMsg{d, err}
	}
}

func patchCmd(be Backend, c gen.Change) tea.Cmd {
	return func() tea.Msg { return patchMsg{c.Path, be.Patch(c)} }
}

func previewCmd(be Backend, id int) tea.Cmd {
	return func() tea.Msg {
		p, x, err := be.Preview(id)
		return previewMsg{id, p, x, err}
	}
}

func actionCmd(f func() (string, error)) tea.Cmd {
	return func() tea.Msg {
		t, err := f()
		return actionMsg{t, err}
	}
}

// Init starts loading the data.
func (m Model) Init() tea.Cmd { return loadCmd(m.be) }
