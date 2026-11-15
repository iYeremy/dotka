# Guía de Dotka para usuarios

Esta guía es para quien solo leyó la descripción del proyecto y quiere entender qué hace Dotka, qué significa cada palabra que aparece en pantalla y cómo probarlo sin riesgo.

---

## 1. ¿Qué es Dotka, en dos minutos?

Cuando personalizas Linux (Hyprland, una barra, un tema, tu terminal) cambias muchos archivos de configuración repartidos por tu carpeta personal. Si un día ejecutas el `install.sh` de los dotfiles de otra persona, ese script puede:

- sobrescribir tus archivos,
- instalar paquetes,
- añadir programas que se ejecutan solos cada vez que inicias sesión,
- y no decirte nada de esto.

**Dotka es como los puntos de guardado de un videojuego, pero para tu configuración.** Guardas una partida antes de experimentar. Si algo sale mal, Dotka te dice exactamente qué cambió y te deja volver a ese punto.

```
Guardas partida  ->  pruebas los dotfiles de alguien  ->  Dotka te dice qué cambió
                                                     ->  te gusta: guardas otra partida
                                                     ->  no te gusta: vuelves atrás
```

---

## 2. Conceptos en lenguaje simple

| Palabra que verás | Qué significa |
|---|---|
| **Home** | Tu carpeta personal (`/home/tu-usuario`). Ahí viven tus configuraciones: `~/.config`, `~/.zshrc`, etc. Dotka **solo** trabaja dentro de ella. |
| **Generación** | Un **punto de guardado**: una foto completa de tu configuración en un momento dado. Tiene un número (`#1`, `#2`…), una fecha y un mensaje opcional. Nunca se modifica después de creada. |
| **Generación actual** | La partida en la que estás. Aparece marcada con `*` en `dotka log`. `status` compara tu configuración de ahora con ella. |
| **Generación automática** | Antes de volver atrás, Dotka guarda **solo** un punto con tu estado actual (origen `auto: before rollback`). Por eso volver atrás también se puede deshacer. |
| **Store (almacén)** | La carpeta donde Dotka guarda las copias de tus archivos y la lista de generaciones: `~/.local/share/dotka`. Es privada (solo tu usuario puede leerla). Si un archivo no cambia entre generaciones, se guarda una sola vez, así que no ocupa el doble. |
| **Rastrear (track)** | Las carpetas y archivos que Dotka vigila. Por defecto: `~/.config`, `~/.local/bin`, fuentes, temas, iconos y los archivos de shell (`.zshrc`, `.bashrc`…). Puedes añadir más. |
| **Exclusiones** | Cosas que Dotka ignora a propósito porque son basura o enormes: cachés, perfiles del navegador, Steam, la papelera. |
| **Secretos** | Archivos que parecen tener contraseñas o tokens (por ejemplo, `~/.config/gh/hosts.yml`). Dotka **no los copia** salvo que se lo permitas, así que tampoco puede restaurarlos. |
| **Mapa de arranque (startup)** | La lista de todo lo que se ejecuta solo: lo que lanza Hyprland al iniciar, los servicios de usuario de systemd, el autostart del escritorio y ciertas líneas de tu `.zshrc`. Es lo más importante de vigilar después de instalar dotfiles ajenos. |
| **Paquetes** | Los programas instalados con pacman o un helper del AUR. Dotka **solo los lee**: nunca instala ni desinstala nada, y nunca usa `sudo`. Te sugiere los comandos para que los ejecutes tú. |
| **Volver atrás (rollback)** | Devolver tus archivos rastreados al estado de una generación: restaura lo modificado, recrea lo borrado y quita lo que se añadió después. |
| **Procedencia** | Saber qué comando introdujo cada cambio. Funciona cuando instalas con `dotka run`: esa partida queda asociada al comando. Es algo que git no puede hacer. |
| **Padre** | La partida de la que partió otra. Si vuelves a `#2` y guardas, la nueva partida tiene como padre a `#2`. Así `why` no confunde un rollback con un borrado. |
| **Heurística** | Una detección "por patrón" en vez de segura. Por ejemplo, adivinar que una línea de `.zshrc` ejecuta algo. Dotka la marca como `[heuristic]` para que sepas que puede equivocarse. |
| **Sandbox** | Un home de mentira (`.sandbox/home`) para probar Dotka sin tocar tu configuración real. |

---

## 3. Qué hace cada comando

