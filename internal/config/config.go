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
	"reflect"
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
	{"group", "#9aa4b1", "#group tags for groups without a colour of their own"},
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
		{"group_add", []string{"g"}, "add to a group"},
		{"group_remove", []string{"G"}, "remove from a group"},
		{"group_color", []string{"p"}, "recolour a group of the selected session"},
		{"rename", []string{"r"}, "rename"},
		{"unname", []string{"x"}, "clear the name"},
		{"details", []string{"v"}, "everything about the session, full width"},
		{"copy", []string{"y"}, "copy the opening message"},
		{"view_next", []string{"right", "d"}, "next tab: all → claude → codex → your group tabs"},
		{"view_prev", []string{"left", "a"}, "previous tab"},
		{"tab_add", []string{"+"}, "add a tab: claude, codex, or a group (up to 5 groups)"},
		{"tab_close", []string{"-"}, "close the current tab (all stays)"},
		{"end", []string{"c"}, "end the selected paused session (it can be resumed later)"},
		{"down", []string{"j", "down"}, "move down"},
		{"up", []string{"k", "up"}, "move up"},
		{"next_page", []string{"l", "pgdown"}, "next page"},
		{"prev_page", []string{"h", "pgup"}, "previous page"},
		{"first", []string{"home"}, "first session"},
		{"last", []string{"end"}, "last session"},
		{"help", []string{"?"}, "list all keys"},
		{"back", []string{"esc"}, "clear the filter (never quits)"},
		{"quit", []string{"q"}, "quit (asks first if sessions are paused); ctrl+c also quits"},
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
// error. Any error returned is a list of problems, not a failure: the
// Config is always usable — every valid setting applied, the rest left at
// their defaults — so a mistake, or a setting a newer or older asp does
// not know, never stops asp from starting.
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

// Parse reads TOML text over the defaults; name is used in messages. As
// with Load, the Config is usable even when problems are returned.
func Parse(text, name string) (Config, error) {
	c := Default()
	var f file
	md, err := toml.Decode(text, &f)
	if err != nil {
		return c, fmt.Errorf("%s: %w (using the defaults)", name, err)
	}
	var problems []string
	for _, k := range md.Undecoded() {
		problems = append(problems, fmt.Sprintf("unknown setting %q (ignored)", k.String()))
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
	userSet := map[string]map[string]bool{} // section → actions the file sets
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
			if userSet[section] == nil {
				userSet[section] = map[string]bool{}
			}
			userSet[section][action] = true
		}
	}
	// One key, one action, per section. Keys the user chose win: a default
	// that collides with one quietly gives it up (so a new action added in
	// a later version never breaks a config written for an earlier one).
	// Two actions the user bound to the same key is a mistake: the first
	// in the list keeps it.
	for _, s := range Sections {
		owner := map[string]string{}
		for _, b := range s.Bindings {
			if !userSet[s.Name][b.Action] {
				continue
			}
			var keep []string
			for _, k := range c.Keys[s.Name][b.Action] {
				if prev, dup := owner[k]; dup {
					problems = append(problems, fmt.Sprintf("keys.%s: %q is bound to both %s and %s; %s keeps it", s.Name, DisplayKey(k), prev, b.Action, prev))
					continue
				}
				owner[k] = b.Action
				keep = append(keep, k)
			}
			c.Keys[s.Name][b.Action] = keep
		}
		for _, b := range s.Bindings {
			if userSet[s.Name][b.Action] {
				continue
			}
			var keep []string
			for _, k := range c.Keys[s.Name][b.Action] {
				if _, taken := owner[k]; !taken {
					keep = append(keep, k)
				}
			}
			c.Keys[s.Name][b.Action] = keep
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
		b.WriteString(colorLine(c))
	}
	for _, s := range Sections {
		b.WriteString(sectionHeader(s))
		for _, bind := range s.Bindings {
			b.WriteString(bindingLine(bind))
		}
	}
	return b.String()
}

func colorLine(c Color) string {
	return fmt.Sprintf("%-13s = %-10q # %s\n", c.Name, c.Default, c.Desc)
}

