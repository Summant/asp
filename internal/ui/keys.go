package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/summant/asp/internal/config"
)

// keymap resolves keys to actions per section (see config.Sections), and
// actions back to keys for the hints on screen.
type keymap struct {
	byKey    map[string]map[string]string   // section → key → action
	byAction map[string]map[string][]string // section → action → keys
}

func newKeymap(keys map[string]map[string][]string) keymap {
	if keys == nil {
		keys = config.Default().Keys
	}
	km := keymap{byKey: map[string]map[string]string{}, byAction: keys}
	for section, actions := range keys {
		km.byKey[section] = map[string]string{}
		for action, ks := range actions {
			for _, k := range ks {
				km.byKey[section][k] = action
			}
		}
	}
	return km
}

// action is what msg does in section, or "".
func (km keymap) action(section string, msg tea.KeyMsg) string {
	return km.byKey[section][msg.String()]
}

// show is the key to display for an action: its first key, or a run of
// arrows ("←→", "↑↓") when it starts with one.
func (km keymap) show(section, action string) string {
	ks := km.byAction[section][action]
	if len(ks) == 0 {
		return ""
	}
	out := config.DisplayKey(ks[0])
	if !isArrow(out) {
		return out
	}
	for _, k := range ks[1:] {
		d := config.DisplayKey(k)
		if !isArrow(d) || strings.Contains(out, d) {
			break
		}
		out += d
	}
	return out
}

func isArrow(s string) bool { return s == "←" || s == "→" || s == "↑" || s == "↓" }

// pair shows two actions' keys together, "←→" or "↑↓" style when both are
// single symbols, "a/d" otherwise.
func (km keymap) pair(section, a, b string) string {
	x, y := km.show(section, a), km.show(section, b)
	if lipgloss.Width(x) == 1 && lipgloss.Width(y) == 1 && !isLetter(x) && !isLetter(y) {
		return x + y
	}
	return x + "/" + y
}

func isLetter(s string) bool {
	return strings.ContainsAny(strings.ToLower(s), "abcdefghijklmnopqrstuvwxyz0123456789")
}

// all lists every key of an action for the help screen.
func (km keymap) all(section, action string) string {
	var out []string
	for _, k := range km.byAction[section][action] {
		out = append(out, config.DisplayKey(k))
	}
	return strings.Join(out, " ")
}

// hint renders "key desc · key desc" pairs in the footer's two tones.
func hint(pairs ...string) string {
	var parts []string
	for i := 0; i+1 < len(pairs); i += 2 {
		if pairs[i] == "" {
			continue
		}
		parts = append(parts, strings.TrimSpace(pairs[i]+" "+pairs[i+1]))
	}
	return helpDesc.Render(strings.Join(parts, " "+sep+" "))
}
