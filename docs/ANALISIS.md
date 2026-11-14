# Dotka — Análisis del proyecto

> **Versión 0.2** · 14 de noviembre de 2026
> Reemplaza al documento base v0.1 ("DotPilot"). Recoge las decisiones tomadas tras la revisión de la idea.

---

## 1. En una frase

**Dotka es una máquina del tiempo para tu entorno de usuario en Linux: te dice exactamente qué cambió ese `install.sh` (archivos, paquetes, servicios y lo que se ejecuta al iniciar sesión) y te deja volver atrás.**

Toma de Nix la idea de las **generaciones** (estados numerados del entorno a los que se puede volver), pero **sin lenguaje declarativo ni archivos de configuración nuevos**. Funciona sobre el `$HOME` que ya tienes, con las herramientas que ya usas.

---

## 2. El problema real

El documento v0.1 describía bien el dolor, pero lo repartía en demasiados frentes (inventario, clasificación, respaldos, auditoría…). Al reducirlo queda **un problema central**:

> **La ruleta del `install.sh`.** Un ricer ve unos dotfiles bonitos en r/unixporn, clona el repo y ejecuta `./install.sh`. A partir de ahí ya no sabe:
> - qué archivos de `~/.config` se sobrescribieron (y si había algo suyo),
> - qué paquetes se instalaron (pacman + AUR),
> - qué servicios de usuario se habilitaron,
> - **qué se va a ejecutar ahora cada vez que inicia sesión o abre una terminal**,
> - cómo volver a como estaba antes.

Tres semanas después prueba otros dotfiles y el desorden se acumula: archivos huérfanos, paquetes que nadie usa, un `exec-once` que lanza algo que ya no reconoce.

### 2.1 Por qué no lo resuelven las herramientas existentes

| Herramienta | Qué hace bien | Por qué no resuelve esto |
|---|---|---|
| **chezmoi / yadm / GNU Stow** | Gestionar **tus** dotfiles desde un repo fuente | Parten de que tú eres el autor. No capturan lo que hace el script de otro ni rastrean paquetes o servicios. |
| **home-manager (Nix)** | Entorno declarativo y reproducible con generaciones | Exige Nix y reescribir tu configuración en su lenguaje. Barrera de entrada enorme para un ricer de Arch. |
| **Timeshift / Snapper** | Snapshots del sistema (btrfs/rsync) | Pensados para `/`, con mucha granularidad gruesa. No explican *qué* cambió ni lo relacionan con paquetes o con lo que se ejecuta al iniciar. Restaurar es todo o nada. |
| **git init en ~/.config** | Historial de archivos | No ve paquetes, servicios ni fuentes, se lleva mal con binarios y no entiende qué es "algo que se ejecuta al iniciar". |
| **Leer el install.sh a mano** | Lo correcto en teoría | Nadie lo hace, y los scripts llaman a otros scripts, descargan cosas, etc. |

**El hueco que ocupa Dotka:** capturar y explicar los cambios en todo el entorno de usuario, **sin importar quién o qué los hizo**, con vuelta atrás segura.

---

## 3. Qué hace único a Dotka

1. **Generaciones del entorno, no de un repo.** Una generación incluye los archivos, los **paquetes**, los **servicios de usuario** y el **mapa de arranque**, todo en un solo estado comparable.
2. **El mapa de arranque como concepto central.** En los dotfiles, la configuración también es código: `exec-once` de Hyprland, `.desktop` en autostart, unidades de systemd de usuario, líneas de `.zshrc` que hacen `curl … | sh`. Dotka extrae esa lista y la compara entre generaciones:
   ```
   + exec-once = ~/.local/bin/wallpaper-daemon      (hypr/hyprland.conf:42)
   + autostart: discord.desktop
   + systemd --user: pipewire-eq.service (enabled)
   ~ .zshrc:17  eval "$(curl -fsSL https://…)"       ⚠ descarga y ejecuta
   ```
   Ninguna herramienta actual responde a *"¿qué se ejecuta ahora en mi sesión que antes no se ejecutaba?"*.
3. **La vuelta atrás también se puede deshacer.** Antes de cambiar de generación, Dotka guarda automáticamente el estado actual. Nada se borra de verdad: los archivos que se quitan del `$HOME` siguen en el store.
4. **Honestidad con los paquetes.** Dotka nunca usa `sudo`. Calcula qué paquetes sobran y **genera el comando exacto** (`sudo pacman -Rns …`) para que el usuario lo revise y lo ejecute él mismo.
5. **Hoja de ruta con identidad propia:** probar dotfiles en un sandbox sin tocar tu `$HOME` (`dotka try`), adoptar componentes sueltos de repos ajenos (`dotka adopt <repo> waybar`) y compartir generaciones como alternativa transparente al `install.sh`.

