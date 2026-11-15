# dotka user guide

This guide is for when you've read the README blurb and want to actually get it: what the
words on screen mean, what each command is for, and how to try everything without any
risk to your real config.

## 1. The idea in 30 seconds

Customizing Linux means touching a ton of files all over your home. When you run someone
else's `install.sh`, it can overwrite your files, install packages and add stuff that
runs every time you log in. Usually it won't tell you any of that.

dotka is basically **save points from a video game, but for your setup**. Save before you
experiment. If it goes sideways, dotka shows you exactly what changed and takes you back.

```
save  ->  try someone's dotfiles  ->  dotka shows what changed  ->  like it: save again
                                                                ->  hate it: go back
```

## 2. Words you'll see

| Word | What it means |
|---|---|
| **home** | Your personal folder (`/home/you`), where your configs live. dotka only ever works inside it. |
| **save point** (aka generation) | A full snapshot of your setup at one moment. It gets a number (`#1`, `#2`...), a date and an optional message, and it never changes after it's made. |
| **current save point** | The one you're "on", marked with `*` in `dotka log`. `status` compares your setup against it. |
| **automatic save point** | Before going back, dotka quietly saves where you are now (origin `auto: before rollback`). That's why a rollback can always be undone. |
| **store** | Where dotka keeps copies of your files: `~/.local/share/dotka`. Only you can read it. A file that doesn't change between save points is stored once, so it doesn't eat space twice. |
| **tracked** | The folders and files dotka watches. By default `~/.config`, `~/.local/bin`, fonts, themes, icons and your shell files. You can add more. |
| **excluded** | Stuff dotka ignores on purpose because it's junk or huge: caches, browser profiles, Steam, the trash, Electron cache folders. |
| **secret** | A file that looks like it holds passwords or tokens (e.g. `~/.config/gh/hosts.yml`). dotka doesn't copy it unless you allow it, which also means it can't restore it. |
| **startup map** | Everything that runs by itself: what your window manager launches at login, systemd user services, desktop autostart entries and some lines in your shell config. This is the most important thing to check after installing someone's dotfiles. |
| **packages** | Stuff installed with pacman or an AUR helper. dotka only *reads* them. It never installs or removes anything and never uses `sudo`. It just tells you the command. |
| **rollback** | Taking your tracked files back to a save point: restore what changed, bring back what was deleted, remove what was added since. |
| **provenance** | Knowing *which command* caused a change. It works when you install through `dotka run`, because that save point gets tied to the command. |
| **heuristic** | A guess based on patterns, not a sure thing. Marked `[heuristic]` so you know it might be wrong. |

## 3. Commands, by what you're trying to do

**Set up and save**

| Command | Use it to |
|---|---|
| `dotka init` | Set up dotka. Saves your current setup as save point `#1`. On your real home it explains exactly what it'll do and asks for a yes first. |
| `dotka snap -m "msg"` | Save a point, e.g. `snap -m "before trying X's dots"`. |
| `dotka run -- ./install.sh` | The recommended way to try someone's dotfiles. Saves (if you have unsaved changes), runs the command, saves again tied to that command, and shows you what it changed. Heads up: the command runs for real, it's not sandboxed. |

**Look around** (none of these change anything)

| Command | Use it to |
|---|---|
| `dotka status` | "What changed since my last save point?" Files, packages and startup. |
| `dotka diff` | More detail. No numbers compares the current save point with now; `diff 1 3` compares two save points; `--patch` shows changed lines; `--path hypr` filters by folder. |
| `dotka why waybar` | "Where did this come from?" Looks up a file, package or startup item in your history and tells you what caused it. |
| `dotka log` / `dotka show 3` | List save points / summary of one. |
| `dotka startup` | Everything that runs by itself, grouped by where it comes from. Also tells you which window managers it found. |
| `dotka doctor` | Health check: programs launched at login that aren't installed, startup scripts that download and run code or use `sudo`, broken links, secrets other users can read, things started twice, leftover configs from uninstalled apps. Works even without `init`. |

