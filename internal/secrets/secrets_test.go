package secrets

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCheckPath(t *testing.T) {
	yes := []string{".config/gh/hosts.yml", ".ssh/id_ed25519", ".netrc", ".config/spotifyd/token.json", ".gnupg"}
	no := []string{".config/hypr/hyprland.lua", ".config/gh/config.yml", ".zshrc"}
	for _, p := range yes {
		if ok, _ := CheckPath(p); !ok {
			t.Errorf("%s should be a secret path", p)
		}
	}
	for _, p := range no {
		if ok, _ := CheckPath(p); ok {
			t.Errorf("%s should not be a secret path", p)
		}
	}
}

func TestCheckContent(t *testing.T) {
	dir := t.TempDir()
	cases := map[string]bool{
		"key":    true,
		"gh":     true,
		"pass":   true,
		"benign": false,
		"envref": false,
	}
	content := map[string]string{
		"key":    "-----BEGIN OPENSSH PRIVATE KEY-----\nabc\n",
		"gh":     "token: ghp_" + "abcdefghijklmnopqrstuvwxyz0123456789\n",
		"pass":   "[account]\npassword = hunter22\n",
		"benign": "exec-once = waybar\n# password manager: bitwarden\n",
		"envref": "password = $MAIL_PASS\n",
	}
	for name, want := range cases {
		p := filepath.Join(dir, name)
		os.WriteFile(p, []byte(content[name]), 0o600)
		if got, why := CheckContent(p); got != want {
			t.Errorf("%s: got %v (%s), want %v", name, got, why, want)
		}
	}
}