| Comando | Para qué lo usarías |
|---|---|
| `dotka init` | La primera vez: crea el almacén y guarda el punto `#1` con tu estado actual. |
| `dotka snap -m "mensaje"` | Guardar partida. Por ejemplo: `snap -m "antes de probar los dots de X"`. |
| `dotka run -- ./install.sh` | La forma recomendada de probar dotfiles ajenos: guarda partida (si hay cambios sin guardar), **ejecuta el comando** que le pases, guarda otra partida asociada a ese comando y te muestra qué cambió. El comando se ejecuta de verdad, no está aislado. Si Dotka apunta a un home que no es el tuyo (el sandbox), el comando también ve ese home como su `$HOME`. |
| `dotka why waybar` | "¿De dónde salió esto?". Busca un archivo, paquete o elemento de arranque en la historia y dice quién lo introdujo: un comando lanzado con `dotka run`, un cambio hecho fuera de Dotka, o que ya estaba al empezar. |
| `dotka status` | "¿Qué cambió desde mi último punto de guardado?": archivos, paquetes y lo que se ejecuta al iniciar. |
| `dotka diff` | Más detalle. Sin números compara el último punto con ahora. `diff 1 3` compara dos puntos. `--patch` muestra las líneas cambiadas. `--path hypr` filtra por carpeta. |
| `dotka log` | Lista de todos tus puntos de guardado. |
| `dotka show 3` | Resumen de un punto: cuántos archivos, paquetes y elementos de arranque tiene. |
| `dotka rollback 1` | Volver al punto `#1`. Primero muestra el plan y pide confirmación. `--dry-run` solo muestra el plan, `--keep-new` conserva los archivos nuevos y `-y` no pregunta. |
| `dotka startup` | Ver todo lo que se ejecuta solo al iniciar sesión o abrir una terminal. **Solo lee**: es seguro usarlo en tu configuración real. |
| `dotka doctor` | Chequeo de salud de tu configuración. **Solo lee** y funciona sin `init`. Busca: programas que se lanzan al iniciar pero no están instalados, scripts de arranque que descargan y ejecutan código o usan `sudo`, symlinks rotos, archivos secretos que otros usuarios pueden leer, programas lanzados dos veces y configuraciones que quedaron de programas desinstalados. |
| `dotka track` | Ver qué se rastrea. `track add .local/share/nvim` añade una carpeta y `track rm` la quita. |
| `dotka secrets` | Ver qué archivos se excluyeron por parecer secretos. `secrets allow RUTA` permite guardar uno. |
| `dotka forget 3` | Borrar un punto de guardado que ya no necesitas. Libera el espacio de las copias que **solo** usaba ese punto. Tus archivos no se tocan; solo pierdes la posibilidad de volver a ese punto. No se puede borrar el punto actual. |
| `dotka prune --keep 10` | Conservar solo los 10 puntos más recientes (más el actual) y olvidar el resto. |
| `dotka gc` | Mantenimiento: borra copias que no usa ningún punto (por ejemplo, restos de un `snap` interrumpido). Normalmente no hace falta, porque `forget` y `prune` ya liberan el espacio. |

---

### ¿Cómo se ocupa y se libera el espacio?

Cada punto de guardado es una **lista** que dice "mi `hyprland.lua` es la copia A, mi `.zshrc` es la copia B…". Las copias se guardan una sola vez aunque muchas listas las usen:

```
#1  hyprland.lua -> copia A    .zshrc -> copia B
#2  hyprland.lua -> copia C    .zshrc -> copia B   (B se comparte, no se duplica)
#3  hyprland.lua -> copia C    .zshrc -> copia D
```

Si haces `dotka forget 1`, la copia **A** ya no la usa nadie y se borra. La copia **B** se queda porque #2 la sigue usando. Por eso olvidar un punto a veces libera poco: casi todo se comparte con los demás. `dotka log` muestra al final cuánto ocupa el almacén.

### La interfaz interactiva (TUI)

Si ejecutas `dotka` sin nada más (o `dotka tui`) se abre una interfaz con cinco pestañas:

| Pestaña | Para qué |
|---|---|
| `1` Overview | Resumen: ¿hay cambios sin guardar?, ¿algún problema?, ¿qué puedo hacer ahora? |
| `2` Changes | Cada cambio desde tu último punto de guardado, con su explicación y su diff |
| `3` History | Tus puntos de guardado. `b` vuelve a uno y `x` lo olvida |
| `4` Startup | Todo lo que se ejecuta solo, explicado en lenguaje simple |
| `5` Health | Lo mismo que `dotka doctor`, con qué significa cada problema y qué hacer |

Te mueves con `j`/`k` (o las flechas), pasas al detalle con `Tab`, guardas con `s` y la
ayuda está en `?`. Antes de cambiar cualquier cosa, siempre te pide confirmación. Sobre
tu home real se abre en **solo lectura**. Capturas reales y el diseño completo, en
[TUI.md](TUI.md).