**Go back and clean up**

| Command | Use it to |
|---|---|
| `dotka rollback 1` | Go back to save point `#1`. Shows the plan, asks first. `--dry-run` only shows the plan, `--keep-new` keeps files that are newer than the save point, `-y` skips the question. |
| `dotka forget 3` | Delete a save point you don't need anymore. Frees the space of copies that *only* it used. Your files aren't touched; you just can't go back there. You can't forget the current one. |
| `dotka prune --keep 10` | Keep the 10 newest save points (plus the current one), forget the rest. |
| `dotka gc` | Maintenance: deletes stored copies nothing uses (like leftovers from an interrupted `snap`). You rarely need it since `forget`/`prune` already free space. |

**Configure**

| Command | Use it to |
|---|---|
| `dotka track` | See what's tracked. `track add .local/share/nvim` adds a folder, `track rm` removes it. |
| `dotka secrets` | See which files were skipped as secrets. `secrets allow PATH` lets dotka store one (the store isn't encrypted, so think twice). |

### How space works (and why `forget` sometimes frees so little)

Each save point is a **list** like "my `hyprland.lua` is copy A, my `.zshrc` is copy B...".
Copies are stored once, no matter how many lists point at them:

```
#1  hyprland.lua -> copy A    .zshrc -> copy B
#2  hyprland.lua -> copy C    .zshrc -> copy B   (B is shared, not duplicated)
#3  hyprland.lua -> copy C    .zshrc -> copy D
```

`dotka forget 1` deletes copy **A** because nobody uses it anymore. Copy **B** stays since
`#2` still needs it. So forgetting a save point often frees little: most stuff is shared.
`dotka log` shows how much the store uses at the bottom.

## 4. Reading the output

```
Files     +8  ~3  -0                 8 added, 3 modified, 0 removed
  + .config/waybar/ (3 files)        new folder with 3 files
  ~ .config/hypr/hyprland.lua        modified
  ~ .config/kitty/kitty.conf (file -> symlink: ...) [!] your file was replaced by a symlink
Packages  +4  -0                     installed / uninstalled
Startup   +6  -2                     new things that will run / things that stopped running
  + hypr-autostart waybar  .config/hypr/hyprland.lua:3
                           kind  command  file:line it comes from
```

| Marker | Meaning |
|---|---|
| `+` / `~` / `-` | added / modified / removed |
| `*` (in `log`) | current save point |
| `[!]` | probably worth a look |
| `[risk: downloads and runs code]` | grabs code from the internet and runs it (`curl ... \| sh`). The most serious one. |
| `[risk: runs a local script]` | runs a script from your home. Worth reading it. |
| `[heuristic]` | pattern-based guess, might be wrong |
| `[secret]` | looks like a password/token file, not stored |
| `[nix-managed]` | link into `/nix/store`, owned by home-manager. Never touched by rollback. |
| `warning:` | heads-up that doesn't stop the command |

All output is plain ASCII, so it looks fine on a bare TTY too.

## 5. Try everything without risk (walkthrough)

### 5.0 Set up the sandbox

```bash
cd dotka
go build -o dotka ./cmd/dotka
scripts/sandbox.sh setup
eval "$(scripts/sandbox.sh env)"
echo $DOTKA_HOME          # should end in .sandbox/home
```

The `eval` points dotka at the fake home, and it only lasts in **that** terminal. In a new
terminal, run it again. If you forget, nothing bad happens without you noticing: on your
real home, `init` stops and asks before doing anything.

Careful with `~` in arguments: your shell expands it to your *real* home before dotka sees
it. Use relative paths (`.config/x`) or `"$DOTKA_HOME/.config/x"`.

### 5.1 Look around first

```bash
./dotka startup      # what runs at login: quickshell, dunst, nm-applet
./dotka doctor       # a few findings already: the fake token is readable by others,
                     # and maybe programs the fake config starts that you don't have
./dotka init         # save point #1
./dotka log
./dotka secrets      # .config/gh/hosts.yml detected and NOT stored
```

### 5.2 Install someone's dotfiles (the fake ones)

The sandbox comes with a fake dotfiles repo in `.sandbox/super-rice/`. Install it through
dotka:

```bash
./dotka run -- .sandbox/super-rice/install.sh
```

You should see: `hyprland.lua` modified, a new `waybar/`, your `kitty.conf` replaced by a
symlink, 4 new packages, and in startup `waybar`, `swww-daemon`, a user service, an
autostart entry and a `curl` in `.zshrc` flagged `[risk: downloads and runs code]`. You'll
also see `-2` in Startup: the installer quietly **removed** quickshell and nm-applet from
your autostart. Rude.

```bash
./dotka why waybar             # package, config and startup entry: by `.../install.sh`
./dotka why kitty/kitty.conf   # was there at the start; the installer made it a symlink
./dotka doctor                 # now flags the autoupdate script that does curl | sh
./dotka diff --patch --path hypr
```

### 5.3 Go back

```bash
./dotka rollback 1 --dry-run   # just the plan
./dotka rollback 1             # answer n: nothing changes
./dotka rollback 1             # answer y
./dotka status                 # only the packages remain
```

The packages sticking around is **expected**: dotka never touches them, it shows you the
`sudo pacman -Rns ...` command to run yourself. Run `./dotka why waybar` again and the
package shows as `still installed`, "kept after going back".

### 5.4 Undo the undo

```bash
./dotka log          # there's a new "auto: before rollback" save point
./dotka rollback 2   # back to the riced state
```

### 5.5 Free play

| Try | How | Expected |
|---|---|---|
| Manual edit | `echo 'hl.exec_cmd("discord")' >> $DOTKA_HOME/.config/hypr/hyprland.lua`, then `./dotka status` | modified file, `discord` new in startup, marked `[heuristic]` (it's outside the startup block) |
| Keep new files | `echo x > $DOTKA_HOME/.config/mine.conf`, then `./dotka rollback 1 --keep-new` | modified stuff restored, `mine.conf` stays |
| A secret | `printf 'password = hunter22\n' > $DOTKA_HOME/.config/mail.conf`, then `status` | marked `[secret]`; a rollback won't delete it (no copy to bring it back) |
| Huge file | `head -c 11M /dev/urandom > $DOTKA_HOME/.config/big.bin`, then `snap` | size warning; a rollback won't delete it |
| Track more | `./dotka track add .local/share/nvim`, then `snap` | included from now on; going back to an older save point won't wipe it |
| Fake pacman | edit `.sandbox/packages.txt` (add ` aur` for AUR ones), then `status` | packages added/removed |
| Forget | `./dotka forget 2 --dry-run`, `./dotka prune --keep 2` | shows what it frees; never lets you forget the current one |
| The UI | `./dotka` | all of the above, interactively |

### 5.6 Start over

```bash
scripts/sandbox.sh clean && scripts/sandbox.sh setup && eval "$(scripts/sandbox.sh env)"
```

## 6. Using it on your real setup

```bash
dotka doctor        # read-only, a good first look
dotka init          # explains what happens, asks for a yes
dotka run -- ./install.sh
```

From there it's the same as the sandbox. Two habits that help: `snap` before you mess
with stuff, and install other people's dotfiles with `dotka run` so `why` can tell you
later who did what.

## 7. What dotka won't do (on purpose)

- **Install or remove packages**, or use `sudo`. It gives you the commands.
- **Delete what it couldn't bring back**: secrets and huge files that weren't stored stay put.
- **Touch what it doesn't track.** If an installer creates `~/dots/` and that's not
  tracked, dotka doesn't see it or delete it.
- **Fight home-manager.** Links into `/nix/store` are shown but never changed.
- **Pretend to be an antivirus.** The startup map and `doctor` read text and flag
  dangerous patterns, but they can't promise something is safe.
- **Run the scripts it analyzes.** It only reads them.