---

## 4. Decisiones tomadas

| Tema | Decisión | Motivo |
|---|---|---|
| Usuario principal | **Ricer intermedio** en Arch y derivadas | Es quien sufre el problema con más frecuencia. Domina la terminal pero no audita scripts. |
| Modelo | **Generaciones sin declarativo** | Lo útil de Nix (volver a cualquier estado) sin su barrera de entrada. |
| Cómo se capturan los cambios | **Por fases:** foto antes y después en el MVP, **sandbox** como función estrella posterior | La foto es simple y segura. El sandbox es lo más original, pero también lo más complejo. |
| Qué entra en un estado | Archivos de configuración, **lo que se ejecuta al iniciar**, **fuentes, temas e iconos**, **paquetes** (pacman + AUR) y **servicios de usuario** | Es todo lo que un instalador de dotfiles suele tocar. |
| Materialización | **Archivos reales en `$HOME` + store sin duplicados** (por hash) | Las apps que reescriben su propia config (KDE, VS Code…) no se rompen, a diferencia de los symlinks de Nix. |
| Paquetes al volver atrás | **Generar comandos**, nunca ejecutarlos | Mínimo privilegio. Dotka no usa `sudo` en ningún caso. |
| Secretos | **Detectar, excluir por defecto y preguntar**. Store con permisos `0700` | Un respaldo nunca debe filtrar tokens sin que lo sepas. |
| Adoptar dotfiles ajenos | **Por componentes** (fuera del MVP) | Tomar la waybar de un repo y el rofi de otro. Nadie lo resuelve bien hoy. |
| Distribución | **Arch + AUR** en el MVP (pacman, `pacman -Qm`) | Ahí está la comunidad ricer. El núcleo no depende de pacman, que queda detrás de una interfaz. |
| Gestor de ventanas | **Hyprland** como única referencia del MVP | Es el WM del autor y el más popular entre ricers. Sway, i3 y otros llegan después. |
| Volver atrás | **Quita los archivos añadidos** después de la generación (siguen en el store) | Es lo que limpia los residuos. `--keep-new` queda disponible para conservarlos. |
| Licencia | **MIT** | Adopción máxima y pocas fricciones para contribuir. |
| Lenguaje | **Go**, un solo binario | Distribución fácil (AUR, releases), compilación rápida para el plazo de una semana, y `mvdan.cc/sh` es el mejor parser de shell disponible como biblioteca. Rust era la alternativa: más seguridad de tipos, pero más fricción para este plazo. |
| Contexto | Proyecto académico **y** open source publicado | Plazo de **~1 semana** para el MVP. Después se mantiene como proyecto comunitario. |

---

## 5. Conceptos

- **Raíces rastreadas:** directorios y archivos que Dotka observa (ver §7.2).
- **Estado:** la foto completa de las raíces rastreadas más los paquetes, los servicios y el mapa de arranque en un momento dado.
- **Generación:** un estado guardado, inmutable y numerado (`#1`, `#2`…), con mensaje, fecha y origen (`manual`, `auto: antes de rollback`, …).
- **Store:** almacén de contenido por hash (`sha256`). El mismo archivo en diez generaciones ocupa espacio una sola vez.
- **Drift:** diferencia entre el `$HOME` actual y la última generación, es decir, cambios hechos fuera de Dotka.
- **Mapa de arranque:** lista normalizada de todo lo que se ejecuta automáticamente en la sesión o la shell del usuario, con el archivo y la línea de origen.

---

## 6. Experiencia de uso (CLI del MVP)

La CLI estará en **inglés**, que es la convención de la comunidad. La documentación estará en español e inglés.

```bash
dotka init                          # crea el store y la configuración, y toma la generación #1
dotka snap -m "antes de dots de X"  # guarda una generación nueva
# … el usuario ejecuta él mismo ./install.sh …
dotka status                        # qué cambió desde la última generación (drift)
dotka diff                          # cambios detallados: archivos, paquetes, servicios, arranque
dotka diff 3 5 --path hypr/         # compara dos generaciones, filtrando por ruta
dotka log                           # lista de generaciones
dotka show 4                        # detalle de una generación
dotka startup                       # mapa de arranque actual, con origen de cada entrada
dotka rollback 3                    # vuelve a la #3: plan → confirmación → aplicar
dotka gc                            # limpia del store los objetos que ninguna generación usa
```

