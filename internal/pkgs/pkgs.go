// Package pkgs reads the list of installed packages. It is strictly
// read-only: Dotka never installs or removes packages.
package pkgs

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"

	"github.com/iYeremy/dotka/internal/gen"
)

// Source lists installed packages.
type Source interface {
	Snapshot() (*gen.Packages, error)
}

// ErrUnavailable means no supported package manager was found.
var ErrUnavailable = errors.New("no supported package manager found")

// FromEnv selects a source from DOTKA_PACKAGES:
//
//	"" or "pacman"  pacman (default)
//	"off"           disabled
//	"file:PATH"     read from a text file (tests and demos): one package per
//	                line, optionally followed by " aur" for foreign packages
func FromEnv() Source {
	v := os.Getenv("DOTKA_PACKAGES")
	switch {
	case v == "off":
		return nil
	case strings.HasPrefix(v, "file:"):
		return FileSource{Path: strings.TrimPrefix(v, "file:")}
	}
	return Pacman{}
}

// Pacman reads explicitly installed (-Qqe) and foreign (-Qqm) packages.
type Pacman struct{}

func (Pacman) Snapshot() (*gen.Packages, error) {
	if _, err := exec.LookPath("pacman"); err != nil {
		return nil, ErrUnavailable
	}
	explicit, err := pacmanQuery("-Qqe")
	if err != nil {
		return nil, err
	}
	foreign, err := pacmanQuery("-Qqm")
	if err != nil {
		return nil, err
	}
	return &gen.Packages{Manager: "pacman", Explicit: explicit, Foreign: foreign}, nil
}

func pacmanQuery(flag string) ([]string, error) {
	cmd := exec.Command("pacman", flag)
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		// pacman -Qqm exits 1 when there are no foreign packages.
		var ee *exec.ExitError
		if errors.As(err, &ee) && ee.ExitCode() == 1 && out.Len() == 0 && errb.Len() == 0 {
			return nil, nil
		}
		return nil, fmt.Errorf("pacman %s: %v %s", flag, err, strings.TrimSpace(errb.String()))
	}
	return ParseList(out.String()), nil
}

// ParseList parses one package name per line.
func ParseList(s string) []string {
	var out []string
	sc := bufio.NewScanner(strings.NewReader(s))
	for sc.Scan() {
		if f := strings.Fields(sc.Text()); len(f) > 0 {
			out = append(out, f[0])
		}
	}
	sort.Strings(out)
	return out
}

// FileSource reads a fake package list (for tests and demos).
type FileSource struct{ Path string }

func (f FileSource) Snapshot() (*gen.Packages, error) {
	b, err := os.ReadFile(f.Path)
	if err != nil {
		return nil, err
	}
	p := &gen.Packages{Manager: "file"}
	sc := bufio.NewScanner(bytes.NewReader(b))
	for sc.Scan() {
		fl := strings.Fields(sc.Text())
		if len(fl) == 0 || strings.HasPrefix(fl[0], "#") {
			continue
		}
		p.Explicit = append(p.Explicit, fl[0])
		if len(fl) > 1 && fl[1] == "aur" {
			p.Foreign = append(p.Foreign, fl[0])
		}
	}
	sort.Strings(p.Explicit)
	sort.Strings(p.Foreign)
	return p, nil
}
