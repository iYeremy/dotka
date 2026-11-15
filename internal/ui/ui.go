// Package ui has tiny helpers for terminal output (colors, relative times).
package ui

import (
	"fmt"
	"os"
	"time"
)

var enabled = func() bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	fi, err := os.Stdout.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}()

func wrap(code, s string) string {
	if !enabled {
		return s
	}
	return "\x1b[" + code + "m" + s + "\x1b[0m"
}

func Green(s string) string  { return wrap("32", s) }
func Red(s string) string    { return wrap("31", s) }
func Yellow(s string) string { return wrap("33", s) }
func Cyan(s string) string   { return wrap("36", s) }
func Bold(s string) string   { return wrap("1", s) }
func Dim(s string) string    { return wrap("2", s) }

// Ago formats a duration in a human way.
func Ago(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%d min ago", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%d h ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%d days ago", int(d.Hours()/24))
	}
}