### Ejemplo de `dotka status` tras un install.sh

```
Changes since generation #4 "antes de dots de X" (12 min ago)

Files      +38  ~6  -1
  ~ .config/hypr/hyprland.conf
  + .config/waybar/ (14 files)
  - .config/kitty/kitty.conf            ⚠ tu archivo fue reemplazado por un symlink
Packages   +7 (3 AUR)
  + swww  + waybar  + rofi-wayland  + cava  + ttf-jetbrains-mono-nerd  …
Services   +1
  + pipewire-eq.service (systemd --user, enabled)
Startup    +3
  + exec-once = swww-daemon                     hypr/hyprland.conf:12
  + exec-once = ~/.local/bin/autoupdate.sh      hypr/hyprland.conf:15   ⚠ script local
  + .zshrc:88 source <(curl -s https://…)                               ⚠ descarga y ejecuta

Run `dotka snap -m "…"` to keep this, or `dotka rollback 4` to undo.
```

### Ejemplo de `dotka rollback 4`

```
Plan to restore generation #4:
  Auto-snapshot current state as #6 (so this rollback is reversible)
  Restore 6 modified files, remove 38 added files (kept in store), restore 1 deleted file

Packages added since #4 — Dotka will NOT remove them. Review and run:
  sudo pacman -Rns swww waybar rofi-wayland cava
  paru -Rns <AUR packages…>

Proceed? [y/N]
```

---

## 7. Diseño técnico del MVP

### 7.1 Estructura del store

```
~/.local/share/dotka/            (0700)
├── objects/ab/cdef0123…         contenido por sha256 (sin duplicados)
├── generations/0004.json        manifiesto de cada generación (inmutable)
├── HEAD                         generación actual
├── log.jsonl                    registro de operaciones
└── config.json                  raíces, exclusiones, límites (JSON: cero dependencias)
```

**Manifiesto de una generación (JSON):**
```json
{
  "id": 4, "message": "antes de dots de X", "created_at": "…", "origin": "manual",
  "entries": [
    {"path": ".config/hypr/hyprland.conf", "type": "file", "mode": "0644", "size": 3121, "hash": "ab…"},
    {"path": ".config/kitty/kitty.conf", "type": "symlink", "target": "../../dots/kitty.conf"}
  ],
  "packages": {"explicit": ["…"], "foreign": ["…"]},
  "startup": [{"kind": "hypr-exec-once", "cmd": "swww-daemon", "source": ".config/hypr/hyprland.conf", "line": 12}],
  "excluded_secrets": [".config/gh/hosts.yml"]
}
```

Para el MVP alcanza con JSON: es simple, legible y fácil de inspeccionar. SQLite no hace falta.

### 7.2 Qué se rastrea por defecto

- `~/.config/`
- `~/.local/bin/`, `~/.local/share/{fonts,icons,themes,applications}/`
- `~/.themes/`, `~/.icons/`, `~/.fonts/`
- Archivos sueltos: `.bashrc .zshrc .zprofile .profile .bash_profile .xinitrc .xprofile .Xresources .gitconfig .tmux.conf .vimrc`

**Excluido siempre:** `~/.cache`, perfiles de navegador (`.mozilla`, `.config/chromium`, `.config/google-chrome`…), `Steam`, `Trash`, sockets, FIFOs y dispositivos. Los archivos de más de **N MB** (configurable, por defecto 10 MB) se registran con hash pero sin contenido, y se avisa al usuario.

### 7.3 Paquetes y servicios

- **Paquetes:** `pacman -Qqe` (explícitos) y `pacman -Qqm` (AUR/foráneos). Son lecturas sin root. Va detrás de una interfaz `PackageSource` para añadir flatpak, apt, etc. más adelante.
- **Servicios de usuario:** el estado de "habilitado" son symlinks en `~/.config/systemd/user/*.wants/`, que **ya se capturan como archivos**. Además se consulta `systemctl --user list-unit-files --state=enabled` para mostrarlo de forma legible.

### 7.4 Mapa de arranque (extractores del MVP)

| Fuente | Qué se extrae |
|---|---|
| Hyprland | `exec`, `exec-once`, `exec-shutdown`, siguiendo los `source = …` (rutas con `~` y globs) |
| XDG autostart | `~/.config/autostart/*.desktop` (respetando `Hidden=`) |
| systemd --user | unidades habilitadas |
| Shell rc | líneas de `.zshrc`, `.bashrc` y `.profile` con patrones de riesgo: `curl`/`wget` + `sh`/`eval`/`source <(…)`, ejecución de rutas locales, gestores de plugins |
| X11 | `.xinitrc` y `.xprofile` |

