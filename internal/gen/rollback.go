package gen

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"sort"
	"strings"

	"github.com/iYeremy/dotka/internal/config"
	"github.com/iYeremy/dotka/internal/fsutil"
	"github.com/iYeremy/dotka/internal/scan"
	"github.com/iYeremy/dotka/internal/store"
)

// Action kinds, in the order they are applied.
const (
	ActRemove  = "remove"  // remove a file or symlink
	ActRmdir   = "rmdir"   // remove a directory if it is empty
	ActMkdir   = "mkdir"   // create a directory
	ActWrite   = "write"   // write a file from the store
	ActSymlink = "symlink" // create a symlink
	ActChmod   = "chmod"   // change permissions
)

var actOrder = map[string]int{ActRemove: 0, ActRmdir: 1, ActMkdir: 2, ActWrite: 3, ActSymlink: 3, ActChmod: 4}

// Action is one filesystem operation of a rollback.
type Action struct {
	Kind string
	Path string
	// Want is the target entry (write/symlink/mkdir/chmod).
	Want *scan.Entry
	// Have is the entry currently on disk (remove/rmdir), used to verify
	// nothing changed between planning and applying.
	Have *scan.Entry
	// Reason explains why (added after target, modified, type changed...).
	Reason string
}

// Skipped is something the rollback will not touch, and why.
type Skipped struct {
	Path   string
	Reason string
}

// Plan is the full list of operations to go from one state to a target
// generation.
type Plan struct {
	Target  Generation
	Actions []Action
	Skipped []Skipped
}

// Empty reports whether the plan changes nothing.
func (p Plan) Empty() bool { return len(p.Actions) == 0 }

// PlanOptions tunes BuildPlan.
type PlanOptions struct {
	// KeepNew keeps files that did not exist in the target generation.
	KeepNew bool
	// AssumeStored treats every non-skipped current file as restorable.
	// Used for previews, before the pre-rollback snapshot is taken.
	AssumeStored bool
}

// BuildPlan computes the actions needed to turn `current` into `target`.
// Only paths tracked both in the target generation and in the current
// configuration are considered.
func BuildPlan(target Generation, curRoots []string, current []scan.Entry, opt PlanOptions) Plan {
	plan := Plan{Target: target}
	inScope := func(p string) bool {
		return config.UnderRoots(target.Roots, p) && config.UnderRoots(curRoots, p)
	}
	ti, ci := map[string]scan.Entry{}, map[string]scan.Entry{}
	for _, e := range target.Entries {
		if inScope(e.Path) {
			ti[e.Path] = e
		}
	}
	for _, e := range current {
		if inScope(e.Path) {
			ci[e.Path] = e
		}
	}
	restorable := func(e scan.Entry) bool {
		if opt.AssumeStored && e.Type == scan.TypeFile && e.Skip == "" {
			return true
		}
		return e.Restorable()
	}
	// removed tracks current paths that will disappear, so a directory is
	// only planned for removal when we expect it to become empty.
	blocked := map[string]bool{} // current paths that must stay

	// 1. Entries present now but not in the target: remove them.
	for p, c := range ci {
		if _, ok := ti[p]; ok {
			continue
		}
		c := c
		switch {
		case opt.KeepNew:
			blocked[p] = true
		case c.NixManaged():
			blocked[p] = true
			plan.Skipped = append(plan.Skipped, Skipped{p, nixReason})
		case !restorable(c):
			blocked[p] = true
			plan.Skipped = append(plan.Skipped, Skipped{p, "added after target but not stored (" + skipWhy(c) + "); kept"})
		case c.Type == scan.TypeDir:
			plan.Actions = append(plan.Actions, Action{Kind: ActRmdir, Path: p, Have: &c, Reason: "added after target"})
		default:
			plan.Actions = append(plan.Actions, Action{Kind: ActRemove, Path: p, Have: &c, Reason: "added after target"})
		}
	}

	// 2. Entries of the target: create or fix them.
	for p, t := range ti {
		t := t
		c, exists := ci[p]
		if exists && c.Type == t.Type && c.SameContent(t) && (c.Mode == t.Mode || t.Type == scan.TypeSymlink) {
			continue
		}
		if exists && c.Type == t.Type && c.SameContent(t) {
			plan.Actions = append(plan.Actions, Action{Kind: ActChmod, Path: p, Want: &t, Reason: "mode changed"})
			continue
		}
		if t.NixManaged() || (exists && c.NixManaged()) {
			plan.Skipped = append(plan.Skipped, Skipped{p, nixReason})
			blocked[p] = true
			continue
		}
		if !t.Restorable() {
			if exists && c.Skip == scan.SkipSecret && t.Skip == scan.SkipSecret {
				plan.Skipped = append(plan.Skipped, Skipped{p, "secret changed; Dotka never stores secrets"})
			} else {
				plan.Skipped = append(plan.Skipped, Skipped{p, "cannot restore (" + skipWhy(t) + ")"})
			}
			blocked[p] = true
			continue
		}
		reason := "missing"
		if exists {
			reason = "modified"
			if c.Type != t.Type {
				// The current object must go first.
				if !restorable(c) {
					plan.Skipped = append(plan.Skipped, Skipped{p, "type changed but current " + c.Type + " is not stored; kept"})
					blocked[p] = true
					continue
				}
				c := c
				kind := ActRemove
				if c.Type == scan.TypeDir {
					kind = ActRmdir
				}
				plan.Actions = append(plan.Actions, Action{Kind: kind, Path: p, Have: &c, Reason: "type changed"})
				reason = "type changed"
			}
		}
		switch t.Type {
		case scan.TypeDir:
			if exists && c.Type == scan.TypeDir {
				plan.Actions = append(plan.Actions, Action{Kind: ActChmod, Path: p, Want: &t, Reason: "mode changed"})
			} else {
				plan.Actions = append(plan.Actions, Action{Kind: ActMkdir, Path: p, Want: &t, Reason: reason})
			}
		case scan.TypeFile:
			plan.Actions = append(plan.Actions, Action{Kind: ActWrite, Path: p, Want: &t, Reason: reason})
		case scan.TypeSymlink:
			plan.Actions = append(plan.Actions, Action{Kind: ActSymlink, Path: p, Want: &t, Reason: reason})
		}
	}

	// A directory can only be removed if nothing inside it stays.
	var kept []Action
	for _, a := range plan.Actions {
		if a.Kind == ActRmdir && hasBlockedChild(blocked, a.Path) {
			plan.Skipped = append(plan.Skipped, Skipped{a.Path, "directory contains files that are kept"})
			continue
		}
		kept = append(kept, a)
	}
	plan.Actions = kept
	sortActions(plan.Actions)
	sort.Slice(plan.Skipped, func(i, j int) bool { return plan.Skipped[i].Path < plan.Skipped[j].Path })
	return plan
}

