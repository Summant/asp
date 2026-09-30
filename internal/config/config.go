// Package config reads ~/.config/asp/config.toml: the colour scheme and
// every keybinding. Everything is optional; a missing file or setting keeps
// the default. Mistakes — an unknown setting, a bad colour, an unknown key
// name, one key bound twice in the same place — are reported, not ignored.
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/BurntSushi/toml"
)

// Color is one colour role in the scheme.
type Color struct {
	Name, Default, Desc string
}

// Colors are the roles, in the order the example file lists them.
var Colors = []Color{
	{"accent", "#7a1fff", "selected gutter and title, logo background, claude tag, prompt labels"},
	{"accent_muted", "#9d7bff", "the selected item's second line"},
	{"secondary", "#8ec9ff", "filter-match gutter, codex tag, key names, headings, confirmations"},
	{"text", "#cfd8e3", "titles and values"},
	{"text_strong", "#ffffff", "logo text"},
	{"muted", "#6b7280", "second lines, key descriptions, labels, ○"},
	{"subtle", "#4a4560", "the header's session counts"},
	{"rule", "#2e2a3d", "the line between list and details, inactive page dot"},
	{"dot_active", "#9aa4b1", "active page dot"},
	{"error", "#ff5555", "error messages"},
}

// Binding is one action and its default keys.
type Binding struct {
	Action string
	Keys   []string
	Desc   string
}

// Section is a place keys are read: the list, a prompt, the browser, …
type Section struct {
	Name, Desc string
	Bindings   []Binding
}

// Sections holds every configurable binding with its default.
var Sections = []Section{
	{"list", "the session list", []Binding{
		{"open", []string{"enter"}, "open the session, or go back to a paused one"},
		{"new", []string{"n"}, "new session: agent, name, folder"},
		{"filter", []string{"/"}, "filter (f:folder, g:group, \"quoted words\")"},
		{"folders", []string{"f"}, "browse folders and show the sessions in one"},
		{"group_add", []string{"b"}, "add to a group"},
		{"group_remove", []string{"B"}, "remove from a group"},
		{"rename", []string{"r"}, "rename"},
		{"unname", []string{"x"}, "clear the name"},
		{"details", []string{"v"}, "everything about the session, full width"},
		{"copy", []string{"y"}, "copy the opening message"},
		{"view_next", []string{"right", "d"}, "next view: all → claude → codex"},
		{"view_prev", []string{"left", "a"}, "previous view"},
		{"down", []string{"j", "down"}, "move down"},
		{"up", []string{"k", "up"}, "move up"},
		{"next_page", []string{"l", "pgdown"}, "next page"},
		{"prev_page", []string{"h", "pgup"}, "previous page"},
		{"first", []string{"g", "home"}, "first session"},
		{"last", []string{"G", "end"}, "last session"},
		{"help", []string{"?"}, "list all keys"},
		{"back", []string{"esc"}, "clear the filter, or quit if there is none"},
		{"quit", []string{"q"}, "quit (asks first if sessions are paused)"},
	}},
	{"prompt", "text prompts: filter, rename, new-session name and folder, groups", []Binding{
		{"confirm", []string{"enter"}, "accept"},
		{"cancel", []string{"esc"}, "cancel"},
		{"next", []string{"down", "ctrl+n"}, "next suggestion"},
		{"prev", []string{"up", "ctrl+p"}, "previous suggestion"},
		{"fill", []string{"tab"}, "fill in the suggestion"},
		{"browse", []string{"ctrl+o"}, "open the folder browser (folder prompt, filter)"},
	}},
	{"agent", "choosing the agent for a new session", []Binding{
		{"toggle", []string{"left", "right", "tab", "h", "l"}, "switch between claude and codex"},
		{"claude", []string{"c"}, "choose claude"},
		{"codex", []string{"o"}, "choose codex"},
		{"confirm", []string{"enter"}, "next step"},
		{"cancel", []string{"esc"}, "cancel"},
	}},
	{"finder", "the folder browser", []Binding{
		{"open", []string{"enter", "right"}, "go into the folder (or use it, on \"Use this folder\")"},
		{"parent", []string{"left"}, "go up a folder (backspace also does, when nothing is typed)"},
		{"use", []string{"tab"}, "use the highlighted folder without going in"},
		{"next", []string{"down", "ctrl+n", "ctrl+j"}, "move down"},
		{"prev", []string{"up", "ctrl+p", "ctrl+k"}, "move up"},
		{"next_page", []string{"pgdown"}, "next page"},
		{"prev_page", []string{"pgup"}, "previous page"},
		{"cancel", []string{"esc"}, "close the browser"},
	}},
	{"details", "the details view (v)", []Binding{
		{"down", []string{"j", "down"}, "scroll down"},
		{"up", []string{"k", "up"}, "scroll up"},
		{"page_down", []string{"space", "pgdown", "f"}, "page down"},
		{"page_up", []string{"b", "pgup"}, "page up"},
		{"top", []string{"g", "home"}, "top"},
		{"bottom", []string{"G", "end"}, "bottom"},
		{"copy", []string{"y"}, "copy the opening message"},
		{"back", []string{"esc", "v", "q"}, "back to the list"},
	}},
}

