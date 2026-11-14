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

## Development

```bash
go test ./...
go build ./cmd/dotka
# Always try it against a fake home, never your real one:
./dotka --home /tmp/fakehome --store /tmp/fakestore init
```

Project analysis (Spanish): [docs/ANALISIS.md](docs/ANALISIS.md)

## License

MIT
