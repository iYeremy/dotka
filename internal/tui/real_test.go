package tui

import (
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// TestRealBackendRender loads a real home with the production backend and
// renders every tab. It only runs when DOTKA_TUI_TEST_HOME points at a
// sandbox home (never your real one), e.g.:
//
//	DOTKA_TUI_TEST_HOME=$PWD/.sandbox/home go test ./internal/tui -run Real -v
func TestRealBackendRender(t *testing.T) {
	home := os.Getenv("DOTKA_TUI_TEST_HOME")
	if home == "" {
		t.Skip("set DOTKA_TUI_TEST_HOME to a sandbox home")
	}
	be := &Real{Home: home, StoreDir: filepath.Join(home, ".local/share/dotka")}
	d, err := be.Load()
	if err != nil {
		t.Fatal(err)
	}
	m := send(New(be), tea.WindowSizeMsg{Width: 100, Height: 30}, loadedMsg{data: d})
	for _, k := range []string{"1", "2", "3", "4", "5"} {
		m = send(m, keys(k)...)
		v := m.View()
		checkFrame(t, "real tab "+k, v, 100, 30)
		if testing.Verbose() {
			t.Log("\n" + v)
		}
	}
	// The file diff comes from the real store + disk.
	m = send(m, keys("2")...)
	if r := m.selected(); r != nil && r.change != nil {
		if lines := be.Patch(*r.change); len(lines) == 0 {
			t.Error("empty patch for a file change")
		}
	}
}
