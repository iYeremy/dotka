package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// Palette: ANSI 16 colors only, so it follows the user's terminal theme
// (dark or light) and works on a Linux TTY. Color is never the only
// carrier of meaning: every state also has a symbol or a word.
var (
	sBold     = lipgloss.NewStyle().Bold(true)
	sMuted    = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	sAccent   = lipgloss.NewStyle().Foreground(lipgloss.Color("6")).Bold(true)
	sAdded    = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	sModified = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	sRemoved  = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	sDanger   = lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Bold(true)
	sSelected = lipgloss.NewStyle().Reverse(true)
	sTabOn    = lipgloss.NewStyle().Reverse(true).Bold(true)
	sKey      = lipgloss.NewStyle().Bold(true)
)

// fit truncates (with "...") and pads s to exactly w display cells.
func fit(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if lipgloss.Width(s) > w {
		if w <= 3 {
			s = ansi.Truncate(s, w, "")
		} else {
			s = ansi.Truncate(s, w, "...")
		}
	}
	return s + strings.Repeat(" ", w-lipgloss.Width(s))
}

// rule is a horizontal separator line.
func rule(w int) string { return sMuted.Render(strings.Repeat("-", w)) }

// wrap breaks plain text into lines of at most w cells, on spaces.
func wrap(s string, w int) []string {
	if w <= 0 {
		return nil
	}
	var out []string
	for _, para := range strings.Split(s, "\n") {
		words := strings.Fields(para)
		if len(words) == 0 {
			out = append(out, "")
			continue
		}
		line := ""
		for _, word := range words {
			switch {
			case line == "":
				line = word
			case lipgloss.Width(line)+1+lipgloss.Width(word) <= w:
				line += " " + word
			default:
				out = append(out, line)
				line = word
			}
		}
		out = append(out, line)
	}
	return out
}

// window returns the visible slice [off, off+h) of n items, adjusting the
// offset so the cursor stays visible.
func window(cursor, off, n, h int) int {
	if h <= 0 || n <= h {
		return 0
	}
	if cursor < off {
		off = cursor
	}
	if cursor >= off+h {
		off = cursor - h + 1
	}
	if off > n-h {
		off = n - h
	}
	if off < 0 {
		off = 0
	}
	return off
}

// keyHint renders "key label" pairs for the footer.
func keyHint(pairs ...string) string {
	var parts []string
	for i := 0; i+1 < len(pairs); i += 2 {
		parts = append(parts, sKey.Render(pairs[i])+" "+sMuted.Render(pairs[i+1]))
	}
	return strings.Join(parts, "   ")
}
