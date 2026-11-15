package main

import (
	"fmt"
	"os"

	"github.com/charmbracelet/x/term"
	"github.com/iYeremy/dotka/internal/tui"
)

// cmdTUI opens the interactive interface. Setting dotka up from the UI shows
// the same beta notice and confirmation as `dotka init`.
func cmdTUI(e env) error {
	if !isTerminal(os.Stdout) || !isTerminal(os.Stdin) {
		return fmt.Errorf("the interface needs an interactive terminal; use the commands instead (dotka help)")
	}
	be := &tui.Real{
		Home:      e.home,
		StoreDir:  e.storeDir,
		RealHome:  e.realHome,
		Collector: collector,
	}
	return tui.Run(be)
}

// stdinIsTerminal is a variable so tests can simulate scripts and pipes.
var stdinIsTerminal = func() bool { return isTerminal(os.Stdin) }

// isTerminal reports whether f is a real TTY (not just a character device:
// /dev/null is one too, and must not count as interactive).
func isTerminal(f *os.File) bool {
	return term.IsTerminal(f.Fd())
}
