package store

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPutFileDeduplicates(t *testing.T) {
	s, err := Create(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	a, b := filepath.Join(dir, "a"), filepath.Join(dir, "b")
	os.WriteFile(a, []byte("same"), 0o644)
	os.WriteFile(b, []byte("same"), 0o644)
	ha, err := s.PutFile(a)
	if err != nil {
		t.Fatal(err)
	}
	hb, _ := s.PutFile(b)
	if ha != hb || ha != HashBytes([]byte("same")) {
		t.Fatalf("hash mismatch %s %s", ha, hb)
	}
	got, err := s.ReadObject(ha)
	if err != nil || string(got) != "same" {
		t.Fatalf("ReadObject = %q, %v", got, err)
	}
	n := 0
	filepath.WalkDir(filepath.Join(s.Dir, "objects"), func(p string, d os.DirEntry, _ error) error {
		if !d.IsDir() {
			n++
		}
		return nil
	})
	if n != 1 {
		t.Fatalf("expected 1 object, got %d", n)
	}
}

func TestStoreIsPrivate(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "store")
	if _, err := Create(dir); err != nil {
		t.Fatal(err)
	}
	fi, _ := os.Stat(dir)
	if fi.Mode().Perm() != 0o700 {
		t.Fatalf("store perms = %v", fi.Mode().Perm())
	}
	if _, err := Create(dir); err == nil {
		t.Fatal("creating an existing store must fail")
	}
}

func TestGenerationsAreImmutable(t *testing.T) {
	s, _ := Create(filepath.Join(t.TempDir(), "store"))
	if err := s.SaveGeneration(1, map[string]int{"x": 1}); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveGeneration(1, map[string]int{"x": 2}); err == nil {
		t.Fatal("overwriting a generation must fail")
	}
	var v map[string]int
	if err := s.LoadGeneration(1, &v); err != nil || v["x"] != 1 {
		t.Fatalf("got %v %v", v, err)
	}
	if id, _ := s.NextID(); id != 2 {
		t.Fatalf("NextID = %d", id)
	}
}

func TestLockIsExclusive(t *testing.T) {
	s, _ := Create(filepath.Join(t.TempDir(), "store"))
	unlock, err := s.Lock()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Lock(); err == nil {
		t.Fatal("second lock should fail")
	}
	unlock()
	u2, err := s.Lock()
	if err != nil {
		t.Fatal(err)
	}
	u2()
}

func TestGC(t *testing.T) {
	s, _ := Create(filepath.Join(t.TempDir(), "store"))
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a"), []byte("keep"), 0o644)
	os.WriteFile(filepath.Join(dir, "b"), []byte("drop"), 0o644)
	ha, _ := s.PutFile(filepath.Join(dir, "a"))
	hb, _ := s.PutFile(filepath.Join(dir, "b"))
	n, _, err := s.GC(map[string]bool{ha: true})
	if err != nil || n != 1 {
		t.Fatalf("GC removed %d, %v", n, err)
	}
	if !s.Has(ha) || s.Has(hb) {
		t.Fatal("GC removed the wrong object")
	}
}
