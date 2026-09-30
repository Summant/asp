package ui

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// browser walks the filesystem one folder at a time, nvim-picker style:
// type to narrow the current folder, → or tab to go in, ← to go up, enter
// to choose. It returns a folder to the prompt it was opened from.
type browser struct {
	from    mode   // prompt to return to
	saved   string // that prompt's input, restored on esc
	dir     string
	entries []dirEntry
	sel     int
}

type dirEntry struct {
	name string
	dir  bool
}

// here is the first row: choose the folder being browsed.
const here = "./"

func (m *Model) openBrowser(from mode, dir string) {
	m.browse = browser{from: from, saved: m.input.Value()}
	m.mode = modeBrowse
	m.openInput("")
	m.browseTo(dir)
}

func (m *Model) browseTo(dir string) {
	m.browse.dir = dir
	m.browse.sel = 0
	m.browse.entries = readEntries(dir)
	m.input.SetValue("")
}

func readEntries(dir string) []dirEntry {
	list, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	out := make([]dirEntry, 0, len(list))
	for _, e := range list {
		out = append(out, dirEntry{e.Name(), isDir(filepath.Join(dir, e.Name()))})
	}
	sort.SliceStable(out, func(a, b int) bool {
		if out[a].dir != out[b].dir {
			return out[a].dir // folders first
		}
		return strings.ToLower(out[a].name) < strings.ToLower(out[b].name)
	})
	return out
}

// visible is "./" followed by the entries matching the typed filter.
// Hidden entries appear once the filter starts with ".".
func (b browser) visible(filter string) []dirEntry {
	out := []dirEntry{{here, true}}
	for _, e := range b.entries {
		if strings.HasPrefix(e.name, ".") && !strings.HasPrefix(filter, ".") {
			continue
		}
		if _, ok := match(e.name, filter); ok {
			out = append(out, e)
		}
	}
	return out
}

func (m Model) browseSelected() (dirEntry, []dirEntry) {
	vis := m.browse.visible(m.input.Value())
	return vis[min(m.browse.sel, len(vis)-1)], vis
}

func (m Model) updateBrowse(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	sel, vis := m.browseSelected()
	switch msg.String() {
	case "ctrl+c":
		return m.quit(false)
	case "esc":
		m.mode = m.browse.from
		m.openInput(m.browse.saved)
		return m, nil
	case "down", "ctrl+n":
		m.browse.sel = min(m.browse.sel+1, len(vis)-1)
		return m, nil
	case "up", "ctrl+p":
		m.browse.sel = max(m.browse.sel-1, 0)
		return m, nil
	case "right", "tab":
		if sel.dir && sel.name != here {
			m.browseTo(filepath.Join(m.browse.dir, sel.name))
		}
		return m, nil
	case "left":
		m.browseTo(filepath.Dir(m.browse.dir))
		return m, nil
	case "backspace":
		if m.input.Value() == "" {
			m.browseTo(filepath.Dir(m.browse.dir))
			return m, nil
		}
	case "enter":
		chosen := m.browse.dir
		if sel.dir && sel.name != here {
			chosen = filepath.Join(m.browse.dir, sel.name)
		}
		return m.chooseFolder(chosen)
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	m.browse.sel = min(m.browse.sel, len(m.browse.visible(m.input.Value()))-1)
	if m.browse.sel == 0 && len(m.browse.visible(m.input.Value())) > 1 && m.input.Value() != "" {
		m.browse.sel = 1 // typing narrows to entries; jump past "./"
	}
	return m, cmd
}

// chooseFolder hands the folder back to the prompt the browser came from.
func (m Model) chooseFolder(dir string) (tea.Model, tea.Cmd) {
	switch m.browse.from {
	case modeFilter:
		m.mode = modeFilter
		v := strings.TrimSpace(m.browse.saved)
		if v != "" {
			v += " "
		}
		m.openInput(v + "f:" + quoteTerm(collapseHome(dir)) + " ")
		m.applyQuery()
	default:
		m.mode = modeNewDir
		m.openInput(collapseHome(dir))
	}
	return m, nil
}
