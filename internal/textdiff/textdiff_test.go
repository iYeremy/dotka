package textdiff

import (
	"strings"
	"testing"
)

func TestUnified(t *testing.T) {
	a := "a\nb\nc\nd\ne\nf\ng\nh\n"
	b := "a\nb\nc\nD\ne\nf\ng\nh\ni\n"
	out := Unified("old", "new", []byte(a), []byte(b), 1)
	for _, want := range []string{"--- old", "+++ new", "-d", "+D", "+i", "@@ -3,3 +3,3 @@"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	if Unified("a", "b", []byte(a), []byte(a), 3) != "" {
		t.Error("equal inputs should produce no diff")
	}
}

func TestBinary(t *testing.T) {
	out := Unified("a", "b", []byte{0, 1, 2}, []byte{0, 1, 3}, 3)
	if !strings.HasPrefix(out, "Binary files") {
		t.Errorf("got %q", out)
	}
}
