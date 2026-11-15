package gen

// Extra holds the non-file parts of a state. Fields are filled by the
// Collector; they are optional so old manifests stay readable.
type Extra struct {
	Packages *Packages     `json:"packages,omitempty"`
	Startup  []StartupItem `json:"startup,omitempty"`
}

// Packages installed in the system, as seen by the user (read-only).
type Packages struct {
	Manager  string   `json:"manager"`
	Explicit []string `json:"explicit"`
	Foreign  []string `json:"foreign"` // AUR / not in sync repos
}

// StartupItem is something that runs automatically in the user session.
type StartupItem struct {
	Kind       string `json:"kind"`
	Command    string `json:"command"`
	Source     string `json:"source"`
	Line       int    `json:"line,omitempty"`
	Confidence string `json:"confidence"`     // "confirmed" | "heuristic"
	Risk       string `json:"risk,omitempty"` // e.g. "remote-exec", "local-script"
}

// Key identifies a startup item across generations (line numbers move).
func (s StartupItem) Key() string { return s.Kind + "\x00" + s.Source + "\x00" + s.Command }

// ExtraDiff is the difference in packages and startup items.
type ExtraDiff struct {
	PkgAdded, PkgRemoved         []string
	ForeignAdded, ForeignRemoved []string
	StartupAdded, StartupRemoved []StartupItem
}

// Empty reports whether nothing changed.
func (d ExtraDiff) Empty() bool {
	return len(d.PkgAdded)+len(d.PkgRemoved)+len(d.StartupAdded)+len(d.StartupRemoved) == 0
}

// DiffExtra compares the extra parts of two states. If either side has no
// package information, package changes are not reported.
func DiffExtra(o, n Extra) ExtraDiff {
	var d ExtraDiff
	if o.Packages != nil && n.Packages != nil {
		d.PkgAdded, d.PkgRemoved = setDiff(o.Packages.Explicit, n.Packages.Explicit)
		d.ForeignAdded, d.ForeignRemoved = setDiff(o.Packages.Foreign, n.Packages.Foreign)
	}
	om := map[string]StartupItem{}
	for _, s := range o.Startup {
		om[s.Key()] = s
	}
	nm := map[string]StartupItem{}
	for _, s := range n.Startup {
		nm[s.Key()] = s
		if _, ok := om[s.Key()]; !ok {
			d.StartupAdded = append(d.StartupAdded, s)
		}
	}
	for _, s := range o.Startup {
		if _, ok := nm[s.Key()]; !ok {
			d.StartupRemoved = append(d.StartupRemoved, s)
		}
	}
	return d
}

func setDiff(a, b []string) (added, removed []string) {
	am, bm := map[string]bool{}, map[string]bool{}
	for _, x := range a {
		am[x] = true
	}
	for _, x := range b {
		bm[x] = true
		if !am[x] {
			added = append(added, x)
		}
	}
	for _, x := range a {
		if !bm[x] {
			removed = append(removed, x)
		}
	}
	return
}
