package tui

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/iYeremy/dotka/internal/collect"
	"github.com/iYeremy/dotka/internal/config"
	"github.com/iYeremy/dotka/internal/doctor"
	"github.com/iYeremy/dotka/internal/gen"
	"github.com/iYeremy/dotka/internal/scan"
	"github.com/iYeremy/dotka/internal/startup"
	"github.com/iYeremy/dotka/internal/store"
	"github.com/iYeremy/dotka/internal/textdiff"
)

// Data is everything the TUI shows. It is produced by a Backend and never
// mutated by the views.
type Data struct {
	Home        string
	StoreDir    string
	Sandbox     bool // home is not the user's real home
	Initialized bool // a store exists

	Head    gen.Generation   // current generation (zero if not initialized)
	Gens    []gen.Generation // newest first
	Changes []gen.Change     // home vs Head (tracked files)
	Extra   gen.ExtraDiff    // packages / startup vs Head

	Startup  []gen.StartupItem
	Findings []doctor.Finding
}

// Unsaved is the number of pending changes shown in the header.
func (d *Data) Unsaved() int {
	n := 0
	for _, c := range d.Changes {
		if !isFoldedChild(d.Changes, c) {
			n++
		}
	}
	return n + len(d.Extra.PkgAdded) + len(d.Extra.PkgRemoved) +
		len(d.Extra.StartupAdded) + len(d.Extra.StartupRemoved)
}

// Problems counts doctor findings with severity "problem".
func (d *Data) Problems() int {
	n := 0
	for _, f := range d.Findings {
		if f.Severity == doctor.Problem {
			n++
		}
	}
	return n
}

// Backend performs all I/O for the TUI. The model only talks to it through
// tea.Cmds, so rendering never touches the disk.
type Backend interface {
	Load() (*Data, error)
	// Patch returns the detail lines for a file change (diff or preview).
	Patch(c gen.Change) []string
	Snap(message string) (string, error)
	Init() (string, error)
	Preview(id int) (gen.Plan, gen.ExtraDiff, error)
	Rollback(id int) (string, error)
	Forget(id int) (string, error)
}

// Real is the production Backend.
type Real struct {
	Home     string
	StoreDir string
	RealHome bool
	// Collector gathers packages and the startup map for generations.
	// Defaults to collect.Extra; tests may replace it.
	Collector gen.Collector
}

func (r *Real) config() config.Config {
	cfg, err := config.Load(r.StoreDir)
	if err != nil {
		cfg = config.Default()
	}
	if rel, err := filepath.Rel(r.Home, r.StoreDir); err == nil && !strings.HasPrefix(rel, "..") {
		cfg.Exclude = append(cfg.Exclude, rel)
	}
	return cfg
}

func (r *Real) manager() (*gen.Manager, error) {
	s, err := store.Open(r.StoreDir)
	if err != nil {
		return nil, err
	}
	c := r.Collector
	if c == nil {
		c = collect.Extra
	}
	return &gen.Manager{Home: r.Home, Store: s, Config: r.config(), Collector: c}, nil
}

// Load reads the current state. It only reads.
func (r *Real) Load() (*Data, error) {
	d := &Data{Home: r.Home, StoreDir: r.StoreDir, Sandbox: !r.RealHome}
	d.Startup, _ = startup.Extract(r.Home)
	in, err := doctor.Collect(r.Home, r.config())
	if err != nil {
		return nil, err
	}
	in.Startup = d.Startup
	d.Findings = doctor.Run(in)

	m, err := r.manager()
	if errors.Is(err, store.ErrNotInitialized) {
		return d, nil
	}
	if err != nil {
		return nil, err
	}
	d.Initialized = true
	gens, err := m.List()
	if err != nil {
		return nil, err
	}
	for i := len(gens) - 1; i >= 0; i-- {
		d.Gens = append(d.Gens, gens[i])
	}
	head, err := m.Store.Head()
	if err != nil {
		return nil, err
	}
	if d.Head, err = m.Load(head); err != nil {
		return nil, err
	}
	cur, err := m.Current()
	if err != nil {
		return nil, err
	}
	d.Changes = gen.FilterRoots(gen.Diff(d.Head.Entries, cur.Entries), d.Head.Roots, cur.Roots)
	d.Extra = gen.DiffExtra(d.Head.Extra, cur.Extra)
	return d, nil
}

const previewLines = 200