// nixReason explains why links into the Nix store are left alone.
const nixReason = "managed by Nix/home-manager (link into /nix/store); use home-manager to change it"

func hasBlockedChild(blocked map[string]bool, dir string) bool {
	for p := range blocked {
		if strings.HasPrefix(p, dir+"/") {
			return true
		}
	}
	return false
}

func skipWhy(e scan.Entry) string {
	switch e.Skip {
	case scan.SkipSecret:
		return "secret"
	case scan.SkipLarge:
		return "too large"
	}
	return "not stored"
}

func depth(p string) int { return strings.Count(p, "/") }

func sortActions(a []Action) {
	sort.SliceStable(a, func(i, j int) bool {
		oi, oj := actOrder[a[i].Kind], actOrder[a[j].Kind]
		if oi != oj {
			return oi < oj
		}
		switch a[i].Kind {
		case ActRmdir, ActChmod:
			// Deepest first: children before parents.
			if depth(a[i].Path) != depth(a[j].Path) {
				return depth(a[i].Path) > depth(a[j].Path)
			}
		case ActMkdir:
			if depth(a[i].Path) != depth(a[j].Path) {
				return depth(a[i].Path) < depth(a[j].Path)
			}
		}
		return a[i].Path < a[j].Path
	})
}

// ---- Applying -------------------------------------------------------------

// Failure is an action that could not be applied.
type Failure struct {
	Action Action
	Err    error
}

// ErrChangedSincePlan means the file on disk is not what the plan expected.
var ErrChangedSincePlan = errors.New("changed since the plan was made; left untouched")

// Apply executes a plan against home, reading contents from the store.
// It never stops at the first error: every action is attempted and every
// failure is reported, so the user knows exactly what state they are in.
func Apply(home string, s *store.Store, plan Plan) []Failure {
	var fails []Failure
	for _, a := range plan.Actions {
		if err := applyOne(home, s, a); err != nil {
			fails = append(fails, Failure{a, err})
		}
	}
	return fails
}

