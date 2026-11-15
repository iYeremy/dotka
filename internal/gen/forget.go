package gen

import (
	"fmt"
	"slices"
	"sort"

	"github.com/iYeremy/dotka/internal/store"
)

// ForgetPlan lists generations to delete and the space that will be freed.
type ForgetPlan struct {
	IDs []int
	// Objects and bytes referenced only by the forgotten generations.
	FreedObjects int
	FreedBytes   int64
}

// PlanForget validates ids and computes what forgetting them frees.
// The current generation (HEAD) cannot be forgotten.
func (m *Manager) PlanForget(ids []int) (ForgetPlan, error) {
	gens, err := m.List()
	if err != nil {
		return ForgetPlan{}, err
	}
	head, _ := m.Store.Head()
	exists := map[int]bool{}
	for _, g := range gens {
		exists[g.ID] = true
	}
	uniq := map[int]bool{}
	for _, id := range ids {
		if !exists[id] {
			return ForgetPlan{}, fmt.Errorf("generation %d does not exist", id)
		}
		if id == head {
			return ForgetPlan{}, fmt.Errorf("generation %d is the current one and cannot be forgotten", id)
		}
		uniq[id] = true
	}
	return m.freed(gens, uniq), nil
}

// PlanPrune keeps the `keep` most recent generations (plus HEAD, always) and
// plans to forget the rest.
func (m *Manager) PlanPrune(keep int) (ForgetPlan, error) {
	if keep < 1 {
		return ForgetPlan{}, fmt.Errorf("--keep must be at least 1")
	}
	gens, err := m.List()
	if err != nil {
		return ForgetPlan{}, err
	}
	head, _ := m.Store.Head()
	drop := map[int]bool{}
	for i, g := range gens {
		newest := i >= len(gens)-keep
		if !newest && g.ID != head {
			drop[g.ID] = true
		}
	}
	return m.freed(gens, drop), nil
}

func (m *Manager) freed(gens []Generation, drop map[int]bool) ForgetPlan {
	p := ForgetPlan{}
	kept := map[string]bool{}
	for _, g := range gens {
		if drop[g.ID] {
			p.IDs = append(p.IDs, g.ID)
			continue
		}
		for _, e := range g.Entries {
			if e.Stored {
				kept[e.Hash] = true
			}
		}
	}
	counted := map[string]bool{}
	for _, g := range gens {
		if !drop[g.ID] {
			continue
		}
		for _, e := range g.Entries {
			if e.Stored && !kept[e.Hash] && !counted[e.Hash] {
				counted[e.Hash] = true
				p.FreedObjects++
				p.FreedBytes += e.Size
			}
		}
	}
	sort.Ints(p.IDs)
	return p
}

// Forget deletes the planned generations and then removes the objects no
// remaining generation uses. The caller must hold the store lock.
func (m *Manager) Forget(p ForgetPlan) (objects int, bytes int64, err error) {
	head, _ := m.Store.Head()
	if slices.Contains(p.IDs, head) {
		return 0, 0, fmt.Errorf("refusing to forget the current generation %d", head)
	}
	for _, id := range p.IDs {
		if err := m.Store.DeleteGeneration(id); err != nil {
			return 0, 0, err
		}
	}
	keep, err := m.LiveObjects()
	if err != nil {
		return 0, 0, err
	}
	objects, bytes, err = m.Store.GC(keep)
	m.Store.AppendLog(store.LogEntry{Op: "forget", Result: resultOf(err),
		Details: fmt.Sprintf("generations %v, %d objects freed", p.IDs, objects)})
	return objects, bytes, err
}

func resultOf(err error) string {
	if err != nil {
		return "error: " + err.Error()
	}
	return "ok"
}