Cada entrada se marca como **detección confirmada** (sintaxis conocida) o **heurística** (patrón en una shell). Es el principio de *honest reporting* del v0.1.

### 7.5 Reglas de seguridad al restaurar

1. **Antes de restaurar se guarda siempre una generación automática** del estado actual, así que la vuelta atrás nunca pierde datos.
2. Escritura **atómica**: archivo temporal en el mismo directorio, `fsync`, `rename`.
3. **No se siguen symlinks**: se guardan y restauran como symlinks. No se escribe nada fuera de `$HOME` (control de *path traversal* en cada ruta del manifiesto).
4. Los archivos "añadidos" que la vuelta atrás quita del `$HOME` **siguen en el store**, así que nada se borra de verdad.
5. Siempre se muestra el plan y se pide confirmación. `--dry-run` disponible.
6. Dotka **nunca** ejecuta `sudo`, scripts de terceros ni gestores de paquetes en modo escritura.

### 7.6 Secretos

- Lista de rutas conocidas (`gh/hosts.yml`, `rclone/rclone.conf`, `.netrc`, `.ssh/` (no rastreada), `*/token*`, configuraciones de clientes de correo…) y patrones de contenido (`ghp_…`, `-----BEGIN … PRIVATE KEY-----`, `password=`).
- Lo que se detecta **se excluye por defecto** y queda registrado en el manifiesto. `dotka secrets` lista lo excluido y permite incluirlo explícitamente.
- El contenido de los secretos nunca se imprime en `diff` ni en los logs.

### 7.7 Arquitectura del código

```
cmd/dotka/          CLI (stdlib flag o cobra)
internal/scan/      recorrido de las raíces, exclusiones, detección de tipos
internal/store/     objetos por hash, manifiestos, gc
internal/gen/       crear, comparar y restaurar generaciones (núcleo)
internal/pkg/       interfaz PackageSource + implementación pacman
internal/startup/   extractores del mapa de arranque
internal/secrets/   detección y exclusión
internal/fsutil/    escritura atómica, rutas seguras
```

Dependencias mínimas: biblioteca estándar, una de diff de texto (`github.com/pmezard/go-difflib` o similar) y, si hace falta, `lipgloss` para dar color a la salida. **La TUI no entra en el MVP.**

---

## 8. Plan de una semana

| Día | Entrega | Prueba que lo valida |
|---|---|---|
| 1 | `scan` + `store`: recorrer las raíces, aplicar exclusiones y guardar objetos por hash | fixtures con archivos, directorios, symlinks y symlinks rotos |
| 2 | `init`, `snap`, `log`, `show`: manifiestos de generación | crear generaciones y verificar que no hay duplicados |
| 3 | `status` y `diff` (archivos), con diff de texto y detección de binarios | archivos añadidos, modificados, borrados y cambio de tipo (archivo → symlink) |
| 4 | `rollback` con generación automática previa, escritura atómica y `--dry-run` | volver atrás y volver adelante sin perder nada; destino modificado; permisos |
| 5 | Paquetes (pacman/AUR) + servicios de usuario + comandos sugeridos | interfaz `PackageSource` con implementación falsa en los tests |
| 6 | Mapa de arranque (Hyprland, autostart, systemd, shell rc) + secretos | configuraciones de ejemplo con entradas conocidas y trampas |
| 7 | README (español/inglés), demo grabada (asciinema), release y PKGBUILD para el AUR | instalar desde cero en un usuario limpio |

**Si falta tiempo, se recorta en este orden:** detección de secretos por contenido (quedan solo las rutas) → extractor de shell rc → `gc`.
**Lo que no se toca:** que la vuelta atrás sea segura y reversible. Es la promesa del proyecto.

### Criterios de aceptación del MVP

- [ ] `snap` → ejecutar un `install.sh` de ejemplo → `status` muestra archivos, paquetes, servicios y entradas de arranque nuevas.
- [ ] `rollback` devuelve los archivos al estado exacto (verificado por hash) y es reversible a su vez.
- [ ] Ninguna operación escribe fuera de `$HOME` ni sigue symlinks fuera de las raíces.
- [ ] Ningún secreto conocido aparece en el store sin consentimiento explícito.
- [ ] Los tests cubren: permisos denegados, symlinks rotos, destino modificado tras la generación y rutas con `..`.
- [ ] Se instala desde el AUR o con `go install` y funciona en un usuario limpio.

