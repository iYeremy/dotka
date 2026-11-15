package pkgs

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestParseList(t *testing.T) {
	got := ParseList("waybar\nhyprland 0.56\n\n  kitty\n")
	if !slices.Equal(got, []string{"hyprland", "kitty", "waybar"}) {
		t.Fatalf("got %v", got)
	}
}

func TestFileSource(t *testing.T) {
	p := filepath.Join(t.TempDir(), "pkgs")
	os.WriteFile(p, []byte("# comment\nhyprland\nswww aur\nkitty\n"), 0o644)
	s, err := FileSource{Path: p}.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(s.Explicit, []string{"hyprland", "kitty", "swww"}) || !slices.Equal(s.Foreign, []string{"swww"}) {
		t.Fatalf("got %+v", s)
	}
}

func TestFromEnv(t *testing.T) {
	t.Setenv("DOTKA_PACKAGES", "off")
	if FromEnv() != nil {
		t.Fatal("off should disable packages")
	}
	t.Setenv("DOTKA_PACKAGES", "file:/x")
	if _, ok := FromEnv().(FileSource); !ok {
		t.Fatal("file: should select FileSource")
	}
}
