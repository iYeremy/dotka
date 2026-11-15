#!/usr/bin/env bash
# Builds a fake $HOME to try Dotka without touching your real configuration.
#
#   scripts/sandbox.sh setup     create .sandbox/ with a fake Hyprland (Lua) home
#   scripts/sandbox.sh install   run a fake third-party "install.sh" inside it
#   scripts/sandbox.sh env       print the exports to point dotka at the sandbox
#   scripts/sandbox.sh clean     delete .sandbox/
#
# Typical session:
#   go build -o dotka ./cmd/dotka
#   scripts/sandbox.sh setup && eval "$(scripts/sandbox.sh env)"
#   ./dotka init
#   scripts/sandbox.sh install
#   ./dotka status
#   ./dotka rollback 1
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
SB="${DOTKA_SANDBOX:-$ROOT/.sandbox}"
FAKE_HOME="$SB/home"
PKGS="$SB/packages.txt"
MARKER=".dotka-sandbox"

die() { echo "sandbox: $*" >&2; exit 1; }

setup() {
  [[ -e "$SB" ]] && die "$SB already exists (run: $0 clean)"
  mkdir -p "$FAKE_HOME"
  cd "$FAKE_HOME"
  touch "$MARKER"
  mkdir -p .config/hypr/scripts .config/kitty .config/gh .config/dunst .local/bin

  cat > .config/hypr/hyprland.lua <<'EOF'
-- ~/.config/hypr/hyprland.lua (sandbox)

---- AUTOSTART ----
hl.on("hyprland.start", function()
    hl.exec_cmd("qs -p ~/.config/quickshell/bar/shell.qml")
    hl.exec_cmd("dunst")
    hl.exec_cmd("nm-applet")
end)

require("keybinds")
EOF
  cat > .config/hypr/keybinds.lua <<'EOF'
local mainMod = "SUPER"
hl.bind(mainMod .. " + T", hl.dsp.exec_cmd("kitty"))
hl.bind(mainMod .. " + D", hl.dsp.exec_cmd("rofi -show drun"))
EOF
  printf '#!/bin/sh\nhyprctl dispatch exit\n' > .config/hypr/scripts/logout.sh
  chmod +x .config/hypr/scripts/logout.sh
  printf 'font_family JetBrains Mono\nfont_size 11\n' > .config/kitty/kitty.conf
  printf '[global]\nfont = Monospace 10\n' > .config/dunst/dunstrc
  printf 'github.com:\n    oauth_token: gho_FAKEFAKEFAKE\n' > .config/gh/hosts.yml
  cat > .zshrc <<'EOF'
export EDITOR=nvim
eval "$(starship init zsh)"
alias ll='ls -la'
EOF

  printf 'hyprland\nkitty\ndunst\nnetwork-manager-applet\nquickshell aur\n' > "$PKGS"
  echo "Sandbox ready at $SB"
  echo "Next: eval \"\$($0 env)\" && ./dotka init"
}

# A fake third-party installer, like the ones in popular dotfiles repos.
# It only writes inside the sandbox and refuses to run anywhere else.
install() {
  [[ -f "$FAKE_HOME/$MARKER" ]] || die "no sandbox at $SB (run: $0 setup)"
  HOME="$FAKE_HOME" bash -s "$PKGS" <<'EOF'
set -euo pipefail
[[ -f "$HOME/.dotka-sandbox" ]] || { echo "refusing: not a sandbox home" >&2; exit 1; }
PKGS="$1"
cd "$HOME"
echo ":: Installing super-rice dotfiles..."

# 1. Overwrite the Hyprland config, adding autostart entries.
cat > .config/hypr/hyprland.lua <<'LUA'
-- super-rice hyprland config
hl.on("hyprland.start", function()
    hl.exec_cmd("waybar")
    hl.exec_cmd("swww-daemon")
    hl.exec_cmd("dunst")
    hl.exec_cmd("~/.local/bin/rice-autoupdate.sh")
end)

require("keybinds")
LUA

# 2. New app configs.
mkdir -p .config/waybar/modules
echo '{ "layer": "top" }' > .config/waybar/config.jsonc
echo '* { font-family: "JetBrainsMono Nerd Font"; }' > .config/waybar/style.css
echo '{ "format": "{:%H:%M}" }' > .config/waybar/modules/clock.json

# 3. Replace the user's kitty.conf with a symlink into the repo.
mkdir -p dots
printf 'font_size 14\nbackground_opacity 0.8\n' > dots/kitty.conf
rm -f .config/kitty/kitty.conf
ln -s "$HOME/dots/kitty.conf" .config/kitty/kitty.conf

# 4. A self-updating script that runs at every login.
cat > .local/bin/rice-autoupdate.sh <<'SH'
#!/bin/sh
curl -fsSL https://example.invalid/rice/update.sh | sh
SH
chmod +x .local/bin/rice-autoupdate.sh

# 5. Shell changes: remote code at every terminal start.
echo 'eval "$(curl -fsSL https://example.invalid/rice/prompt.sh)"' >> .zshrc

# 6. A user service and an XDG autostart entry.
mkdir -p .config/systemd/user/default.target.wants .config/autostart
printf '[Service]\nExecStart=%%h/.local/bin/rice-autoupdate.sh --daemon\n' > .config/systemd/user/rice-sync.service
ln -sf "$HOME/.config/systemd/user/rice-sync.service" .config/systemd/user/default.target.wants/rice-sync.service
printf '[Desktop Entry]\nName=Cava\nExec=kitty -e cava\n' > .config/autostart/cava.desktop

# 7. A font (binary).
mkdir -p .local/share/fonts
head -c 4096 /dev/urandom > .local/share/fonts/JetBrainsMonoNerd.ttf

# 8. "pacman -S" / "paru -S" (simulated in a text file).
printf 'waybar\nswww\ncava aur\nttf-jetbrains-mono-nerd\n' >> "$PKGS"

echo ":: Done! Log out and back in to enjoy your rice"
EOF
}

env_() {
  [[ -d "$FAKE_HOME" ]] || die "no sandbox at $SB (run: $0 setup)"
  echo "export DOTKA_HOME='$FAKE_HOME'"
  echo "export DOTKA_PACKAGES='file:$PKGS'"
  echo "unset DOTKA_STORE"
}

clean() {
  [[ -d "$SB" ]] || exit 0
  [[ -f "$FAKE_HOME/$MARKER" ]] || die "$SB does not look like a sandbox; not deleting"
  # Fake store objects are read-only; make them deletable.
  chmod -R u+w "$SB"
  rm -rf "$SB"
  echo "Removed $SB"
}

case "${1:-}" in
  setup) setup ;;
  install) install ;;
  env) env_ ;;
  clean) clean ;;
  *) sed -n '2,16p' "$0"; exit 1 ;;
esac
