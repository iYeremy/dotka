package gen

import (
	"slices"
	"strconv"
	"strings"

	"github.com/iYeremy/dotka/internal/config"
	"github.com/iYeremy/dotka/internal/scan"
)

// Event is one change to something (a file, package or startup item) in the
// history, attributed to the generation that recorded it.
type Event struct {
	Gen    Generation // manifest of the generation where the change was seen
	Parent int        // generation it was compared against (0 = none)
	Kind   string     // Added, Removed, Modified, TypeChg, ModeChg
	Detail string
	// Kept is set when an "added" change is really something that was
	// added earlier (in generation Kept) and survived a rollback to an
	// older generation, e.g. packages, which dotka never removes.
	Kept int
}

// Cause explains what produced the change, in plain words.
func (e Event) Cause() string {
	g := e.Gen
	switch {
	case e.Kept != 0:
		return "kept after going back to #" + strconv.Itoa(e.Parent) + " (first added in #" + strconv.Itoa(e.Kept) + ")"
	case g.Run != nil:
		return "by `" + g.Run.Display() + "`"
	case g.Origin == OriginInit:
		return "already there when dotka was initialized"
	case e.Parent == 0:
		return "first seen here"
	default:
		return "changed outside dotka (between #" + strconv.Itoa(e.Parent) + " and this snapshot)"
	}
}

// markKept flags "added" events that re-add something already added in a
// generation newer than the event's parent (it survived a rollback).
func markKept(evs []Event) []Event {
	lastAdded := 0
	for i := range evs {
		ev := &evs[i]
		switch ev.Kind {
		case Added:
			if lastAdded != 0 && ev.Parent != 0 && ev.Parent < lastAdded {
				ev.Kept = lastAdded
			} else {
				lastAdded = ev.Gen.ID
			}
		case Removed:
			lastAdded = 0
		}
	}
	return evs
}

// parentOf returns the generation a given one should be compared with:
// its recorded parent, or the previous generation for old manifests.
func parentOf(gens []Generation, i int) (Generation, bool) {
	g := gens[i]
	if g.Parent != 0 {
		for _, p := range gens {
			if p.ID == g.Parent {
				return p, true
			}
		}
		// The parent was forgotten: fall back to the previous generation.
	}
	if i > 0 {
		return gens[i-1], true
	}
	return Generation{}, false
}

// FileHistory returns every change to a home-relative path across all
// generations, oldest first.
func (m *Manager) FileHistory(path string) ([]Event, error) {
	gens, err := m.List()
	if err != nil {
		return nil, err
	}
	var out []Event
	for i, g := range gens {
		cur, inCur := scan.Index(g.Entries)[path]
		p, hasParent := parentOf(gens, i)
		if !hasParent {
			if inCur {
				out = append(out, Event{Gen: g, Kind: Added, Detail: cur.Type})
			}
			continue
		}
		if !config.UnderRoots(p.Roots, path) || !config.UnderRoots(g.Roots, path) {
			continue
		}
		prev, inPrev := scan.Index(p.Entries)[path]
		ev := Event{Gen: g, Parent: p.ID}
		switch {
		case !inPrev && inCur:
			ev.Kind, ev.Detail = Added, cur.Type
		case inPrev && !inCur:
			ev.Kind = Removed
		case inPrev && inCur && prev.Type != cur.Type:
			ev.Kind, ev.Detail = TypeChg, prev.Type+" -> "+cur.Type
		case inPrev && inCur && !prev.SameContent(cur):
			ev.Kind = Modified
		case inPrev && inCur && prev.Mode != cur.Mode && cur.Type != scan.TypeSymlink:
			ev.Kind = ModeChg
		default:
			continue
		}
		out = append(out, ev)
	}
	return markKept(out), nil
}

// PackageHistory returns when a package appeared in or disappeared from the
// explicitly installed packages.
func (m *Manager) PackageHistory(name string) ([]Event, error) {
	return m.setHistory(func(g Generation) (bool, bool) {
		if g.Packages == nil {
			return false, false
		}
		return slices.Contains(g.Packages.Explicit, name), true
	})
}

// StartupHistory returns changes to startup items whose command contains
// substr.
func (m *Manager) StartupHistory(substr string) ([]Event, error) {
	gens, err := m.List()
	if err != nil {
		return nil, err
	}
	var out []Event
	for i, g := range gens {
		cur := matching(g.Startup, substr)
		p, ok := parentOf(gens, i)
		var prev map[string]StartupItem
		if ok {
			prev = matching(p.Startup, substr)
		}
		for k, it := range cur {
			if _, was := prev[k]; !was {
				out = append(out, Event{Gen: g, Parent: p.ID, Kind: Added, Detail: describe(it)})
			}
		}
		for k, it := range prev {
			if _, is := cur[k]; !is {
				out = append(out, Event{Gen: g, Parent: p.ID, Kind: Removed, Detail: describe(it)})
			}
		}
	}
	return out, nil
}

func (m *Manager) setHistory(has func(Generation) (present, known bool)) ([]Event, error) {
	gens, err := m.List()
	if err != nil {
		return nil, err
	}
	var out []Event
	for i, g := range gens {
		now, known := has(g)
		if !known {
			continue
		}
		p, ok := parentOf(gens, i)
		if !ok {
			if now {
				out = append(out, Event{Gen: g, Kind: Added})
			}
			continue
		}
		before, pknown := has(p)
		if !pknown {
			continue
		}
		switch {
		case !before && now:
			out = append(out, Event{Gen: g, Parent: p.ID, Kind: Added})
		case before && !now:
			out = append(out, Event{Gen: g, Parent: p.ID, Kind: Removed})
		}
	}
	return markKept(out), nil
}

func matching(items []StartupItem, substr string) map[string]StartupItem {
	m := map[string]StartupItem{}
	for _, it := range items {
		if strings.Contains(it.Command, substr) {
			m[it.Key()] = it
		}
	}
	return m
}

func describe(it StartupItem) string {
	loc := it.Source
	if it.Line > 0 {
		loc += ":" + strconv.Itoa(it.Line)
	}
	return it.Kind + " " + it.Command + "  (" + loc + ")"
}