// Patch builds the detail of a file change: a unified diff for modified
// text files, a preview for added/removed ones.
func (r *Real) Patch(c gen.Change) []string {
	en := c.New
	if en == nil {
		en = c.Old
	}
	if en.Skip == scan.SkipSecret {
		return []string{"Content hidden: this file looks like a secret and is never stored or shown."}
	}
	if en.Type != scan.TypeFile || (c.Old != nil && c.Old.Type != scan.TypeFile) {
		return nil
	}
	m, err := r.manager()
	if err != nil {
		return []string{"Could not open the store: " + err.Error()}
	}
	var oldB, newB []byte
	if c.Old != nil {
		if !c.Old.Stored {
			return []string{"The previous version was not stored (too large), so no diff is available."}
		}
		if oldB, err = m.Store.ReadObject(c.Old.Hash); err != nil {
			return []string{"Could not read the stored version: " + err.Error()}
		}
	}
	if c.New != nil {
		if newB, err = os.ReadFile(filepath.Join(r.Home, c.Path)); err != nil {
			return []string{"Could not read the file: " + err.Error()}
		}
	}
	if textdiff.IsBinary(oldB) || textdiff.IsBinary(newB) {
		return []string{"Binary file: no text preview."}
	}
	var out string
	switch c.Kind {
	case gen.Added:
		out = prefixLines(string(newB), "+")
	case gen.Removed:
		out = prefixLines(string(oldB), "-")
	default:
		out = textdiff.Unified("saved", "now", oldB, newB, 3)
	}
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) > previewLines {
		lines = append(lines[:previewLines], fmt.Sprintf("... %d more lines", len(lines)-previewLines))
	}
	return lines
}

func prefixLines(s, p string) string {
	var b strings.Builder
	for _, l := range strings.Split(strings.TrimRight(s, "\n"), "\n") {
		b.WriteString(p + l + "\n")
	}
	return b.String()
}

func (r *Real) locked(f func(m *gen.Manager) (string, error)) (string, error) {
	m, err := r.manager()
	if err != nil {
		return "", err
	}
	unlock, err := m.Store.Lock()
	if err != nil {
		return "", err
	}
	defer unlock()
	return f(m)
}

// Snap saves a new generation.
func (r *Real) Snap(message string) (string, error) {
	return r.locked(func(m *gen.Manager) (string, error) {
		g, _, err := m.Snapshot(message, gen.OriginManual)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("Saved as save point #%d.", g.ID), nil
	})
}

// Init creates the store and the first generation.
func (r *Real) Init() (string, error) {
	if _, err := store.Create(r.StoreDir); err != nil {
		return "", err
	}
	cfg := config.Default()
	if err := cfg.Save(r.StoreDir); err != nil {
		return "", err
	}
	return r.locked(func(m *gen.Manager) (string, error) {
		g, _, err := m.Snapshot("initial state", gen.OriginInit)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("Dotka is ready: your current state is save point #%d.", g.ID), nil
	})
}

// Preview computes a rollback plan without writing anything.
func (r *Real) Preview(id int) (gen.Plan, gen.ExtraDiff, error) {
	m, err := r.manager()
	if err != nil {
		return gen.Plan{}, gen.ExtraDiff{}, err
	}
	plan, cur, err := m.Preview(id, false)
	if err != nil {
		return gen.Plan{}, gen.ExtraDiff{}, err
	}
	return plan, gen.DiffExtra(cur.Extra, plan.Target.Extra), nil
}

// Rollback restores a generation (saving the current state first).
func (r *Real) Rollback(id int) (string, error) {
	return r.locked(func(m *gen.Manager) (string, error) {
		res, err := m.Rollback(id, false)
		if err != nil {
			return "", err
		}
		msg := fmt.Sprintf("Went back to #%d. Your previous state is saved as #%d.", id, res.Pre.ID)
		if len(res.Failures) > 0 {
			msg += fmt.Sprintf(" %d action(s) failed; run `dotka status`.", len(res.Failures))
		}
		return msg, nil
	})
}

// Forget deletes one generation and frees its space.
func (r *Real) Forget(id int) (string, error) {
	return r.locked(func(m *gen.Manager) (string, error) {
		p, err := m.PlanForget([]int{id})
		if err != nil {
			return "", err
		}
		_, b, err := m.Forget(p)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("Forgot #%d and freed %s.", id, humanSize(b)), nil
	})
}

func humanSize(b int64) string {
	switch {
	case b < 1024:
		return fmt.Sprintf("%d B", b)
	case b < 1<<20:
		return fmt.Sprintf("%.1f KiB", float64(b)/1024)
	}
	return fmt.Sprintf("%.1f MiB", float64(b)/(1<<20))
}

// isFoldedChild reports whether c lives inside a directory that was added
// or removed as a whole (it is shown folded under that directory).
func isFoldedChild(all []gen.Change, c gen.Change) bool {
	for _, p := range all {
		if p.Path == c.Path || p.Kind != c.Kind || (p.Kind != gen.Added && p.Kind != gen.Removed) {
			continue
		}
		e := p.New
		if e == nil {
			e = p.Old
		}
		if e.Type == scan.TypeDir && strings.HasPrefix(c.Path, p.Path+"/") {
			return true
		}
	}
	return false
}

// foldedChildren lists the files inside a folded directory change.
func foldedChildren(all []gen.Change, dir gen.Change) []string {
	var out []string
	for _, c := range all {
		if c.Kind == dir.Kind && strings.HasPrefix(c.Path, dir.Path+"/") {
			e := c.New
			if e == nil {
				e = c.Old
			}
			if e.Type != scan.TypeDir {
				out = append(out, strings.TrimPrefix(c.Path, dir.Path+"/"))
			}
		}
	}
	sort.Strings(out)
	return out
}