## 4. Cómo leer lo que imprime Dotka

```
Files     +8  ~3  -0                 8 añadidos, 3 modificados, 0 borrados
  + .config/waybar/ (3 files)        carpeta nueva con 3 archivos
  ~ .config/hypr/hyprland.lua        archivo modificado
  ~ .config/kitty/kitty.conf (file -> symlink: ...) [!] your file was replaced by a symlink
  - algo                             archivo que ya no existe
Packages  +4  -0                     paquetes instalados / desinstalados
Startup   +6  -2                     cosas nuevas que se ejecutarán / que dejaron de ejecutarse
  + hypr-autostart waybar  .config/hypr/hyprland.lua:3
                                     tipo        comando  archivo:línea de donde sale
```

| Marca | Significado |
|---|---|
| `+` / `~` / `-` | Añadido / modificado / eliminado |
| `*` (en `log`) | Generación actual |
| `[!]` | Algo que probablemente quieras revisar |
| `[risk: downloads and runs code]` | Descarga código de internet y lo ejecuta (`curl ... \| sh`). Es la señal más seria. |
| `[risk: runs a local script]` | Ejecuta un script guardado en tu home. Conviene leerlo. |
| `[heuristic]` | Detección por patrón: puede equivocarse |
| `[problem]` / `[note]` (en `doctor`) | Algo que conviene arreglar / algo que conviene revisar |
| `[secret]` | Archivo con pinta de contraseña o token: no se guarda |
| `warning:` | Aviso que no detiene el comando |

La salida usa solo caracteres ASCII, para verse bien también en una TTY (la consola sin entorno gráfico).

---

## 5. Probarlo sin riesgo (paso a paso)

### 5.0 Preparar el sandbox

```bash
cd ~/proyectos/Dotka
go build -o dotka ./cmd/dotka
scripts/sandbox.sh setup
eval "$(scripts/sandbox.sh env)"
echo $DOTKA_HOME          # debe terminar en .sandbox/home
```

El `eval` le dice a Dotka que trabaje sobre el home de mentira, y solo dura en **esa** terminal. Si lo olvidas no pasa nada grave: mientras Dotka sea un prototipo, los comandos que escriben se niegan a tocar tu home real:

```
dotka: refusing to run "rollback" on your real home (/home/tu-usuario).
```

Cuidado con `~` en los argumentos: la shell lo convierte en tu home real. Usa rutas relativas (`.config/x`) o `"$DOTKA_HOME/.config/x"`.

### 5.1 Conocer el sistema antes de tocarlo

```bash
./dotka startup      # qué se ejecuta al iniciar: quickshell, dunst, nm-applet
./dotka init         # guarda el punto #1
./dotka log          # un solo punto, marcado con *
./dotka show 1
./dotka secrets      # .config/gh/hosts.yml detectado y NO guardado
./dotka track        # qué vigila Dotka
```

### 5.1b Chequeo de salud

```bash
./dotka doctor       # antes del instalador: casi limpio
```

Después del paso 5.2 vuelve a ejecutarlo. Debería detectar el script `rice-autoupdate.sh`, que descarga código en cada inicio de sesión, los programas que el instalador lanza pero no están instalados y el `hosts.yml` con permisos abiertos. `doctor` también se puede usar sobre tu configuración real, porque no modifica nada.

### 5.2 Simular que instalas los dotfiles de otra persona

El sandbox incluye un repo de dotfiles falso en `.sandbox/super-rice/`. Instálalo a través de Dotka:

```bash
./dotka run -- .sandbox/super-rice/install.sh
```

Al terminar, Dotka muestra qué cambió ese comando. (Alternativa sin procedencia: `scripts/sandbox.sh install` y después `./dotka status`).

Esperado: `hyprland.lua` modificado, `waybar/` nuevo, tu `kitty.conf` reemplazado por un symlink, 4 paquetes nuevos y en el arranque `waybar`, `swww-daemon`, un servicio, un autostart y un `curl` remoto en `.zshrc`, marcado como `[risk: downloads and runs code]`. También verás `-2` en Startup: el instalador **quitó** quickshell y nm-applet de tu arranque sin avisar.

```bash
./dotka diff --patch --path hypr
./dotka diff --patch --path .zshrc
```

### 5.2b Preguntar de dónde salió algo

```bash
./dotka why waybar               # el paquete, su config y la entrada de arranque: by `.../install.sh`
./dotka why kitty/kitty.conf     # estaba al inicio; el instalador lo convirtió en symlink
./dotka why .zshrc
```

### 5.3 Volver atrás

