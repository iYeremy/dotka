// Package config holds what Dotka tracks and where it keeps its data.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/iYeremy/dotka/internal/fsutil"
)

// FileName is the config file inside the store directory.
const FileName = "config.json"

// Config describes the tracked roots. All paths are relative to $HOME.
type Config struct {
	// Roots are directories or single files tracked recursively.
	Roots []string `json:"roots"`
	// Exclude are path patterns relative to $HOME. A pattern matches a path if
	// it is equal to it, is a parent directory of it, or matches it with
	// filepath.Match semantics.
	Exclude []string `json:"exclude"`
	// ExcludeNames are base names excluded anywhere (typical cache dirs).
	ExcludeNames []string `json:"exclude_names"`
	// MaxFileSize in bytes. Bigger files are hashed but their content is not
	// stored, so they cannot be restored.
	MaxFileSize int64 `json:"max_file_size"`
	// AllowSecrets lists paths the user explicitly allowed to be stored even
	// though they look like secrets.
	AllowSecrets []string `json:"allow_secrets"`
}

// Default returns the configuration used by `dotka init`.
func Default() Config {
	return Config{
		Roots: []string{
			".config",
			".local/bin",
			".local/share/fonts",
			".local/share/icons",
			".local/share/themes",
			".local/share/applications",
			".themes", ".icons", ".fonts",
			".bashrc", ".bash_profile", ".zshrc", ".zprofile", ".zshenv", ".profile",
			".xinitrc", ".xprofile", ".Xresources",
			".gitconfig", ".tmux.conf", ".vimrc",
		},
		Exclude: []string{
			".cache",
			".mozilla",
			".config/chromium",
			".config/google-chrome",
			".config/BraveSoftware",
			".config/vivaldi",
			".config/Code/User/workspaceStorage",
			".config/Code/User/globalStorage",
			".local/share/Steam",
			".local/share/Trash",
			".local/share/dotka",
		},
		ExcludeNames: []string{
			"Cache", "Code Cache", "GPUCache", "CachedData", "CachedExtensions",
			"Service Worker", "IndexedDB", "Local Storage", "Session Storage",
			"blob_storage", "Crashpad", "DawnCache", "DawnGraphiteCache", "DawnWebGPUCache", "GrShaderCache",
			"ShaderCache", "logs", "node_modules", "__pycache__", ".git",
		},
		MaxFileSize: 10 << 20,
	}
}

// Load reads the config from the store directory.
func Load(storeDir string) (Config, error) {
	b, err := os.ReadFile(filepath.Join(storeDir, FileName))
	if err != nil {
		return Config{}, err
	}
	var c Config
	if err := json.Unmarshal(b, &c); err != nil {
		return Config{}, fmt.Errorf("parsing %s: %w", FileName, err)
	}
	return c, c.Validate()
}

// Save writes the config atomically into the store directory.
func (c Config) Save(storeDir string) error {
	if err := c.Validate(); err != nil {
		return err
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return fsutil.AtomicWrite(filepath.Join(storeDir, FileName), strings.NewReader(string(b)+"\n"), 0o600)
}

// Validate checks that every root is a safe relative path.
func (c Config) Validate() error {
	for _, r := range c.Roots {
		if _, err := fsutil.CleanRel(r); err != nil {
			return fmt.Errorf("invalid root: %w", err)
		}
	}
	if c.MaxFileSize <= 0 {
		return errors.New("max_file_size must be positive")
	}
	return nil
}

// AddRoot adds a root (relative to home). Returns false if already present.
func (c *Config) AddRoot(rel string) (bool, error) {
	r, err := fsutil.CleanRel(rel)
	if err != nil {
		return false, err
	}
	if slices.Contains(c.Roots, r) {
		return false, nil
	}
	c.Roots = append(c.Roots, r)
	return true, nil
}

// RemoveRoot removes a root. Returns false if it was not present.
func (c *Config) RemoveRoot(rel string) bool {
	r := filepath.Clean(rel)
	i := slices.Index(c.Roots, r)
	if i < 0 {
		return false
	}
	c.Roots = slices.Delete(c.Roots, i, i+1)
	return true
}

// Excluded reports whether a home-relative path is excluded.
func (c Config) Excluded(rel string) bool {
	base := filepath.Base(rel)
	if slices.Contains(c.ExcludeNames, base) {
		return true
	}
	for _, p := range c.Exclude {
		if rel == p || strings.HasPrefix(rel, p+"/") {
			return true
		}
		if ok, _ := filepath.Match(p, rel); ok {
			return true
		}
	}
	return false
}

// SecretAllowed reports whether the user allowed storing this path.
func (c Config) SecretAllowed(rel string) bool {
	return slices.Contains(c.AllowSecrets, rel)
}

// UnderRoots reports whether rel is one of roots or inside one of them.
func UnderRoots(roots []string, rel string) bool {
	for _, r := range roots {
		if rel == r || strings.HasPrefix(rel, r+"/") {
			return true
		}
	}
	return false
}