// Config is the resolved configuration: every colour and every binding,
// defaults filled in.
type Config struct {
	Colors map[string]string              // role → colour
	Keys   map[string]map[string][]string // section → action → keys
}

// file is the TOML shape; everything optional.
type file struct {
	Colors map[string]string              `toml:"colors"`
	Keys   map[string]map[string][]string `toml:"keys"`
}

// Path is where the config lives.
func Path() string {
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(dir, "asp", "config.toml")
}

// Default is the configuration with nothing overridden.
func Default() Config {
	c := Config{Colors: map[string]string{}, Keys: map[string]map[string][]string{}}
	for _, col := range Colors {
		c.Colors[col.Name] = col.Default
	}
	for _, s := range Sections {
		c.Keys[s.Name] = map[string][]string{}
		for _, b := range s.Bindings {
			keys := make([]string, len(b.Keys))
			for i, k := range b.Keys {
				keys[i], _ = NormalizeKey(k) // defaults are valid; see TestDefaultsValid
			}
			c.Keys[s.Name][b.Action] = keys
		}
	}
	return c
}

// Load reads the file at path over the defaults. A missing file is not an
// error.
func Load(path string) (Config, error) {
	c := Default()
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return c, nil
	}
	if err != nil {
		return c, err
	}
	return Parse(string(b), path)
}

// Parse reads TOML text over the defaults; name is used in messages.
func Parse(text, name string) (Config, error) {
	c := Default()
	var f file
	md, err := toml.Decode(text, &f)
	if err != nil {
		return c, fmt.Errorf("%s: %w", name, err)
	}
	var problems []string
	for _, k := range md.Undecoded() {
		problems = append(problems, fmt.Sprintf("unknown setting %q", k.String()))
	}
	for role, v := range f.Colors {
		if _, ok := c.Colors[role]; !ok {
			problems = append(problems, fmt.Sprintf("colors: unknown colour %q (known: %s)", role, colorNames()))
			continue
		}
		norm, err := normalizeColor(v)
		if err != nil {
			problems = append(problems, fmt.Sprintf("colors.%s: %v", role, err))
			continue
		}
		c.Colors[role] = norm
	}
	for section, actions := range f.Keys {
		known, ok := c.Keys[section]
		if !ok {
			problems = append(problems, fmt.Sprintf("keys: unknown section %q (known: %s)", section, sectionNames()))
			continue
		}
		for action, keys := range actions {
			if _, ok := known[action]; !ok {
				problems = append(problems, fmt.Sprintf("keys.%s: unknown action %q (known: %s)", section, action, actionNames(section)))
				continue
			}
			var norm []string
			for _, k := range keys {
				n, err := NormalizeKey(k)
				if err != nil {
					problems = append(problems, fmt.Sprintf("keys.%s.%s: %v", section, action, err))
					continue
				}
				norm = append(norm, n)
			}
			known[action] = norm
		}
	}
	// One key, one action, per section.
	for _, s := range Sections {
		owner := map[string]string{}
		for _, b := range s.Bindings {
			for _, k := range c.Keys[s.Name][b.Action] {
				if prev, dup := owner[k]; dup && prev != b.Action {
					problems = append(problems, fmt.Sprintf("keys.%s: %q is bound to both %s and %s", s.Name, DisplayKey(k), prev, b.Action))
				}
				owner[k] = b.Action
			}
		}
	}
	if len(problems) > 0 {
		sort.Strings(problems)
		return c, fmt.Errorf("%s:\n  %s", name, strings.Join(problems, "\n  "))
	}
	return c, nil
}

var hexColor = regexp.MustCompile(`^#([0-9a-fA-F]{3}|[0-9a-fA-F]{6})$`)

// normalizeColor accepts "#rrggbb", "#rgb" or an ANSI colour number 0–255.
func normalizeColor(v string) (string, error) {
	v = strings.TrimSpace(v)
	if hexColor.MatchString(v) {
		return strings.ToLower(v), nil
	}
	if n, err := strconv.Atoi(v); err == nil && n >= 0 && n <= 255 {
		return v, nil
	}
	return "", fmt.Errorf("%q is not a colour; use \"#rrggbb\", \"#rgb\" or an ANSI number 0-255", v)
}

