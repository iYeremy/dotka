package scan

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/iYeremy/dotka/internal/config"
	"github.com/iYeremy/dotka/internal/store"
)

// fakeHome builds a small home directory for tests.
func fakeHome(t *testing.T) string {
	t.Helper()
	h := t.TempDir()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(os.MkdirAll(filepath.Join(h, ".config/hypr"), 0o755))
	must(os.MkdirAll(filepath.Join(h, ".config/discord/Cache"), 0o755))
	must(os.MkdirAll(filepath.Join(h, ".config/gh"), 0o755))
	must(os.MkdirAll(filepath.Join(h, ".cache"), 0o755))
	must(os.WriteFile(filepath.Join(h, ".config/hypr/hyprland.conf"), []byte("exec-once = waybar\n"), 0o644))
	must(os.WriteFile(filepath.Join(h, ".config/discord/Cache/blob"), []byte("x"), 0o644))
	must(os.WriteFile(filepath.Join(h, ".config/gh/hosts.yml"), []byte("oauth_token: x\n"), 0o600))
	must(os.WriteFile(filepath.Join(h, ".cache/junk"), []byte("x"), 0o644))
	must(os.WriteFile(filepath.Join(h, ".zshrc"), []byte("echo hi\n"), 0o644))
	must(os.Symlink("hyprland.conf", filepath.Join(h, ".config/hypr/link.conf")))
	must(os.Symlink("/nonexistent/target", filepath.Join(h, ".config/broken")))
	must(os.Symlink("/etc", filepath.Join(h, ".config/etc-link"))) // must not be followed
	return h
}

func TestScanClassifiesAndExcludes(t *testing.T) {
	h := fakeHome(t)
	cfg := config.Default()
	cfg.Roots = []string{".config", ".zshrc", ".missing"}
	res, err := Scan(Options{Home: h, Config: cfg})
	if err != nil {
		t.Fatal(err)
	}
	idx := Index(res.Entries)
	want := map[string]string{
		".config":                    TypeDir,
		".config/hypr":               TypeDir,
		".config/hypr/hyprland.conf": TypeFile,
		".config/hypr/link.conf":     TypeSymlink,
		".config/broken":             TypeSymlink,
		".config/etc-link":           TypeSymlink,
		".config/gh/hosts.yml":       TypeFile,
		".zshrc":                     TypeFile,
	}
	for p, typ := range want {
		if e, ok := idx[p]; !ok || e.Type != typ {
			t.Errorf("%s: got %+v, want type %s", p, e, typ)
		}
	}
	for _, p := range []string{".config/discord/Cache", ".config/discord/Cache/blob", ".cache/junk", ".config/etc-link/passwd"} {
		if _, ok := idx[p]; ok {
			t.Errorf("%s should not be scanned", p)
		}
	}
	if idx[".config/gh/hosts.yml"].Skip != SkipSecret {
		t.Error("gh hosts.yml should be marked as secret")
	}
	if idx[".config/broken"].Target != "/nonexistent/target" {
		t.Error("broken symlink target not recorded")
	}
}

