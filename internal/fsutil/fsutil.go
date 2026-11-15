// Package fsutil contains the filesystem primitives Dotka relies on to never
// lose data: safe path resolution inside $HOME and atomic writes.
package fsutil

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// ErrUnsafePath is returned when a relative path would escape the base
// directory or traverse a symlink.
var ErrUnsafePath = errors.New("unsafe path")

// CleanRel validates a path that is meant to be relative to a base directory
// (for example a manifest entry relative to $HOME). It rejects absolute paths,
// empty paths and any ".." component.
func CleanRel(rel string) (string, error) {
	if rel == "" || filepath.IsAbs(rel) {
		return "", fmt.Errorf("%w: %q", ErrUnsafePath, rel)
	}
	c := filepath.Clean(rel)
	if c == "." || c == ".." || strings.HasPrefix(c, "../") {
		return "", fmt.Errorf("%w: %q", ErrUnsafePath, rel)
	}
	for _, part := range strings.Split(c, string(filepath.Separator)) {
		if part == ".." {
			return "", fmt.Errorf("%w: %q", ErrUnsafePath, rel)
		}
	}
	return c, nil
}

// SafeJoin joins base and rel, guaranteeing the result stays inside base and
// that no existing parent component between base and the final element is a
// symlink (which could redirect a write outside base). The final element
// itself may be a symlink: callers decide how to treat it (usually Lstat).
func SafeJoin(base, rel string) (string, error) {
	c, err := CleanRel(rel)
	if err != nil {
		return "", err
	}
	full := filepath.Join(base, c)
	parts := strings.Split(c, string(filepath.Separator))
	cur := base
	for _, p := range parts[:len(parts)-1] {
		cur = filepath.Join(cur, p)
		fi, err := os.Lstat(cur)
		if errors.Is(err, os.ErrNotExist) {
			break // the rest does not exist yet; nothing to traverse
		}
		if err != nil {
			return "", err
		}
		if fi.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("%w: parent %q is a symlink", ErrUnsafePath, cur)
		}
		if !fi.IsDir() {
			return "", fmt.Errorf("%w: parent %q is not a directory", ErrUnsafePath, cur)
		}
	}
	return full, nil
}

// AtomicWrite writes the content of r to path through a temporary file in the
// same directory, fsyncs it and renames it over the destination. The
// destination is never left half-written.
func AtomicWrite(path string, r io.Reader, perm os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".dotka-tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	ok := false
	defer func() {
		if !ok {
			tmp.Close()
			os.Remove(tmpName)
		}
	}()
	if _, err := io.Copy(tmp, r); err != nil {
		return err
	}
	if err := tmp.Chmod(perm); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	ok = true
	return syncDir(dir)
}

// AtomicSymlink creates (or replaces) a symlink at path pointing to target.
func AtomicSymlink(target, path string) error {
	dir := filepath.Dir(path)
	f, err := os.CreateTemp(dir, ".dotka-lnk-*")
	if err != nil {
		return err
	}
	tmpName := f.Name()
	f.Close()
	os.Remove(tmpName)
	if err := os.Symlink(target, tmpName); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return err
	}
	return syncDir(dir)
}

func syncDir(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	// Some filesystems do not support fsync on directories; ignore that.
	_ = d.Sync()
	return nil
}
