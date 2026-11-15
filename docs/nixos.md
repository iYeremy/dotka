# dotka on NixOS / home-manager

tl;dr: it works, as a **viewer** plus a safety net for the imperative stuff. It won't
fight home-manager.

## Why it doesn't clash with Nix

With home-manager your dotfiles are **symlinks into `/nix/store`**, and home-manager owns
them. dotka is built to be polite about that:

- It records symlinks **as symlinks** and never follows them, so a `home-manager switch`
  shows up as "these links now point somewhere else".
- `rollback` **never touches links into `/nix/store`**. Restoring them would point at
  store paths that may already be garbage-collected, and it would step on home-manager's
  toes. They show up as `[nix-managed]` and the plan says "use home-manager to change it".
- Everything you manage **by hand** (files that aren't home-manager links) gets the
  normal treatment: save points, diffs, rollback.
- `doctor` and `startup` are read-only, so they just work.

So the split is: home-manager handles what it declares, dotka watches everything and
covers the stuff you change imperatively.

## What works and what doesn't

| Feature | On NixOS |
|---|---|
| `status`, `diff`, `log`, `why` | works. HM links show as links with their store targets |
| `dotka run -- home-manager switch` | works, and it's kinda nice: shows exactly what a switch changed in your home |
| `rollback` | works for imperative files; skips `/nix/store` links on purpose |
| `startup` | works. HM-generated configs (e.g. a `hyprland.conf` link) are read through the link |
| `doctor` | works. Programs are found through your normal `$PATH` (`/run/current-system/sw/bin`, `~/.nix-profile/bin`...) |
| packages section | not shown (pacman only for now). Nix profiles could come later |
| systemd user units from HM | listed by name. Their `ExecStart` lives in the store, so it isn't shown |
| `nixos-rebuild` changes | out of scope: dotka only tracks your home, not `/etc` or the system |

## Build it

No package yet, so build from source:

```bash
git clone https://github.com/iYeremy/dotka
cd dotka
nix shell nixpkgs#go -c go build -o dotka ./cmd/dotka
./dotka --help
```

## Test plan (thanks for testing!)

Start in the sandbox, then your real home if you're feeling brave.

```bash
# 1. sandbox (no risk at all)
nix shell nixpkgs#go -c go test ./...
scripts/sandbox.sh setup && eval "$(scripts/sandbox.sh env)"
./dotka init && ./dotka run -- .sandbox/super-rice/install.sh
./dotka                        # click around the UI
./dotka rollback 1
scripts/sandbox.sh clean

# 2. your real home: read-only stuff first
unset DOTKA_HOME DOTKA_PACKAGES
./dotka doctor
./dotka startup

# 3. real home, for real
./dotka init                   # explains what it does, asks for a yes
./dotka run -- home-manager switch
./dotka status
./dotka rollback 1 --dry-run   # check that HM links show as "managed by Nix"
```

Stuff that'd be super useful to hear back about:

- Did `go test ./...` pass?
- Anything that showed up wrong or noisy in `doctor` (false positives, like "missing
  program" for something you do have)?
- After `home-manager switch`, did `status` make sense?
- In `rollback --dry-run`, were all the HM links listed as skipped (managed by Nix)?
- Your WM: is its autostart listed in `dotka startup`? If it's Qtile/awesome/xmonad you
  should get a "not supported yet" note instead.
- Anything that looked like it'd touch something it shouldn't. That's the one that
  really matters.