```bash
./dotka rollback 1 --dry-run   # solo el plan
./dotka rollback 1             # responde n: no cambia nada
./dotka rollback 1             # responde y
```

Comprueba que volvió:

```bash
./dotka status                              # solo quedan los paquetes
ls -la $DOTKA_HOME/.config/kitty/           # kitty.conf vuelve a ser un archivo normal
cat $DOTKA_HOME/.config/hypr/hyprland.lua   # vuelven quickshell y nm-applet
```

Que los paquetes sigan apareciendo es **lo esperado**: Dotka nunca los toca, solo te muestra el comando `sudo pacman -Rns ...` para que lo ejecutes tú.

Vuelve a ejecutar `./dotka why waybar` después del rollback. El paquete aparece como `still installed`, "kept after going back", porque Dotka nunca desinstala paquetes. La línea `now:` dice si cada cosa existe en la partida actual.

### 5.4 Deshacer la vuelta atrás

```bash
./dotka log          # aparece #2 "auto: before rollback"
./dotka rollback 2   # vuelve al estado con los dotfiles instalados
./dotka status       # Nothing changed
```

### 5.5 Pruebas libres

| Qué probar | Cómo | Qué debería pasar |
|---|---|---|
| Edición manual | `echo 'hl.exec_cmd("discord")' >> $DOTKA_HOME/.config/hypr/hyprland.lua` y luego `./dotka status` | Archivo modificado y `discord` nuevo en el arranque, marcado `[heuristic]` porque está fuera del bloque de inicio |
| Guardar y comparar | `./dotka snap -m "con discord"`, luego `./dotka log` y `./dotka diff 1 3` | Punto nuevo y diferencias entre dos puntos |
| Conservar lo nuevo | `echo x > $DOTKA_HOME/.config/mio.conf`, luego `./dotka rollback 1 --keep-new` | Se restaura lo modificado y `mio.conf` se queda |
| Secreto | `printf 'password = hunter22\n' > $DOTKA_HOME/.config/mail.conf`, luego `status` y `secrets` | Marcado `[secret]`. Al volver atrás **no se borra**, porque no hay copia para recuperarlo |
| Permitir un secreto | `./dotka secrets allow .config/mail.conf` | Se guarda desde el próximo `snap`, con aviso de que el almacén no está cifrado |
| Archivo grande | `head -c 11M /dev/urandom > $DOTKA_HOME/.config/big.bin` y luego `snap` | Aviso de tamaño. Al volver atrás no se borra |
| Rastrear otra carpeta | `mkdir -p $DOTKA_HOME/.local/share/nvim && echo x > $DOTKA_HOME/.local/share/nvim/a.lua`, luego `./dotka track add .local/share/nvim` y `snap` | Se incluye desde ese momento. Volver a un punto antiguo **no** la borra |
| Simular pacman | Edita `.sandbox/packages.txt` (añade ` aur` para los del AUR) y luego `status` | Paquetes añadidos o quitados |
| Permisos | `chmod 600 $DOTKA_HOME/.zshrc` y luego `status` | `(mode 644 -> 600)`, y volver atrás lo corrige |
| Symlink roto | `ln -s /no/existe $DOTKA_HOME/.config/roto`, luego `snap` y `rollback` | Se guarda y restaura como symlink |
| Olvidar puntos | `./dotka log`, luego `./dotka forget 2 --dry-run` y `./dotka prune --keep 2` | Muestra cuánto libera. Nunca deja borrar el punto actual y los números no se reutilizan |

### 5.6 Empezar de cero

```bash
scripts/sandbox.sh clean && scripts/sandbox.sh setup && eval "$(scripts/sandbox.sh env)"
```

---

## 6. Lo que Dotka NO hace (a propósito)

- **No instala ni desinstala paquetes**, y nunca usa `sudo`. Solo te da los comandos.
- **No borra lo que no podría recuperar**: secretos y archivos demasiado grandes se quedan donde están.
- **No vigila carpetas que no rastrea.** En la demo, el instalador crea `~/dots/`, que no está rastreada: Dotka no la ve ni la borra.
- **No es un antivirus.** El mapa de arranque es un análisis del texto de los archivos: avisa de patrones peligrosos, pero no garantiza que algo sea seguro.
- **No ejecuta los scripts que analiza.**

---

## 7. Qué anotar mientras pruebas

- Mensajes confusos o en los que no sabes qué hacer después.
- Detecciones equivocadas: algo marcado sin motivo, o algo que se ejecuta al iniciar y no aparece en `startup`.
- Cualquier cosa que no hayas podido deshacer.
- Que nada haya cambiado fuera del sandbox: `ls ~/.local/share/dotka` debe seguir sin existir.
