package main

import (
	"fmt"
	"os"

	"github.com/iYeremy/dotka/internal/tui"
)

// cmdTUI opens the interactive interface. On the real home it is read-only
// while dotka is a prototype (same rule as the write commands).
func cmdTUI(e env) error {
	if !isTerminal(os.Stdout) || !isTerminal(os.Stdin) {
		return fmt.Errorf("the interface needs an interactive terminal; use the commands instead (dotka help)")
	}
	be := &tui.Real{
		Home:      e.home,
		StoreDir:  e.storeDir,
		RealHome:  e.realHome,
		ReadOnly:  e.realHome && os.Getenv("DOTKA_ALLOW_REAL_HOME") != "1",
		Collector: collector,
	}
	return tui.Run(be)
}

func isTerminal(f *os.File) bool {
	fi, err := f.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}
