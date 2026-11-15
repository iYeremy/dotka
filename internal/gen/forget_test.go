package gen

import (
	"testing"

	"github.com/iYeremy/dotka/internal/store"
)

// threeGens creates generations with .zshrc = v1, v2, v3 (HEAD = 3).
func threeGens(t *testing.T) *Manager {
	t.Helper()
	m := newManager(t)
	for _, v := range []string{"v1\n", "v2\n", "v3\n"} {
		write(t, m.Home, ".zshrc", v)
		write(t, m.Home, ".config/shared.conf", "same in every generation\n")
		if _, _, err := m.Snapshot(v, OriginManual); err != nil {
			t.Fatal(err)
		}
	}
	return m
}

func TestForgetFreesOnlyUnsharedObjects(t *testing.T) {
	m := threeGens(t)
	p, err := m.PlanForget([]int{1})
	if err != nil {
		t.Fatal(err)
	}
	if p.FreedObjects != 1 || p.FreedBytes != 3 {
		t.Fatalf("plan = %+v, want 1 object / 3 bytes (only v1)", p)
	}
	n, _, err := m.Forget(p)
	if err != nil || n != 1 {
		t.Fatalf("Forget removed %d objects, err %v", n, err)
	}
	if m.Store.Has(store.HashBytes([]byte("v1\n"))) {
		t.Error("v1 object should be gone")
	}
	if !m.Store.Has(store.HashBytes([]byte("same in every generation\n"))) {
		t.Error("shared object must be kept")
	}
	if _, err := m.Load(1); err == nil {
		t.Error("generation 1 should be deleted")
	}
}

func TestCannotForgetCurrentOrMissing(t *testing.T) {
	m := threeGens(t)
	if _, err := m.PlanForget([]int{3}); err == nil {
		t.Error("forgetting HEAD must fail")
	}
	if _, err := m.PlanForget([]int{42}); err == nil {
		t.Error("forgetting a missing generation must fail")
	}
}

func TestPruneKeepsNewestAndHead(t *testing.T) {
	m := threeGens(t)
	m.Store.SetHead(1) // e.g. after a rollback to #1
	p, err := m.PlanPrune(1)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.IDs) != 1 || p.IDs[0] != 2 {
		t.Fatalf("prune --keep 1 should drop only #2 (keep newest #3 and HEAD #1), got %v", p.IDs)
	}
}

func TestIDsAreNeverReused(t *testing.T) {
	m := threeGens(t)
	m.Store.SetHead(2)
	p, _ := m.PlanForget([]int{3})
	if _, _, err := m.Forget(p); err != nil {
		t.Fatal(err)
	}
	g, _, err := m.Snapshot("new", OriginManual)
	if err != nil {
		t.Fatal(err)
	}
	if g.ID != 4 {
		t.Fatalf("new generation got id %d, want 4 (3 must not be reused)", g.ID)
	}
}