func sectionHeader(s Section) string { return fmt.Sprintf("\n# %s\n[keys.%s]\n", s.Desc, s.Name) }

func bindingLine(bind Binding) string {
	quoted := make([]string, len(bind.Keys))
	for i, k := range bind.Keys {
		quoted[i] = strconv.Quote(k)
	}
	return fmt.Sprintf("%-12s = %-32s # %s\n", bind.Action, "["+strings.Join(quoted, ", ")+"]", bind.Desc)
}

var tableHeader = regexp.MustCompile(`^\s*\[([A-Za-z0-9_.]+)\]\s*(#.*)?$`)

// AddMissing adds every setting text lacks — with its default and what it
// does — to the right section, and returns the new text and what was
// added. Nothing already in text changes, and the result must load to
// exactly the same configuration; if it would not (a file laid out in a
// way this cannot follow), an error is returned and nothing should be
// written.
func AddMissing(text string) (string, []string, error) {
	var f file
	if _, err := toml.Decode(text, &f); err != nil {
		return "", nil, fmt.Errorf("the file does not parse, so it was left alone: %w", err)
	}
	// Missing settings are written with the values they have now: the
	// defaults, except a default key that already gave way to one of the
	// user's (see Parse) is written without it.
	before, _ := Parse(text, "before")
	type block struct {
		table string   // "colors", "keys.list", …
		head  string   // header to add if the table is not there
		lines []string // settings to add
		names []string // for the report
	}
	var blocks []block
	col := block{table: "colors", head: "\n[colors]\n"}
	for _, c := range Colors {
		if _, ok := f.Colors[c.Name]; !ok {
			col.lines = append(col.lines, colorLine(c))
			col.names = append(col.names, "colors."+c.Name)
		}
	}
	blocks = append(blocks, col)
	for _, s := range Sections {
		b := block{table: "keys." + s.Name, head: sectionHeader(s)}
		for _, bind := range s.Bindings {
			if _, ok := f.Keys[s.Name][bind.Action]; !ok {
				now := bind
				now.Keys = before.Keys[s.Name][bind.Action]
				b.lines = append(b.lines, bindingLine(now))
				b.names = append(b.names, "keys."+s.Name+"."+bind.Action)
			}
		}
		blocks = append(blocks, b)
	}

	lines := strings.SplitAfter(text, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) > 0 && !strings.HasSuffix(lines[len(lines)-1], "\n") {
		lines[len(lines)-1] += "\n"
	}
	var added []string
	var tail []string
	for _, b := range blocks {
		if len(b.lines) == 0 {
			continue
		}
		added = append(added, b.names...)
		// Find the table, and the last setting in it (before the next table).
		at := -1
		for i, l := range lines {
			if m := tableHeader.FindStringSubmatch(l); m != nil && m[1] == b.table {
				at = i
				for j := i + 1; j < len(lines); j++ {
					if tableHeader.MatchString(lines[j]) {
						break
					}
					if t := strings.TrimSpace(lines[j]); t != "" && !strings.HasPrefix(t, "#") {
						at = j
					}
				}
				break
			}
		}
		if at < 0 {
			tail = append(tail, b.head)
			tail = append(tail, b.lines...)
			continue
		}
		lines = append(lines[:at+1], append(append([]string{}, b.lines...), lines[at+1:]...)...)
	}
	out := strings.Join(lines, "") + strings.Join(tail, "")

	// The promise: same configuration as before, only now written out.
	after, _ := Parse(out, "after")
	if _, err := toml.Decode(out, &file{}); err != nil || !reflect.DeepEqual(before, after) {
		return "", nil, errors.New("could not add the missing settings without changing the file's meaning; it was left alone")
	}
	return out, added, nil
}

// UpdateFile adds missing settings to the config file at path, keeping a
// copy of the old file as path+".bak". It reports what was added.
func UpdateFile(path string) ([]string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	out, added, err := AddMissing(string(b))
	if err != nil || len(added) == 0 {
		return nil, err
	}
	if err := os.WriteFile(path+".bak", b, 0o644); err != nil {
		return nil, err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(out), 0o644); err != nil {
		return nil, err
	}
	return added, os.Rename(tmp, path)
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
