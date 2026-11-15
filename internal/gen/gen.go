// Package gen implements generations: immutable, numbered states of the user
// environment that can be listed, compared and restored.
package gen

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/iYeremy/dotka/internal/config"
	"github.com/iYeremy/dotka/internal/scan"
	"github.com/iYeremy/dotka/internal/store"
)

// Origins of a generation.
const (
	OriginInit         = "init"
	OriginManual       = "manual"
	OriginPreRollback  = "auto: before rollback"
	OriginPostRollback = "rollback"
	OriginPreRun       = "auto: before run"
	OriginRun          = "run"
)

// Generation is the manifest stored for each saved state.
type Generation struct {
	ID        int       `json:"id"`
	Message   string    `json:"message,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	Origin    string    `json:"origin"`
	// Parent is the generation the home was based on when this one was
	// taken (HEAD at that moment). 0 for the first one and old manifests.
	Parent  int          `json:"parent,omitempty"`
	Run     *RunInfo     `json:"run,omitempty"`
	Roots   []string     `json:"roots"`
	Entries []scan.Entry `json:"entries"`
	Extra
}

// RunInfo records the command that produced a generation (`dotka run`).
type RunInfo struct {
	Command  []string `json:"command"`
	Dir      string   `json:"dir"`
	ExitCode int      `json:"exit_code"`
	Before   int      `json:"before"` // generation taken right before running it
}

// Display returns the command in a short, readable form: the program path
// is made relative to the directory it was run from, or abbreviated.
func (r RunInfo) Display() string {
	if len(r.Command) == 0 {
		return ""
	}
	return strings.Join(append([]string{ShortPath(r.Command[0], r.Dir)}, r.Command[1:]...), " ")
}

// ShortPath abbreviates a program path for display.
func ShortPath(p, dir string) string {
	if !filepath.IsAbs(p) {
		return p
	}
	if dir != "" {
		if rel, err := filepath.Rel(dir, p); err == nil && !strings.HasPrefix(rel, "..") {
			return "./" + rel
		}
	}
	parts := strings.Split(p, "/")
	if len(parts) > 3 {
		return ".../" + strings.Join(parts[len(parts)-2:], "/")
	}
	return p
}

// State is the current (unsaved) state of the environment.
type State struct {
	Roots    []string
	Entries  []scan.Entry
	Warnings []scan.Warning
	Extra
}

// Collector gathers the non-file parts of a state (packages, startup map...).
// It is a hook so the core does not depend on pacman or Hyprland.
type Collector func(home string, entries []scan.Entry) (Extra, []scan.Warning)

// Manager ties a home directory, its store and config together.
type Manager struct {
	Home      string
	Store     *store.Store
	Config    config.Config
	Collector Collector
	Now       func() time.Time
}

func (m *Manager) now() time.Time {
	if m.Now != nil {
		return m.Now()
	}
	return time.Now()
}

func (m *Manager) previousIndex() map[string]scan.Entry {
	head, err := m.Store.Head()
	if err != nil {
		return nil
	}
	g, err := m.Load(head)
	if err != nil {
		return nil
	}
	return scan.Index(g.Entries)
}

func (m *Manager) collect(entries []scan.Entry) (Extra, []scan.Warning) {
	if m.Collector == nil {
		return Extra{}, nil
	}
	return m.Collector(m.Home, entries)
}

// Current scans the home without writing to the store.
func (m *Manager) Current() (State, error) {
	res, err := scan.Scan(scan.Options{Home: m.Home, Config: m.Config, Previous: m.previousIndex()})
	if err != nil {
		return State{}, err
	}
	extra, w := m.collect(res.Entries)
	return State{Roots: m.Config.Roots, Entries: res.Entries, Warnings: append(res.Warnings, w...), Extra: extra}, nil
}

// Snapshot scans the home, stores every file and saves a new generation,
// which becomes HEAD. The caller must hold the store lock.
func (m *Manager) Snapshot(message, origin string) (Generation, []scan.Warning, error) {
	return m.SnapshotRun(message, origin, nil)
}

// SnapshotRun is Snapshot with information about the command that produced
// the changes.
func (m *Manager) SnapshotRun(message, origin string, run *RunInfo) (Generation, []scan.Warning, error) {
	res, err := scan.Scan(scan.Options{Home: m.Home, Config: m.Config, Store: m.Store, Previous: m.previousIndex()})
	if err != nil {
		return Generation{}, nil, err
	}
	extra, w := m.collect(res.Entries)
	id, err := m.Store.NextID()
	if err != nil {
		return Generation{}, nil, err
	}
	parent, _ := m.Store.Head()
	g := Generation{
		ID: id, Message: message, CreatedAt: m.now(), Origin: origin, Parent: parent, Run: run,
		Roots: append([]string(nil), m.Config.Roots...), Entries: res.Entries, Extra: extra,
	}
	if err := m.Store.SaveGeneration(id, g); err != nil {
		return Generation{}, nil, err
	}
	if err := m.Store.SetHead(id); err != nil {
		return Generation{}, nil, err
	}
	m.Store.AppendLog(store.LogEntry{Op: "snap", Gen: id, Result: "ok", Details: message})
	return g, append(res.Warnings, w...), nil
}

// Unsaved reports whether the home differs from the current generation.
func (m *Manager) Unsaved() (bool, error) {
	head, err := m.Store.Head()
	if err != nil {
		return false, err
	}
	base, err := m.Load(head)
	if err != nil {
		return false, err
	}
	cur, err := m.Current()
	if err != nil {
		return false, err
	}
	changes := FilterRoots(Diff(base.Entries, cur.Entries), base.Roots, cur.Roots)
	return len(changes) > 0 || !DiffExtra(base.Extra, cur.Extra).Empty(), nil
}

// Load reads a generation.
func (m *Manager) Load(id int) (Generation, error) {
	var g Generation
	err := m.Store.LoadGeneration(id, &g)
	return g, err
}

// List returns all generations (manifests loaded) in ascending order.
func (m *Manager) List() ([]Generation, error) {
	ids, err := m.Store.GenerationIDs()
	if err != nil {
		return nil, err
	}
	out := make([]Generation, 0, len(ids))
	for _, id := range ids {
		g, err := m.Load(id)
		if err != nil {
			return nil, fmt.Errorf("generation %d: %w", id, err)
		}
		out = append(out, g)
	}
	return out, nil
}

// LiveObjects returns every object hash referenced by any generation.
func (m *Manager) LiveObjects() (map[string]bool, error) {
	gens, err := m.List()
	if err != nil {
		return nil, err
	}
	keep := map[string]bool{}
	for _, g := range gens {
		for _, e := range g.Entries {
			if e.Stored {
				keep[e.Hash] = true
			}
		}
	}
	return keep, nil
}

// ---- Diff -----------------------------------------------------------------

// Change kinds.
const (
	Added    = "added"
	Removed  = "removed"
	Modified = "modified"
	TypeChg  = "type-changed"
	ModeChg  = "mode-changed"
)

// Change is one difference between two sets of entries.
type Change struct {
	Path string
	Kind string
	Old  *scan.Entry
	New  *scan.Entry
}

// Diff compares two sets of entries. Directory mode changes are reported;
// directory mtimes are ignored.
func Diff(oldE, newE []scan.Entry) []Change {
	oi, ni := scan.Index(oldE), scan.Index(newE)
	var out []Change
	for p, o := range oi {
		o := o
		n, ok := ni[p]
		if !ok {
			out = append(out, Change{Path: p, Kind: Removed, Old: &o})
			continue
		}
		n2 := n
		switch {
		case o.Type != n.Type:
			out = append(out, Change{Path: p, Kind: TypeChg, Old: &o, New: &n2})
		case !o.SameContent(n):
			out = append(out, Change{Path: p, Kind: Modified, Old: &o, New: &n2})
		case o.Mode != n.Mode && o.Type != scan.TypeSymlink:
			out = append(out, Change{Path: p, Kind: ModeChg, Old: &o, New: &n2})
		}
	}
	for p, n := range ni {
		if _, ok := oi[p]; !ok {
			n := n
			out = append(out, Change{Path: p, Kind: Added, New: &n})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

// FilterRoots keeps only the changes under roots that are tracked in both
// states, so that adding a new root does not look like "everything added".
func FilterRoots(changes []Change, oldRoots, newRoots []string) []Change {
	var out []Change
	for _, c := range changes {
		if config.UnderRoots(oldRoots, c.Path) && config.UnderRoots(newRoots, c.Path) {
			out = append(out, c)
		}
	}
	return out
}
