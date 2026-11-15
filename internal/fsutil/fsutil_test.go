package fsutil

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCleanRelRejectsEscapes(t *testing.T) {
	bad := []string{"", "/etc/passwd", "..", "../x", "a/../../x", ".", "a/.."}
	for _, p := range bad {
		if _, err := CleanRel(p); !errors.Is(err, ErrUnsafePath) {
			t.Errorf("CleanRel(%q) should fail, got %v", p, err)
		}
	}
	good := map[string]string{".config/hypr/x.lua": ".config/hypr/x.lua", "a//b/": "a/b", "./a": "a"}
	for in, want := range good {
		got, err := CleanRel(in)
		if err != nil || got != want {
			t.Errorf("CleanRel(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
}

func TestSafeJoinRefusesSymlinkParent(t *testing.T) {
	base := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(base, "evil")); err != nil {
		t.Fatal(err)
	}
	if _, err := SafeJoin(base, "evil/file"); !errors.Is(err, ErrUnsafePath) {
		t.Fatalf("expected ErrUnsafePath through symlinked parent, got %v", err)
	}
	// The final element being a symlink is allowed (caller uses Lstat).
	if _, err := SafeJoin(base, "evil"); err != nil {
		t.Fatalf("final symlink element should be allowed: %v", err)
	}
	// Non-existing parents are fine.
	if _, err := SafeJoin(base, "new/dir/file"); err != nil {
		t.Fatal(err)
	}
}

func TestAtomicWriteAndSymlink(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "f")
	if err := AtomicWrite(p, strings.NewReader("hello"), 0o600); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(p)
	fi, _ := os.Stat(p)
	if string(b) != "hello" || fi.Mode().Perm() != 0o600 {
		t.Fatalf("got %q %v", b, fi.Mode())
	}
	// Overwrite a regular file with a symlink atomically.
	if err := AtomicSymlink("target", p); err != nil {
		t.Fatal(err)
	}
	if tgt, _ := os.Readlink(p); tgt != "target" {
		t.Fatalf("readlink = %q", tgt)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatalf("temp files left behind: %v", entries)
	}
}
