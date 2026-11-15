// Package scan walks the tracked roots of a home directory and produces the
// list of entries that make up the file part of a generation.
package scan

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/iYeremy/dotka/internal/config"
	"github.com/iYeremy/dotka/internal/secrets"
	"github.com/iYeremy/dotka/internal/store"
)

// Entry types.
const (
	TypeFile    = "file"
	TypeDir     = "dir"
	TypeSymlink = "symlink"
)

// Skip reasons: the entry is recorded but its content is not in the store.
const (
	SkipSecret = "secret"
	SkipLarge  = "too-large"
)

// Entry is one tracked filesystem object, relative to $HOME.
type Entry struct {
	Path   string `json:"path"`
	Type   string `json:"type"`
	Mode   uint32 `json:"mode"`             // permission bits
	Size   int64  `json:"size,omitempty"`   // files only
	MTime  int64  `json:"mtime,omitempty"`  // unix nanos, used only as a hash cache key
	Hash   string `json:"hash,omitempty"`   // sha256 of the content (files)
	Target string `json:"target,omitempty"` // symlinks
	// Stored is true when the content is available in the store and the
	// entry can be restored.
	Stored bool   `json:"stored,omitempty"`
	Skip   string `json:"skip,omitempty"`
	Reason string `json:"reason,omitempty"`
	// Clean records that the content passed the secret check (it is false
	// for files the user explicitly allowed, which were never checked).
	Clean bool `json:"clean,omitempty"`
}

// NixManaged reports whether the entry is a link into the Nix store, i.e.
// owned by home-manager / NixOS. Dotka shows these but never changes them.
func (e Entry) NixManaged() bool {
	return e.Type == TypeSymlink && strings.HasPrefix(e.Target, "/nix/store/")
}

// Restorable reports whether Dotka can recreate this entry exactly.
func (e Entry) Restorable() bool {
	switch e.Type {
	case TypeDir, TypeSymlink:
		return true
	case TypeFile:
		return e.Stored
	}
	return false
}

// SameContent compares what matters for a diff (not mtime).
func (e Entry) SameContent(o Entry) bool {
	if e.Type != o.Type {
		return false
	}
	switch e.Type {
	case TypeFile:
		if e.Skip == SkipSecret || o.Skip == SkipSecret {
			// Content of secrets is never hashed; compare size only.
			return e.Size == o.Size && e.MTime == o.MTime
		}
		return e.Hash == o.Hash
	case TypeSymlink:
		return e.Target == o.Target
	}
	return true
}

// Options control a scan.
type Options struct {
	Home   string
	Config config.Config
	// Store, when non-nil, receives the content of every regular file
	// (snapshot mode). When nil, files are only hashed (status mode).
	Store *store.Store
	// NoHash skips reading file contents (fast, metadata-only scans).
	NoHash bool
	// Previous entries used as a hash cache: if size, mtime and mode are
	// unchanged the previous hash is reused instead of re-reading the file.
	Previous map[string]Entry
}

// Warning is a non-fatal problem found during the scan.
type Warning struct {
	Path string
	Err  error
}

// Result of a scan.
type Result struct {
	Entries  []Entry
	Warnings []Warning
}

// Scan walks every configured root and returns sorted entries.
func Scan(opt Options) (Result, error) {
	var res Result
	seen := map[string]bool{}
	for _, root := range opt.Config.Roots {
		full := filepath.Join(opt.Home, root)
		if _, err := os.Lstat(full); errors.Is(err, os.ErrNotExist) {
			continue
		}
		err := filepath.WalkDir(full, func(p string, d fs.DirEntry, err error) error {
			rel, rerr := filepath.Rel(opt.Home, p)
			if rerr != nil {
				return rerr
			}
			if err != nil {
				res.Warnings = append(res.Warnings, Warning{rel, err})
				if d != nil && d.IsDir() {
					return fs.SkipDir
				}
				return nil
			}
			if seen[rel] {
				if d.IsDir() {
					return fs.SkipDir
				}
				return nil
			}
			if opt.Config.Excluded(rel) {
				if d.IsDir() {
					return fs.SkipDir
				}
				return nil
			}
			if secrets.IsSecretDir(rel) {
				if d.IsDir() {
					return fs.SkipDir
				}
				return nil
			}
			e, ok, err := entryFor(opt, p, rel, d)
			if err != nil {
				res.Warnings = append(res.Warnings, Warning{rel, err})
				return nil
			}
			if ok {
				seen[rel] = true
				res.Entries = append(res.Entries, e)
			}
			return nil
		})
		if err != nil {
			return res, err
		}
	}
	sort.Slice(res.Entries, func(i, j int) bool { return res.Entries[i].Path < res.Entries[j].Path })
	return res, nil
}

