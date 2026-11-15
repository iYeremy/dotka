# How dotka works

Diagrams render on GitHub and GitLab (Mermaid). If you're reading this in a terminal, the
ASCII bits still make sense on their own.

## The big picture

dotka sits next to your config and keeps save points of it. It reads a lot and writes
very little, and it only writes when you ask.

```mermaid
flowchart TB
    subgraph home["your home (~)"]
        cfg["~/.config, ~/.local/bin,<br/>fonts, themes, .zshrc..."]
    end
    subgraph system["system (read-only for dotka)"]
        pac["pacman -Qqe / -Qqm"]
    end
    subgraph store["~/.local/share/dotka (private store)"]
        obj["objects/<br/>file contents, by hash"]
        gens["generations/<br/>one manifest per save point"]
        log["log.jsonl<br/>what dotka did"]
    end

    cfg -- "scan (never follows symlinks,<br/>skips caches and secrets)" --> state
    pac -- "package list" --> state
    cfg -- "startup map: WM configs,<br/>autostart, systemd, shell rc" --> state
    state["current state"] -- "snap" --> gens
    state -- "snap: new contents only" --> obj
    gens -- "status / diff / why" --> you["you"]
    gens -- "rollback: plan, ask,<br/>auto-save, restore" --> cfg
```

A **save point** is a manifest (a JSON list) that says "at this moment, this path had
this content, these permissions; these packages were installed; these things ran at
startup". The actual file contents live in `objects/`, named by their sha256, so the same
content is only ever stored once.

## What happens on `dotka run -- ./install.sh`

```mermaid
sequenceDiagram
    actor U as you
    participant D as dotka
    participant S as store
    participant I as ./install.sh
    participant H as your home

    U->>D: dotka run -- ./install.sh
    D->>S: lock (no two dotkas at once)
    D->>H: anything unsaved since the current save point?
    alt yes
        D->>S: save "before: ./install.sh"
    end
    D->>I: run it (HOME = the home dotka manages)
    I->>H: overwrites configs, adds stuff...
    I-->>D: exit status
    D->>H: scan files, packages, startup map
    D->>S: save point tied to the command (run info + parent)
    D-->>U: what the command changed + how to undo it
```

Two details that matter:

- If dotka manages a home that isn't your real one (like the sandbox), the command gets
  that home as `HOME` and the `XDG_*` dirs, so it can't write into your real config by
  accident.
- The save point remembers its **parent** (the save point you were on) and the command.
  That's what lets `dotka why` say "added by `./install.sh`" later, and not get confused
  by rollbacks.

## What happens on `dotka rollback N`

```mermaid
sequenceDiagram
    actor U as you
    participant D as dotka
    participant S as store
    participant H as your home

    U->>D: dotka rollback 1
    D->>H: scan current state (read-only)
    D->>D: build the plan: restore, remove, recreate,<br/>skip what can't come back
    D-->>U: show plan + package commands (never run)
    U->>D: y
    D->>S: auto-save current state (so this can be undone)
    loop for each action, in a safe order
        D->>H: remove files/links (only if unchanged since the plan)
        D->>H: remove now-empty folders
        D->>H: create folders, write files atomically, fix permissions
    end
    D->>S: current save point = 1, log it
    D-->>U: done, plus how to undo it (rollback to the auto-save)
```

Rules the planner never breaks:

```
+----------------------------------------------------------------------------+
| never deletes something it can't bring back (secrets, huge files, files     |
|   it didn't store)                                                          |
| never removes a folder that still has untracked or kept stuff inside        |
| never touches links into /nix/store (home-manager owns them)                |
| never writes outside your home, never follows a symlink while writing       |
| never touches paths that weren't tracked in BOTH the target and now         |
| writes files via temp file + fsync + rename, so nothing is left half-done   |
+----------------------------------------------------------------------------+
```

## The startup map

```mermaid
flowchart LR
    subgraph sources["what dotka reads"]
        h["hyprland.lua / .conf"]
        w["sway, i3, niri, river,<br/>Wayfire, bspwm, Openbox..."]
        x["~/.config/autostart/*.desktop"]
        sd["~/.config/systemd/user/*.wants"]
        sh[".zshrc, .bashrc, .profile..."]
    end
    sources --> p["parsers<br/>(read only, nothing executed)"]
    p --> items["startup items:<br/>kind, command, file:line,<br/>confirmed or heuristic"]
    items --> r["risk check:<br/>curl | sh, local scripts"]
    r --> out["dotka startup / status / UI"]
    items --> doc["dotka doctor:<br/>is the program installed?<br/>what does that script do?"]
```

Each item is identified by kind + file + command (not line number), so moving a line
around doesn't show up as a change, but adding or removing one does.

## The store on disk

```
~/.local/share/dotka/        (0700, only you)
  objects/ab/cdef0123...     file contents, named by sha256 (read-only)
  generations/000004.json    one manifest per save point (never modified)
  HEAD                       the current save point
  LAST_ID                    highest number ever used (numbers are never reused)
  log.jsonl                  every snap, rollback, forget...
  config.json                tracked roots, exclusions, size limit, allowed secrets
  lock                       held while dotka writes
```

## Code layout

```mermaid
flowchart TB
    cli["cmd/dotka<br/>commands + output"] --> gen
    tui["internal/tui<br/>Bubble Tea UI"] --> gen
    cli --> doctor
    tui --> doctor
    gen["internal/gen<br/>save points, diff, rollback,<br/>forget, history"] --> scan
    gen --> store["internal/store<br/>objects, manifests, lock, log"]
    gen --> collect["internal/collect<br/>packages + startup"]
    collect --> pkgs["internal/pkgs<br/>pacman (read-only)"]
    collect --> startup["internal/startup<br/>WM / autostart parsers"]
    doctor["internal/doctor<br/>health checks"] --> startup
    doctor --> scan
    scan["internal/scan<br/>walk tracked roots"] --> secrets["internal/secrets<br/>credential detection"]
    scan --> config["internal/config<br/>what's tracked"]
    gen --> fsutil["internal/fsutil<br/>safe paths, atomic writes"]
```

The UI never does I/O while drawing: it goes `Backend -> commands -> messages -> Update
-> View`, so it can be tested without a terminal (see [tui.md](tui.md)).
