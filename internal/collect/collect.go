// Package collect gathers the non-file parts of a state: installed packages
// (read-only) and the startup map. It is the default gen.Collector.
package collect

import (
	"errors"

	"github.com/iYeremy/dotka/internal/gen"
	"github.com/iYeremy/dotka/internal/pkgs"
	"github.com/iYeremy/dotka/internal/scan"
	"github.com/iYeremy/dotka/internal/startup"
)

// Extra implements gen.Collector.
func Extra(home string, _ []scan.Entry) (gen.Extra, []scan.Warning) {
	var extra gen.Extra
	var warns []scan.Warning
	if src := pkgs.FromEnv(); src != nil {
		p, err := src.Snapshot()
		switch {
		case errors.Is(err, pkgs.ErrUnavailable):
		case err != nil:
			warns = append(warns, scan.Warning{Path: "packages", Err: err})
		default:
			extra.Packages = p
		}
	}
	items, sw := startup.Extract(home)
	extra.Startup = items
	for _, w := range sw {
		warns = append(warns, scan.Warning{Path: w.Path, Err: w.Err})
	}
	return extra, warns
}
