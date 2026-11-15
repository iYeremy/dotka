package tui

import (
	tea "github.com/charmbracelet/bubbletea"
)

// Run starts the full-screen interface and blocks until the user quits.
// Bubble Tea restores the terminal (alt screen, cursor, raw mode) on every
// exit path, including panics inside the program loop.
func Run(be Backend) error {
	_, err := tea.NewProgram(New(be), tea.WithAltScreen()).Run()
	return err
}
