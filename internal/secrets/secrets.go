// Package secrets detects files that probably contain credentials so Dotka
// does not copy them into its store without explicit consent.
package secrets

import (
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// knownPaths are home-relative paths (or path globs) that hold credentials.
var knownPaths = []string{
	".config/gh/hosts.yml",
	".config/hub",
	".config/rclone/rclone.conf",
	".config/git/credentials",
	".git-credentials",
	".netrc",
	".pgpass",
	".config/spotify-tui/client.yml",
	".config/aerc/accounts.conf",
	".config/himalaya/config.toml",
	".config/msmtp/config",
	".msmtprc",
	".config/isync/mbsyncrc",
	".mbsyncrc",
	".config/Bitwarden*",
	".config/keepassxc/*",
	".config/*/token*",
	".config/*/*token*",
	".config/*/credentials*",
	".config/*/secret*",
}

// knownDirs are excluded as a whole.
var knownDirs = []string{".ssh", ".gnupg", ".password-store", ".local/share/keyrings"}

type pattern struct {
	name string
	re   *regexp.Regexp
}

var contentPatterns = []pattern{
	{"private key", regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----`)},
	{"GitHub token", regexp.MustCompile(`\b(ghp|gho|ghu|ghs|ghr)_[A-Za-z0-9]{36}\b|\bgithub_pat_[A-Za-z0-9_]{40,}`)},
	{"GitLab token", regexp.MustCompile(`\bglpat-[A-Za-z0-9_\-]{20,}`)},
	{"Slack token", regexp.MustCompile(`\bxox[baprs]-[A-Za-z0-9-]{10,}`)},
	{"AWS access key", regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`)},
	{"API key", regexp.MustCompile(`\bsk-(ant-)?[A-Za-z0-9_\-]{24,}`)},
	{"password assignment", regexp.MustCompile(`(?im)^\s*(password|passwd|pass)\s*[=:]\s*["']?[^\s"'$]{4,}`)},
}

// maxScan bounds how much of a file is inspected.
const maxScan = 256 << 10

// IsSecretDir reports whether rel is (inside) a directory that is never tracked.
func IsSecretDir(rel string) bool {
	for _, d := range knownDirs {
		if rel == d || strings.HasPrefix(rel, d+"/") {
			return true
		}
	}
	return false
}

// CheckPath reports whether a home-relative path is a known secret location.
func CheckPath(rel string) (bool, string) {
	for _, d := range knownDirs {
		if rel == d || strings.HasPrefix(rel, d+"/") {
			return true, "known secret directory " + d
		}
	}
	for _, p := range knownPaths {
		if rel == p {
			return true, "known credentials file"
		}
		if ok, _ := filepath.Match(p, rel); ok {
			return true, "matches " + p
		}
	}
	return false, ""
}

// CheckContent inspects the beginning of a file for credential patterns.
func CheckContent(path string) (bool, string) {
	f, err := os.Open(path)
	if err != nil {
		return false, ""
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, maxScan))
	if err != nil {
		return false, ""
	}
	for _, p := range contentPatterns {
		if p.re.Match(b) {
			return true, "contains " + p.name
		}
	}
	return false, ""
}