func applyOne(home string, s *store.Store, a Action) error {
	full, err := fsutil.SafeJoin(home, a.Path)
	if err != nil {
		return err
	}
	switch a.Kind {
	case ActRemove:
		if err := verifyUnchanged(full, a.Have); err != nil {
			return err
		}
		err := os.Remove(full)
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	case ActRmdir:
		err := os.Remove(full) // only succeeds if empty
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("directory not removed (not empty?): %w", err)
		}
		return nil
	case ActMkdir:
		if err := os.MkdirAll(full, fs.FileMode(a.Want.Mode)|0o700); err != nil {
			return err
		}
		return os.Chmod(full, fs.FileMode(a.Want.Mode))
	case ActWrite:
		if err := ensureParent(home, a.Path); err != nil {
			return err
		}
		f, err := s.OpenObject(a.Want.Hash)
		if err != nil {
			return fmt.Errorf("object missing from store: %w", err)
		}
		defer f.Close()
		if fi, err := os.Lstat(full); err == nil && fi.IsDir() {
			return fmt.Errorf("a directory is in the way")
		}
		return fsutil.AtomicWrite(full, f, fs.FileMode(a.Want.Mode))
	case ActSymlink:
		if err := ensureParent(home, a.Path); err != nil {
			return err
		}
		if fi, err := os.Lstat(full); err == nil && fi.IsDir() {
			return fmt.Errorf("a directory is in the way")
		}
		return fsutil.AtomicSymlink(a.Want.Target, full)
	case ActChmod:
		fi, err := os.Lstat(full)
		if err != nil {
			return err
		}
		if fi.Mode()&fs.ModeSymlink != 0 {
			return nil
		}
		return os.Chmod(full, fs.FileMode(a.Want.Mode))
	}
	return fmt.Errorf("unknown action %q", a.Kind)
}

// ensureParent creates missing parent directories (inside home, never
// through symlinks).
func ensureParent(home, rel string) error {
	i := strings.LastIndex(rel, "/")
	if i < 0 {
		return nil
	}
	parent, err := fsutil.SafeJoin(home, rel[:i])
	if err != nil {
		return err
	}
	return os.MkdirAll(parent, 0o755)
}

// verifyUnchanged checks the object on disk still matches what was planned.
func verifyUnchanged(full string, have *scan.Entry) error {
	if have == nil {
		return nil
	}
	fi, err := os.Lstat(full)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	switch have.Type {
	case scan.TypeSymlink:
		if fi.Mode()&fs.ModeSymlink == 0 {
			return ErrChangedSincePlan
		}
		t, _ := os.Readlink(full)
		if t != have.Target {
			return ErrChangedSincePlan
		}
	case scan.TypeFile:
		if !fi.Mode().IsRegular() || fi.Size() != have.Size {
			return ErrChangedSincePlan
		}
		if have.Hash != "" {
			h, err := store.HashFile(full)
			if err != nil {
				return err
			}
			if h != have.Hash {
				return ErrChangedSincePlan
			}
		}
	}
	return nil
}

// ---- High-level rollback --------------------------------------------------

// RollbackResult describes what happened.
type RollbackResult struct {
	Pre      Generation // automatic snapshot taken before applying
	Plan     Plan
	Failures []Failure
}

// Preview computes the plan against the current state without writing
// anything (not even to the store).
func (m *Manager) Preview(targetID int, keepNew bool) (Plan, State, error) {
	target, err := m.Load(targetID)
	if err != nil {
		return Plan{}, State{}, err
	}
	cur, err := m.Current()
	if err != nil {
		return Plan{}, State{}, err
	}
	return BuildPlan(target, cur.Roots, cur.Entries, PlanOptions{KeepNew: keepNew, AssumeStored: true}), cur, nil
}

// Rollback snapshots the current state, then restores the target
// generation. The caller must hold the store lock.
func (m *Manager) Rollback(targetID int, keepNew bool) (RollbackResult, error) {
	target, err := m.Load(targetID)
	if err != nil {
		return RollbackResult{}, err
	}
	pre, _, err := m.Snapshot(fmt.Sprintf("before rollback to #%d", targetID), OriginPreRollback)
	if err != nil {
		return RollbackResult{}, fmt.Errorf("pre-rollback snapshot failed, nothing was changed: %w", err)
	}
	plan := BuildPlan(target, m.Config.Roots, pre.Entries, PlanOptions{KeepNew: keepNew})
	fails := Apply(m.Home, m.Store, plan)
	result := "ok"
	if len(fails) > 0 {
		result = fmt.Sprintf("partial: %d failures", len(fails))
	}
	if err := m.Store.SetHead(targetID); err != nil {
		return RollbackResult{Pre: pre, Plan: plan, Failures: fails}, err
	}
	m.Store.AppendLog(store.LogEntry{Op: "rollback", Gen: pre.ID, Target: targetID, Result: result,
		Details: fmt.Sprintf("%d actions, %d skipped", len(plan.Actions), len(plan.Skipped))})
	return RollbackResult{Pre: pre, Plan: plan, Failures: fails}, nil
}
