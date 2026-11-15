package doctor

import (
	"os"
	"os/exec"

	"github.com/iYeremy/dotka/internal/config"
	"github.com/iYeremy/dotka/internal/pkgs"
	"github.com/iYeremy/dotka/internal/scan"
	"github.com/iYeremy/dotka/internal/startup"
)

// Collect gathers the input for the checks from a real home directory.
// It only reads (metadata-only scan, startup files, package list).
func Collect(home string, cfg config.Config) (Input, error) {
	res, err := scan.Scan(scan.Options{Home: home, Config: cfg, NoHash: true})
	if err != nil {
		return Input{}, err
	}
	items, _ := startup.Extract(home)
	in := Input{Home: home, Entries: res.Entries, Startup: items, LookPath: exec.LookPath,
		XDGConfigHome: os.Getenv("XDG_CONFIG_HOME")}
	if src := pkgs.FromEnv(); src != nil {
		if all, err := src.All(); err == nil {
			in.Packages = all
		}
	}
	return in, nil
}
