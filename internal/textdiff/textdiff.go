// Package textdiff produces unified line diffs for small text files.
package textdiff

import (
	"bytes"
	"fmt"
	"strings"
	"unicode/utf8"
)

// maxLines bounds the O(n*m) LCS; bigger files only get a summary.
const maxLines = 4000

// IsBinary uses the same heuristic as git: a NUL byte in the first 8 KiB, or
// invalid UTF-8.
func IsBinary(b []byte) bool {
	head := b
	if len(head) > 8192 {
		head = head[:8192]
	}
	return bytes.IndexByte(head, 0) >= 0 || !utf8.Valid(head)
}

type op struct {
	kind byte // ' ', '-', '+'
	text string
}

// Unified returns a unified diff of a and b with the given context lines.
// It returns "" when both are equal.
func Unified(aName, bName string, a, b []byte, context int) string {
	if bytes.Equal(a, b) {
		return ""
	}
	if IsBinary(a) || IsBinary(b) {
		return fmt.Sprintf("Binary files %s and %s differ\n", aName, bName)
	}
	al, bl := splitLines(string(a)), splitLines(string(b))
	if len(al) > maxLines || len(bl) > maxLines {
		return fmt.Sprintf("Files %s and %s differ (too large for a line diff: %d -> %d lines)\n", aName, bName, len(al), len(bl))
	}
	ops := lcsOps(al, bl)

	var sb strings.Builder
	fmt.Fprintf(&sb, "--- %s\n+++ %s\n", aName, bName)
	// Group ops into hunks.
	i := 0
	for i < len(ops) {
		if ops[i].kind == ' ' {
			i++
			continue
		}
		start := max(i-context, 0)
		end := i
		// Extend while changes are within 2*context of each other.
		for end < len(ops) {
			if ops[end].kind != ' ' {
				end++
				continue
			}
			j := end
			for j < len(ops) && ops[j].kind == ' ' && j-end < 2*context {
				j++
			}
			if j < len(ops) && ops[j].kind != ' ' {
				end = j
				continue
			}
			break
		}
		stop := min(end+context, len(ops))
		aStart, bStart := lineNumbers(ops, start)
		var aCount, bCount int
		for _, o := range ops[start:stop] {
			if o.kind != '+' {
				aCount++
			}
			if o.kind != '-' {
				bCount++
			}
		}
		fmt.Fprintf(&sb, "@@ -%d,%d +%d,%d @@\n", aStart, aCount, bStart, bCount)
		for _, o := range ops[start:stop] {
			sb.WriteByte(o.kind)
			sb.WriteString(o.text)
			sb.WriteByte('\n')
		}
		i = stop
	}
	return sb.String()
}

func lineNumbers(ops []op, upto int) (int, int) {
	a, b := 1, 1
	for _, o := range ops[:upto] {
		if o.kind != '+' {
			a++
		}
		if o.kind != '-' {
			b++
		}
	}
	return a, b
}

func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	s = strings.TrimSuffix(s, "\n")
	return strings.Split(s, "\n")
}

func lcsOps(a, b []string) []op {
	n, m := len(a), len(b)
	dp := make([][]int32, n+1)
	for i := range dp {
		dp[i] = make([]int32, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if a[i] == b[j] {
				dp[i][j] = dp[i+1][j+1] + 1
			} else {
				dp[i][j] = max(dp[i+1][j], dp[i][j+1])
			}
		}
	}
	var ops []op
	i, j := 0, 0
	for i < n && j < m {
		switch {
		case a[i] == b[j]:
			ops = append(ops, op{' ', a[i]})
			i++
			j++
		case dp[i+1][j] >= dp[i][j+1]:
			ops = append(ops, op{'-', a[i]})
			i++
		default:
			ops = append(ops, op{'+', b[j]})
			j++
		}
	}
	for ; i < n; i++ {
		ops = append(ops, op{'-', a[i]})
	}
	for ; j < m; j++ {
		ops = append(ops, op{'+', b[j]})
	}
	return ops
}