---

## 9. Hoja de ruta posterior

| Versión | Función | Por qué importa |
|---|---|---|
| **0.2** | `dotka run -- ./install.sh`: generación automática antes y después, en una sola transacción | Comodidad: es el flujo del MVP en un solo comando, y el usuario sigue ejecutando el script de forma explícita. |
| **0.2** | **Detección de cambios no gestionados**: avisar del drift al abrir una shell o mediante un timer | Detecta cambios que hicieron un AUR helper o una app por su cuenta. |
| **0.3** | ⭐ **`dotka try`**: ejecutar el instalador en un **sandbox** (bubblewrap + overlayfs sobre `$HOME`). Ves el resultado de `status` *sin haber tocado tu sistema* y decides `apply` o `discard` | Es la función estrella: probar dotfiles sin riesgo. Ninguna herramienta de dotfiles lo ofrece. |
| **0.4** | **`dotka adopt <repo> waybar rofi`**: adoptar componentes sueltos con plan, conflictos y dependencias que faltan | Permite combinar piezas de varios repos de forma limpia. |
| **0.5** | **Compartir generaciones**: exportar una generación o un componente como paquete autodescriptivo (archivos + paquetes + servicios + arranque, todo declarado) | Una **alternativa transparente al `install.sh`**: el autor publica y quien lo adopta ve todo antes de aplicarlo. |
| **0.5** | **Comparar con otra máquina** (`dotka diff --remote …`, o importando la generación exportada) | Para quien tiene portátil y sobremesa con el "mismo" setup. |
| Después | TUI con Bubble Tea, flatpak/apt/dnf, más extractores (sway, i3, niri, KDE, GNOME) | Ampliar público. |

---

## 10. Riesgos

| Riesgo | Mitigación |
|---|---|
| El store crece demasiado (fuentes, temas) | Sin duplicados por hash, límite de tamaño por archivo, `gc`, y `dotka du` en el futuro |
| Una app reescribe su config mientras se restaura | Avisar si hay procesos conocidos en ejecución (Hyprland, waybar). Recomendar restaurar desde la TTY en casos críticos. |
| Falsos positivos o negativos en el mapa de arranque | Marcar confirmado o heurístico. Nunca afirmar "seguro". Mostrar el archivo y la línea de origen. |
| Volver atrás con paquetes que ya no existen | Dotka solo sugiere comandos y no garantiza la reinstalación. Queda documentado como limitación. |
| Alcance excesivo en una semana | Plan diario con orden de recorte explícito (§8) |
| El sandbox (0.3) resulta inviable (overlayfs sin root, apps de Wayland) | Es una fase posterior y el MVP no depende de él. Investigar `bwrap --overlay` y fuse-overlayfs. |

---

## 11. Principios (se mantienen del v0.1)

Primero solo lectura · Plan antes de aplicar · Consentimiento explícito · Mínimo privilegio (sin `sudo`, nunca) · Nada se ejecuta solo · **Toda operación se puede deshacer** · Informes honestos (confirmado / heurístico / desconocido) · Manejo seguro del sistema de archivos.

---

## 12. Preguntas abiertas para el autor

Hay que responderlas antes o durante el día 1:

**Resueltas:**
- Volver atrás quita los archivos añadidos (con `--keep-new` para conservarlos).
- Hyprland como WM de referencia, con soporte para la configuración nueva en Lua (`hyprland.lua`, `hl.exec_cmd`) y la clásica (`hyprland.conf`, `exec-once`).
- Licencia MIT. Lenguaje Go.
- Raíces rastreadas extensibles: `dotka track add <ruta>` permite añadir otras rutas (por ejemplo `~/.local/share/nvim`).
- El nombre `dotka` está disponible.
- No hace falta documento formal por ahora. El PDF académico (planteamiento, objetivos, diagrama de secuencia) se elabora al final.
- Repositorio git con ramas por funcionalidad y commits con el formato `tipo: descripción` (`feat`, `fix`, `chore`, `docs`, `test`, `refactor`). Remotos previstos: GitHub y GitLab.
- Desarrollo y pruebas siempre sobre un `$HOME` falso (`--home` / `DOTKA_HOME`). Nunca sobre la configuración real del autor.

**Abiertas:**
1. Repo real de dotfiles de Hyprland para la demo del README (se puede probar dentro del `$HOME` falso).