func entryFor(opt Options, full, rel string, d fs.DirEntry) (Entry, bool, error) {
	info, err := d.Info()
	if err != nil {
		return Entry{}, false, err
	}
	mode := info.Mode()
	e := Entry{Path: rel, Mode: uint32(mode.Perm())}
	switch {
	case mode&fs.ModeSymlink != 0:
		// Never follow symlinks: record them as such.
		t, err := os.Readlink(full)
		if err != nil {
			return Entry{}, false, err
		}
		e.Type, e.Target, e.Mode = TypeSymlink, t, 0o777
		return e, true, nil
	case mode.IsDir():
		e.Type = TypeDir
		return e, true, nil
	case mode.IsRegular():
		e.Type = TypeFile
		e.Size = info.Size()
		e.MTime = info.ModTime().UnixNano()
	default:
		// Sockets, FIFOs, devices: ignored on purpose.
		return Entry{}, false, nil
	}

	prev, hasPrev := opt.Previous[rel]
	unchanged := hasPrev && prev.Type == TypeFile && prev.Size == e.Size && prev.MTime == e.MTime && prev.Mode == e.Mode
	if !opt.Config.SecretAllowed(rel) {
		if ok, why := secrets.CheckPath(rel); ok {
			e.Skip, e.Reason = SkipSecret, why
			return e, true, nil
		}
		switch {
		case unchanged && prev.Skip == SkipSecret:
			e.Skip, e.Reason = SkipSecret, prev.Reason
			return e, true, nil
		case unchanged && prev.Clean:
			// Same content as last time, which passed the check.
			e.Clean = true
		default:
			if ok, why := secrets.CheckContent(full); ok {
				e.Skip, e.Reason = SkipSecret, why
				return e, true, nil
			}
			e.Clean = true
		}
	}

	if opt.NoHash {
		return e, true, nil
	}

	large := e.Size > opt.Config.MaxFileSize
	if large {
		e.Skip, e.Reason = SkipLarge, "bigger than max_file_size"
	}

	// Hash cache: unchanged size, mtime and mode means unchanged content.
	if prev, ok := opt.Previous[rel]; ok && prev.Type == TypeFile && prev.Hash != "" &&
		prev.Skip != SkipSecret && prev.Size == e.Size && prev.MTime == e.MTime && prev.Mode == e.Mode {
		e.Hash = prev.Hash
		switch {
		case large:
			return e, true, nil
		case opt.Store == nil:
			e.Stored = prev.Stored
			return e, true, nil
		case opt.Store.Has(e.Hash):
			e.Stored = true
			return e, true, nil
		}
		// Snapshot mode and the object is missing: store it below.
	}

	switch {
	case large:
		h, err := store.HashFile(full)
		if err != nil {
			return Entry{}, false, err
		}
		e.Hash = h
	case opt.Store != nil:
		h, err := opt.Store.PutFile(full)
		if err != nil {
			return Entry{}, false, err
		}
		e.Hash, e.Stored = h, true
	default:
		h, err := store.HashFile(full)
		if err != nil {
			return Entry{}, false, err
		}
		e.Hash = h
	}
	return e, true, nil
}

// Index builds a path → entry map.
func Index(entries []Entry) map[string]Entry {
	m := make(map[string]Entry, len(entries))
	for _, e := range entries {
		m[e.Path] = e
	}
	return m
}