// namedKeys are the key names asp understands besides single characters
// and ctrl+/alt+ combinations. Values are Bubble Tea's names.
var namedKeys = map[string]string{
	"enter": "enter", "return": "enter", "esc": "esc", "escape": "esc",
	"tab": "tab", "shift+tab": "shift+tab", "space": " ", "backspace": "backspace",
	"delete": "delete", "up": "up", "down": "down", "left": "left", "right": "right",
	"pgup": "pgup", "pageup": "pgup", "pgdown": "pgdown", "pagedown": "pgdown",
	"home": "home", "end": "end", "insert": "insert",
	"shift+up": "shift+up", "shift+down": "shift+down", "shift+left": "shift+left", "shift+right": "shift+right",
}

func init() {
	for i := 1; i <= 12; i++ {
		namedKeys[fmt.Sprintf("f%d", i)] = fmt.Sprintf("f%d", i)
	}
}

// NormalizeKey turns a key as written in the config into Bubble Tea's name
// for it: "escape" → "esc", "space" → " ", "Ctrl+O" → "ctrl+o"; a single
// character stays as it is, so "G" and "g" differ.
func NormalizeKey(k string) (string, error) {
	if utf8.RuneCountInString(k) == 1 {
		return k, nil
	}
	lower := strings.ToLower(strings.TrimSpace(k))
	if n, ok := namedKeys[lower]; ok {
		return n, nil
	}
	for _, mod := range []string{"ctrl+", "alt+"} {
		if rest, ok := strings.CutPrefix(lower, mod); ok {
			if utf8.RuneCountInString(rest) == 1 {
				return mod + rest, nil
			}
			if n, ok := namedKeys[rest]; ok && mod == "alt+" {
				return mod + n, nil
			}
		}
	}
	return "", fmt.Errorf("unknown key %q (use a character, a name like enter, esc, tab, space, up, pgdown, f1, or ctrl+x / alt+x)", k)
}

// DisplayKey is how a key is shown in hints: arrows and ↵ as symbols.
func DisplayKey(k string) string {
	switch k {
	case "enter":
		return "↵"
	case "up":
		return "↑"
	case "down":
		return "↓"
	case "left":
		return "←"
	case "right":
		return "→"
	case " ":
		return "space"
	}
	return k
}

func colorNames() string {
	var n []string
	for _, c := range Colors {
		n = append(n, c.Name)
	}
	return strings.Join(n, ", ")
}

func sectionNames() string {
	var n []string
	for _, s := range Sections {
		n = append(n, s.Name)
	}
	return strings.Join(n, ", ")
}

func actionNames(section string) string {
	for _, s := range Sections {
		if s.Name == section {
			var n []string
			for _, b := range s.Bindings {
				n = append(n, b.Action)
			}
			return strings.Join(n, ", ")
		}
	}
	return ""
}

// Example is a complete, commented config.toml holding every default.
func Example() string {
	var b strings.Builder
	b.WriteString(`# asp configuration — ~/.config/asp/config.toml
#
# Every setting is optional: delete any line to keep its default. asp
# reports unknown settings, bad colours and clashing keys when it starts.
#
# Colours: "#rrggbb", "#rgb", or an ANSI colour number 0-255. asp never
# paints a background except the logo, so your terminal's own background
# (and its transparency) always shows through.
#
# Keys: a single character ("q", "G", "/"), a name — enter, esc, tab,
# shift+tab, space, backspace, delete, up, down, left, right, pgup, pgdown,
# home, end, f1…f12 — or ctrl+x / alt+x. Each action takes a list of keys.
# ctrl+c always quits, whatever is configured.

[colors]
`)
	for _, c := range Colors {
		fmt.Fprintf(&b, "%-13s = %-10q # %s\n", c.Name, c.Default, c.Desc)
	}
	for _, s := range Sections {
		fmt.Fprintf(&b, "\n# %s\n[keys.%s]\n", s.Desc, s.Name)
		for _, bind := range s.Bindings {
			quoted := make([]string, len(bind.Keys))
			for i, k := range bind.Keys {
				quoted[i] = strconv.Quote(k)
			}
			fmt.Fprintf(&b, "%-12s = %-32s # %s\n", bind.Action, "["+strings.Join(quoted, ", ")+"]", bind.Desc)
		}
	}
	return b.String()
}

// WriteExample writes Example to path, refusing to overwrite a file.
func WriteExample(path string) error {
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("%s already exists; not overwriting it", path)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(Example()), 0o644)
}
