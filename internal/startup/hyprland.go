package startup

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/iYeremy/dotka/internal/gen"
)

const maxSourceDepth = 16

// ---- hyprland.conf (classic syntax) ---------------------------------------

var (
	confExec   = regexp.MustCompile(`^\s*(exec-once|exec-shutdown|execr-once|exec|execr)\s*=\s*(.+?)\s*$`)
	confSource = regexp.MustCompile(`^\s*source\s*=\s*(.+?)\s*$`)
)

func hyprConf(home string) ([]gen.StartupItem, []Warning) {
	root := filepath.Join(home, ".config/hypr/hyprland.conf")
	var items []gen.StartupItem
	var warns []Warning
	visited := map[string]bool{}
	var walk func(path string, depth int)
	walk = func(path string, depth int) {
		if visited[path] || depth > maxSourceDepth {
			return
		}
		visited[path] = true
		lines, err := readLines(path)
		if err != nil {
			if !errors.Is(err, fs.ErrNotExist) || depth > 0 {
				warns = append(warns, Warning{rel(home, path), err})
			}
			return
		}
		for i, line := range lines {
			line = stripHashComment(line)
			if m := confExec.FindStringSubmatch(line); m != nil {
				items = append(items, gen.StartupItem{
					Kind: "hypr-" + m[1], Command: m[2], Source: rel(home, path), Line: i + 1,
					Confidence: Confirmed, Risk: Risk(m[2]),
				})
				continue
			}
			if m := confSource.FindStringSubmatch(line); m != nil {
				p := expandHome(home, m[1])
				if !filepath.IsAbs(p) {
					p = filepath.Join(filepath.Dir(path), p)
				}
				matches, _ := filepath.Glob(p)
				for _, mp := range matches {
					walk(mp, depth+1)
				}
			}
		}
	}
	walk(root, 0)
	return items, warns
}

// stripHashComment removes a trailing "# comment" (Hyprland uses "##" for a
// literal '#').
func stripHashComment(line string) string {
	for i := 0; i < len(line); i++ {
		if line[i] == '#' {
			if i+1 < len(line) && line[i+1] == '#' {
				i++
				continue
			}
			return line[:i]
		}
	}
	return line
}

// ---- hyprland.lua (Lua configuration) -------------------------------------

