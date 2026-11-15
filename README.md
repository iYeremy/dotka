# dotka

**A time machine for your Linux user environment.**
Dotka tells you exactly what that `install.sh` changed — files, packages, user services and
everything that now runs when you log in — and lets you go back.

> ⚠️ Early prototype. Do not use it on a configuration you care about yet.

## Why

You find beautiful Hyprland dotfiles, clone the repo and run `./install.sh`. Now you don't know
which of your files were overwritten, which packages were installed, or what will run every time
you log in. Dotka captures **generations** of your user environment (inspired by Nix, without a
declarative language) so you can compare and restore them.

```bash
dotka init                          # create the store and generation #1
dotka snap -m "before X's dots"     # save a generation
./install.sh                        # you run the installer yourself
dotka status                        # what changed: files, packages, services, startup
dotka rollback 1                    # go back (the rollback itself is reversible)
```

## Principles

- Read-only first, plan before apply, explicit consent.
- Dotka **never** runs `sudo`, third-party scripts or package managers in write mode.
  It prints the commands for you to review.
- Every rollback first snapshots the current state: nothing is ever truly deleted.
- Honest reporting: confirmed detections vs. heuristics.

## Try it safely (sandbox)

Dotka is a prototype: try it on a **fake home**, never on your real one.
`scripts/sandbox.sh` builds a fake Hyprland (Lua config) home and a fake
third-party `install.sh` that overwrites configs, adds packages, a user service,
an autostart entry and a `curl | sh` in `.zshrc`.

```bash
go build -o dotka ./cmd/dotka
scripts/sandbox.sh setup
eval "$(scripts/sandbox.sh env)"   # points dotka at .sandbox/home
./dotka init
./dotka run -- .sandbox/super-rice/install.sh   # run the "third-party installer" through dotka
./dotka why waybar                 # who added it? -> by `.../super-rice/install.sh`
./dotka doctor                     # health check
./dotka diff --patch --path hypr
./dotka rollback 1                 # preview → confirm → restore
./dotka rollback 2                 # undo the rollback
scripts/sandbox.sh clean
```

`dotka startup` only reads files, so it is safe to run on your real home:
it prints everything that runs at login or shell start.

While dotka is a prototype, write commands refuse to run on your real home
unless you set `DOTKA_ALLOW_REAL_HOME=1`.

## Interactive interface (TUI)

Run `dotka` with no arguments in a terminal (or `dotka tui`). Five tabs answer the
questions you have after trying someone's dotfiles: **Overview** (how am I?),
**Changes** (what changed?), **History** (my save points), **Startup** (what runs by
itself?) and **Health** (is anything broken?). The detail pane explains each item in
plain words, and every action asks for confirmation first.

| Key | Action |
|---|---|
| `1`-`5`, left/right | switch tab |
| `j`/`k`, up/down | move (or scroll the detail) |
| `tab`, `enter` / `esc` | list <-> detail / back |
| `s` | save a point |
| `u` | discard unsaved changes |
| `b` / `x` | History: go back to / forget the selected save point |
| `i` | set dotka up (first time) |
| `r`, `?`, `q` | refresh, help, quit (`ctrl+c` always quits) |

Terminal contract: requires a TTY of at least **70x20**. The output is ASCII-only and
uses the 16 ANSI colors, so it works on the Linux console, inside tmux and over SSH;
it honors `NO_COLOR` (meaning is never carried by color alone). No mouse support yet.
On your real home it opens **read-only** while dotka is a prototype.
Design and real screenshots: [docs/TUI.md](docs/TUI.md).

## Commands

| Command | What it does |
|---|---|
| *(no command)*, `tui` | open the interactive interface |
| `init` | create the store and generation #1 |
| `snap -m MSG` | save a generation |
| `run [-m MSG] -- CMD...` | snapshot, run CMD, snapshot, show what CMD changed (attributed to CMD) |
| `why THING` | where a file, package or startup item came from |
| `status` | changes since the current generation |
| `diff [A [B]] [--path P] [--patch]` | compare generations or a generation with now |
| `rollback N [--dry-run] [--yes] [--keep-new]` | restore generation N (reversible) |
| `log`, `show N` | list / inspect generations |
| `startup` | startup map: Hyprland (Lua and conf), XDG autostart, systemd --user, shell rc |
| `doctor` | read-only health check: missing startup programs, startup scripts that download code or use sudo, broken symlinks, exposed secrets, duplicates, leftover configs |
| `forget N`, `prune --keep N` | delete old generations and free their space |
| `track ls/add/rm PATH` | manage tracked roots |
| `secrets ls/allow/deny PATH` | files excluded because they look like credentials |
| `gc` | delete unreferenced objects |

## Development

```bash
go test ./...
```

Branches: `feat/…`, `fix/…`, `chore/…`, `docs/…`, merged into `main` with `--no-ff`.
Commits: `type: short description` (`feat`, `fix`, `chore`, `docs`, `test`, `refactor`).

Docs (Spanish): [user guide and testing walkthrough](docs/GUIA.md) · [project analysis](docs/ANALISIS.md)

## License

MIT
