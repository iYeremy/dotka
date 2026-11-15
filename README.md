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
scripts/sandbox.sh install         # the "third-party installer"
./dotka status                     # files, packages, services, startup
./dotka diff --patch --path hypr
./dotka rollback 1                 # preview → confirm → restore
./dotka rollback 2                 # undo the rollback
scripts/sandbox.sh clean
```

`dotka startup` only reads files, so it is safe to run on your real home:
it prints everything that runs at login or shell start.

While dotka is a prototype, write commands refuse to run on your real home
unless you set `DOTKA_ALLOW_REAL_HOME=1`.

## Commands

| Command | What it does |
|---|---|
| `init` | create the store and generation #1 |
| `snap -m MSG` | save a generation |
| `status` | changes since the current generation |
| `diff [A [B]] [--path P] [--patch]` | compare generations or a generation with now |
| `rollback N [--dry-run] [--yes] [--keep-new]` | restore generation N (reversible) |
| `log`, `show N` | list / inspect generations |
| `startup` | startup map: Hyprland (Lua and conf), XDG autostart, systemd --user, shell rc |
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