var (
	luaRequire = regexp.MustCompile(`\brequire\s*\(?\s*["']([\w./-]+)["']`)
	luaPcall   = regexp.MustCompile(`\bpcall\s*\(\s*require\s*,\s*["']([\w./-]+)["']`)
	luaDofile  = regexp.MustCompile(`\bdofile\s*\(\s*["']([^"']+)["']`)
	luaOn      = regexp.MustCompile(`\bhl\.on\s*\(\s*["']([\w.\-]+)["']`)
	luaWord    = regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_]*`)
)

const luaExec = "hl.exec_cmd("

// hyprLua extracts commands run by hl.exec_cmd(...) in the Lua config,
// following require()/dofile(). Commands inside hl.on("hyprland.start", ...)
// are autostart; other top-level calls run every time the config is loaded.
// Keybindings (hl.dsp.exec_cmd) are not startup items and are ignored.
func hyprLua(home string) ([]gen.StartupItem, []Warning) {
	dir := filepath.Join(home, ".config/hypr")
	var items []gen.StartupItem
	var warns []Warning
	visited := map[string]bool{}
	var walk func(path string, depth int)
	walk = func(path string, depth int) {
		if visited[path] || depth > maxSourceDepth {
			return
		}
		visited[path] = true
		lines, err := readLines(path)
		if err != nil {
			if !errors.Is(err, fs.ErrNotExist) {
				warns = append(warns, Warning{rel(home, path), err})
			}
			return
		}
		var (
			inLongComment bool
			depthLvl      int
			pendingEvent  string
			events        []luaEvent // active hl.on blocks
		)
		for i, raw := range lines {
			code := stripLuaComments(raw, &inLongComment)
			if strings.TrimSpace(code) == "" {
				continue
			}
			// Follow modules.
			for _, re := range []*regexp.Regexp{luaPcall, luaRequire} {
				for _, m := range re.FindAllStringSubmatch(code, -1) {
					if p := resolveLuaModule(dir, m[1]); p != "" {
						walk(p, depth+1)
					}
				}
			}
			for _, m := range luaDofile.FindAllStringSubmatch(code, -1) {
				p := expandHome(home, m[1])
				if !filepath.IsAbs(p) {
					p = filepath.Join(dir, p)
				}
				walk(p, depth+1)
			}
			// Commands.
			for _, cmd := range luaExecArgs(code) {
				kind, conf := "hypr-lua-exec", Heuristic
				if len(events) > 0 {
					ev := events[len(events)-1].name
					if ev == "hyprland.start" {
						kind, conf = "hypr-autostart", Confirmed
					} else {
						kind = "hypr-on:" + ev
					}
				}
				if !isLuaStringLiteral(cmd) {
					conf = Heuristic
				}
				items = append(items, gen.StartupItem{
					Kind: kind, Command: unquoteLua(cmd), Source: rel(home, path), Line: i + 1,
					Confidence: conf, Risk: Risk(unquoteLua(cmd)), Dynamic: !isLuaStringLiteral(cmd),
				})
			}
			// Track block nesting to know when hl.on(...) blocks end.
			if m := luaOn.FindStringSubmatch(code); m != nil {
				pendingEvent = m[1]
			}
			for _, w := range luaWord.FindAllString(blankLuaStrings(code), -1) {
				switch w {
				case "function", "if", "do", "repeat":
					if w == "function" && pendingEvent != "" {
						events = append(events, luaEvent{name: pendingEvent, depth: depthLvl})
						pendingEvent = ""
					}
					depthLvl++
				case "end", "until":
					depthLvl--
					if n := len(events); n > 0 && events[n-1].depth == depthLvl {
						events = events[:n-1]
					}
				}
			}
		}
	}
	walk(filepath.Join(dir, "hyprland.lua"), 0)
	return items, warns
}

type luaEvent struct {
	name  string
	depth int
}

func resolveLuaModule(dir, name string) string {
	base := filepath.Join(dir, strings.ReplaceAll(name, ".", "/"))
	for _, p := range []string{base + ".lua", filepath.Join(base, "init.lua")} {
		if fi, err := os.Stat(p); err == nil && fi.Mode().IsRegular() {
			return p
		}
	}
	return "" // not a local module (e.g. provided by Hyprland itself)
}

// luaExecArgs returns the raw first argument of every hl.exec_cmd( call.
func luaExecArgs(code string) []string {
	var out []string
	for {
		i := strings.Index(code, luaExec)
		if i < 0 {
			return out
		}
		rest := code[i+len(luaExec):]
		out = append(out, firstLuaArg(rest))
		code = rest
	}
}

// firstLuaArg reads the argument text up to the matching ')' or ',' at depth 0.
func firstLuaArg(s string) string {
	depth := 0
	var quote byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		if quote != 0 {
			if c == '\\' {
				i++
			} else if c == quote {
				quote = 0
			}
			continue
		}
		switch c {
		case '"', '\'':
			quote = c
		case '(', '{', '[':
			depth++
		case ')', '}', ']':
			if depth == 0 {
				return strings.TrimSpace(s[:i])
			}
			depth--
		case ',':
			if depth == 0 {
				return strings.TrimSpace(s[:i])
			}
		}
	}
	t := strings.TrimSpace(s)
	if t == "" {
		return "..."
	}
	return t + " ..."
}

func isLuaStringLiteral(s string) bool {
	if len(s) < 2 {
		return false
	}
	q := s[0]
	return (q == '"' || q == '\'') && s[len(s)-1] == q && !strings.Contains(s[1:len(s)-1], string(q)+" ..")
}

func unquoteLua(s string) string {
	if isLuaStringLiteral(s) {
		return s[1 : len(s)-1]
	}
	return s
}

// stripLuaComments removes -- comments and --[[ ]] block comments, keeping
// string contents intact.
func stripLuaComments(line string, inLong *bool) string {
	var sb strings.Builder
	var quote byte
	for i := 0; i < len(line); i++ {
		if *inLong {
			if strings.HasPrefix(line[i:], "]]") {
				*inLong = false
				i++
			}
			continue
		}
		c := line[i]
		if quote != 0 {
			sb.WriteByte(c)
			if c == '\\' && i+1 < len(line) {
				i++
				sb.WriteByte(line[i])
			} else if c == quote {
				quote = 0
			}
			continue
		}
		if c == '"' || c == '\'' {
			quote = c
			sb.WriteByte(c)
			continue
		}
		if strings.HasPrefix(line[i:], "--") {
			if strings.HasPrefix(line[i+2:], "[[") {
				*inLong = true
				i += 3
				continue
			}
			break
		}
		sb.WriteByte(c)
	}
	return sb.String()
}

// blankLuaStrings replaces string contents so keywords inside strings are
// not counted as blocks.
func blankLuaStrings(code string) string {
	b := []byte(code)
	var quote byte
	for i := 0; i < len(b); i++ {
		if quote != 0 {
			if b[i] == '\\' && i+1 < len(b) {
				b[i], b[i+1] = ' ', ' '
				i++
				continue
			}
			if b[i] == quote {
				quote = 0
				continue
			}
			b[i] = ' '
			continue
		}
		if b[i] == '"' || b[i] == '\'' {
			quote = b[i]
		}
	}
	return string(b)
}