func TestSnapshotStoresContentButNotSecrets(t *testing.T) {
	h := fakeHome(t)
	s, err := store.Create(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	cfg.Roots = []string{".config", ".zshrc"}
	res, err := Scan(Options{Home: h, Config: cfg, Store: s})
	if err != nil {
		t.Fatal(err)
	}
	idx := Index(res.Entries)
	z := idx[".zshrc"]
	if !z.Stored || !s.Has(z.Hash) {
		t.Fatalf(".zshrc not stored: %+v", z)
	}
	sec := idx[".config/gh/hosts.yml"]
	if sec.Stored || sec.Hash != "" {
		t.Fatalf("secret must not be hashed or stored: %+v", sec)
	}
	// Allowing it explicitly stores it.
	cfg.AllowSecrets = []string{".config/gh/hosts.yml"}
	res, _ = Scan(Options{Home: h, Config: cfg, Store: s})
	if e := Index(res.Entries)[".config/gh/hosts.yml"]; !e.Stored {
		t.Fatalf("allowed secret not stored: %+v", e)
	}
}

func TestLargeFilesAreNotStored(t *testing.T) {
	h := t.TempDir()
	os.MkdirAll(filepath.Join(h, ".config"), 0o755)
	os.WriteFile(filepath.Join(h, ".config/big.bin"), make([]byte, 2048), 0o644)
	s, _ := store.Create(filepath.Join(t.TempDir(), "store"))
	cfg := config.Default()
	cfg.Roots = []string{".config"}
	cfg.MaxFileSize = 1024
	res, _ := Scan(Options{Home: h, Config: cfg, Store: s})
	e := Index(res.Entries)[".config/big.bin"]
	if e.Skip != SkipLarge || e.Stored || e.Hash == "" || e.Restorable() {
		t.Fatalf("large file entry wrong: %+v", e)
	}
}

func TestHashCacheDetectsChanges(t *testing.T) {
	h := fakeHome(t)
	cfg := config.Default()
	cfg.Roots = []string{".zshrc"}
	r1, _ := Scan(Options{Home: h, Config: cfg})
	p := filepath.Join(h, ".zshrc")
	os.WriteFile(p, []byte("echo changed, longer\n"), 0o644)
	r2, _ := Scan(Options{Home: h, Config: cfg, Previous: Index(r1.Entries)})
	if r1.Entries[0].Hash == r2.Entries[0].Hash {
		t.Fatal("change not detected with hash cache")
	}
}

func TestUnreadableDirIsWarningNotFailure(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores permissions")
	}
	h := t.TempDir()
	locked := filepath.Join(h, ".config/locked")
	os.MkdirAll(locked, 0o755)
	os.WriteFile(filepath.Join(locked, "f"), []byte("x"), 0o644)
	os.Chmod(locked, 0o000)
	t.Cleanup(func() { os.Chmod(locked, 0o755) })
	cfg := config.Default()
	cfg.Roots = []string{".config"}
	res, err := Scan(Options{Home: h, Config: cfg})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Warnings) == 0 {
		t.Fatal("expected a warning for the unreadable directory")
	}
}

func TestSecretCheckCacheAndDeny(t *testing.T) {
	h := t.TempDir()
	os.MkdirAll(filepath.Join(h, ".config/mail"), 0o755)
	p := filepath.Join(h, ".config/mail/conf")
	os.WriteFile(p, []byte("server = x\n"), 0o644)
	cfg := config.Default()
	cfg.Roots = []string{".config"}
	r1, _ := Scan(Options{Home: h, Config: cfg})
	if e := Index(r1.Entries)[".config/mail/conf"]; !e.Clean || e.Skip != "" {
		t.Fatalf("clean file: %+v", e)
	}
	// Unchanged file: the cached clean result is reused.
	r2, _ := Scan(Options{Home: h, Config: cfg, Previous: Index(r1.Entries)})
	if !Index(r2.Entries)[".config/mail/conf"].Clean {
		t.Fatal("clean flag not carried over")
	}
	// A secret is added: the file changes, so it is re-checked.
	os.WriteFile(p, []byte("server = x\npassword = hunter22\n"), 0o644)
	r3, _ := Scan(Options{Home: h, Config: cfg, Previous: Index(r2.Entries)})
	if Index(r3.Entries)[".config/mail/conf"].Skip != SkipSecret {
		t.Fatal("changed file with a secret must be detected")
	}
	// Allowed, then denied without touching the file: must be detected
	// again (an allowed file is never marked clean).
	cfg.AllowSecrets = []string{".config/mail/conf"}
	r4, _ := Scan(Options{Home: h, Config: cfg, Previous: Index(r3.Entries)})
	if e := Index(r4.Entries)[".config/mail/conf"]; e.Skip != "" || e.Clean {
		t.Fatalf("allowed secret: %+v", e)
	}
	cfg.AllowSecrets = nil
	r5, _ := Scan(Options{Home: h, Config: cfg, Previous: Index(r4.Entries)})
	if Index(r5.Entries)[".config/mail/conf"].Skip != SkipSecret {
		t.Fatal("denied again but not re-detected")
	}
}
