package tui

import (
	"strings"

	"github.com/iYeremy/dotka/internal/gen"
	"github.com/iYeremy/dotka/internal/scan"
)

// Plain-language explanations shown in detail panes. The audience knows
// how to use a terminal but not necessarily what an XDG autostart entry or
// a systemd user unit is.

func explainKind(kind string) string {
	switch {
	case kind == "hypr-autostart":
		return "Hyprland starts this program once, when you log in (inside hl.on(\"hyprland.start\") in your Lua config)."
	case kind == "hypr-exec-once":
		return "Hyprland starts this program once, when you log in (exec-once in hyprland.conf)."
	case kind == "hypr-exec" || kind == "hypr-execr":
		return "Hyprland runs this every time its config is loaded: at login and on every reload."
	case kind == "hypr-lua-exec":
		return "Hyprland runs this every time the Lua config is loaded (it is not inside a startup block), so also on every reload."
	case kind == "hypr-exec-shutdown":
		return "Hyprland runs this when you log out."
	case strings.HasPrefix(kind, "hypr-on:"):
		return "Hyprland runs this when the event \"" + strings.TrimPrefix(kind, "hypr-on:") + "\" happens."
	case kind == "xdg-autostart":
		return "Your desktop session starts this app at login, from a .desktop file in ~/.config/autostart."
	case kind == "systemd-user":
		return "A background service that systemd starts for your user. It keeps running while you are logged in."
	case kind == "x11-exec":
		return "Runs when an X11 session starts (from .xinitrc or .xprofile)."
	case kind == "shell-remote":
		return "A line in your shell config that runs every time you open a terminal, and downloads code to do it."
	case kind == "shell-plugins":
		return "Loads a shell plugin manager (oh-my-zsh, zinit...) every time you open a terminal."
	case kind == "shell-eval":
		return "Runs a command and executes its output every time you open a terminal (common for prompts like starship)."
	case kind == "shell-source":
		return "Loads another file into your shell every time you open a terminal."
	case kind == "shell-exec":
		return "Runs a script every time you open a terminal."
	}
	return "Runs automatically."
}

func explainRisk(risk string) string {
	switch risk {
	case "remote-exec":
		return "Risk: it downloads code from the internet and runs it. Whoever controls that address can run anything on your account, every time this starts."
	case "local-script":
		return "Note: it runs a script stored in your home. Read the script to know what it does (dotka doctor checks it for you)."
	}
	return ""
}

func explainOrigin(g gen.Generation) string {
	switch {
	case g.Run != nil:
		return "created by `dotka run` after running: " + g.Run.Display()
	case g.Origin == gen.OriginInit:
		return "created when dotka was set up"
	case g.Origin == gen.OriginPreRollback:
		return "saved automatically before going back, so that step can be undone"
	case g.Origin == gen.OriginPreRun:
		return "saved automatically before running a command"
	case g.Origin == gen.OriginManual:
		return "saved by you"
	}
	return g.Origin
}

func explainChange(c gen.Change) string {
	switch c.Kind {
	case gen.Added:
		if c.New.Type == scan.TypeDir {
			return "This folder did not exist in your save point. Going back would remove it (its files are kept in dotka's store)."
		}
		return "This did not exist in your save point. Going back would remove it (a copy is kept in dotka's store)."
	case gen.Removed:
		return "This existed in your save point and is gone now. Going back would restore it."
	case gen.TypeChg:
		if c.Old.Type == scan.TypeFile && c.New.Type == scan.TypeSymlink {
			return "Your file was replaced by a link to another place (typical of dotfiles installers). Going back restores your original file."
		}
		return "It changed from " + c.Old.Type + " to " + c.New.Type + ". Going back restores the " + c.Old.Type + "."
	case gen.ModeChg:
		return "Only its permissions changed. Going back restores them."
	}
	return "Its content changed. Going back restores the saved version."
}

func explainCheck(check string) string {
	switch check {
	case "missing-program":
		return "Something tries to start this program at login, but it is not installed, so it fails silently."
	case "startup-script-remote":
		return "A script that runs automatically downloads code from the internet and runs it."
	case "startup-script-sudo":
		return "A script that runs automatically asks for administrator rights. Login scripts should not need that."
	case "broken-symlink":
		return "A link points to something that no longer exists (often a deleted dotfiles repo)."
	case "exposed-secret":
		return "A file with passwords or tokens can be read by other users of this computer."
	case "duplicate-startup":
		return "The same program is started from more than one place, so it may run twice."
	case "leftover-config":
		return "No installed program matches this config folder. It may be left over from an app you removed. This is a guess: double-check before deleting."
	}
	return ""
}

// kindTitle is a short, friendly name for a startup item kind.
func kindTitle(kind string) string {
	switch {
	case kind == "hypr-autostart" || kind == "hypr-exec-once":
		return "Starts at login (Hyprland)"
	case kind == "hypr-exec" || kind == "hypr-execr" || kind == "hypr-lua-exec":
		return "Runs on every Hyprland reload"
	case kind == "hypr-exec-shutdown":
		return "Runs at logout (Hyprland)"
	case strings.HasPrefix(kind, "hypr-on:"):
		return "Runs on a Hyprland event"
	case kind == "xdg-autostart":
		return "Starts at login (desktop autostart)"
	case kind == "systemd-user":
		return "Background service (systemd)"
	case kind == "x11-exec":
		return "Starts with the X11 session"
	case strings.HasPrefix(kind, "shell"):
		return "Runs when you open a terminal"
	}
	return "Runs automatically"
}

// checkTitle is a short, friendly name for a doctor check.
func checkTitle(check string) string {
	switch check {
	case "missing-program":
		return "Missing program"
	case "startup-script-remote":
		return "Startup script downloads code"
	case "startup-script-sudo":
		return "Startup script uses sudo"
	case "broken-symlink":
		return "Broken link"
	case "exposed-secret":
		return "Secret readable by others"
	case "duplicate-startup":
		return "Started twice"
	case "leftover-config":
		return "Possible leftover"
	}
	return check
}
