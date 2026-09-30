package ui

import (
	"os"
	"slices"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/summant/asp/internal/source"
	"github.com/summant/asp/internal/store"
)

// Launch is what main does after the UI exits: either resume a session or
// start a fresh one, optionally remembering a name for it.
type Launch struct {
	Agent    source.Agent
	Dir      string
	ResumeID string // empty means "start a new session"
	Name     string // name to bind once the new session exists
}

// agentView is which agents' sessions are listed. It is remembered between
// runs.
type agentView int

const (
	viewAll agentView = iota
	viewClaude
	viewCodex
	numViews
)

func (v agentView) String() string {
	return [...]string{"all", "claude", "codex"}[v]
}

func parseView(s string) agentView {
	for v := range numViews {
		if v.String() == s {
			return v
		}
	}
	return viewAll
}

// shows reports whether sessions of agent a are listed in this view.
func (v agentView) shows(a source.Agent) bool {
	switch v {
	case viewClaude:
		return a == source.Claude
	case viewCodex:
		return a == source.Codex
	}
	return true
}

type mode int

const (
	modeList mode = iota
	modeFilter
	modeRename
	modeNewName
	modeNewDir
)

type Model struct {
	items  []Item
	order  []int // indices into items: in the current view and matching query
	cursor int   // index into order; the page is derived from it
	view   agentView
	query  string // active filter, live while typing

	mode     mode
	input    textinput.Model
	newName  string       // carried from the name prompt to the folder prompt
	newAgent source.Agent // agent a new session will use
	status   string       // confirmation; cleared on the next keypress
	err      string       // prompt validation error; cleared on the next keypress

	store *store.Store
	w, h  int

	Result *Launch
}

func New(items []Item, st *store.Store) Model {
	ti := textinput.New()
	ti.Prompt = ""
	ti.CharLimit = 200
	ti.Cursor.Style = promptLabel
	ti.TextStyle = fg(text)

	saved := st.State()
	m := Model{items: items, store: st, input: ti, view: parseView(saved.View)}
	m.reorder()
	// Reopen on the session selected last time, if it is still listed.
	for i, idx := range m.order {
		if key(m.items[idx]) == saved.Last {
			m.cursor = i
		}
	}
	return m
}

func key(it Item) string { return string(it.Session.Agent) + ":" + it.Session.ID }

func (m Model) Init() tea.Cmd { return nil }

// reorder rebuilds the visible list for the current view and query, keeping
// the selected session selected if it is still listed.
func (m *Model) reorder() {
	prev := -1
	if it, ok := m.currentIndex(); ok {
		prev = it
	}
	type hit struct{ idx, score int }
	var hits []hit
	for i, it := range m.items {
		if !m.view.shows(it.Session.Agent) {
			continue
		}
		if s, ok := match(it.Haystack(), m.query); ok {
			hits = append(hits, hit{i, s})
		}
	}
	sort.SliceStable(hits, func(a, b int) bool { return hits[a].score < hits[b].score })
	m.order = m.order[:0]
	for _, h := range hits {
		m.order = append(m.order, h.idx)
	}
	m.cursor = max(0, slices.Index(m.order, prev))
}

func (m Model) currentIndex() (int, bool) {
	if m.cursor < 0 || m.cursor >= len(m.order) {
		return 0, false
	}
	return m.order[m.cursor], true
}

func (m Model) current() (Item, bool) {
	i, ok := m.currentIndex()
	if !ok {
		return Item{}, false
	}
	return m.items[i], true
}

// perPage is how many 3-row items fit in the list area.
func (m Model) perPage() int { return max(1, m.bodyRows()/itemRows) }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		return m, nil
	case tea.KeyMsg:
		m.status, m.err = "", ""
		switch m.mode {
		case modeFilter:
			return m.updateFilter(msg)
		case modeRename, modeNewName, modeNewDir:
			return m.updatePrompt(msg)
		default:
			return m.updateList(msg)
		}
	}
	return m, nil
}

func (m Model) updateList(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	per := m.perPage()
	switch msg.String() {
	case "q", "ctrl+c":
		return m.quit()
	case "esc":
		if m.query != "" {
			m.query = ""
			m.reorder()
			return m, nil
		}
		return m.quit()
	case "j", "down":
		m.move(1)
	case "k", "up":
		m.move(-1)
	case "l", "pgdown":
		if next := (m.cursor/per + 1) * per; next < len(m.order) {
			m.cursor = next
		}
	case "h", "pgup":
		m.cursor = max(0, (m.cursor/per-1)*per)
	case "g", "home":
		m.cursor = 0
	case "G", "end":
		m.cursor = max(0, len(m.order)-1)
	case "right", "d":
		m.setView((m.view + 1) % numViews)
	case "left", "a":
		m.setView((m.view + numViews - 1) % numViews)
	case "/":
		m.mode = modeFilter
		m.query = ""
		m.reorder()
		m.input.SetValue("")
		m.input.Focus()
	case "enter":
		if it, ok := m.current(); ok {
			m.Result = &Launch{Agent: it.Session.Agent, Dir: it.Session.CWD, ResumeID: it.Session.ID}
			return m.quit()
		}
	case "n":
		m.newAgent = source.Claude
		if m.view == viewCodex {
			m.newAgent = source.Codex
		}
		m.openPrompt(modeNewName, "")
	case "r":
		if it, ok := m.current(); ok {
			m.openPrompt(modeRename, it.Name)
		}
	case "x":
		if it, ok := m.current(); ok && it.Named() {
			m.setName("")
			m.status = "name cleared"
		}
	}
	return m, nil
}

func (m *Model) move(d int) {
	if len(m.order) > 0 {
		m.cursor = min(max(m.cursor+d, 0), len(m.order)-1)
	}
}

func (m *Model) setView(v agentView) {
	m.view = v
	m.reorder()
	m.saveState()
}

func (m *Model) openPrompt(md mode, value string) {
	m.mode = md
	m.input.SetValue(value)
	m.input.CursorEnd()
	m.input.Focus()
}

func (m Model) quit() (tea.Model, tea.Cmd) {
	m.saveState()
	return m, tea.Quit
}

func (m *Model) saveState() {
	st := store.State{View: m.view.String()}
	if it, ok := m.current(); ok {
		st.Last = key(it)
	}
	_ = m.store.SaveState(st) // losing the view preference is not worth an error
}

func (m *Model) setName(name string) {
	i, ok := m.currentIndex()
	if !ok {
		return
	}
	s := m.items[i].Session
	if err := m.store.Set(string(s.Agent), s.ID, name); err != nil {
		m.status = "could not save: " + err.Error()
		return
	}
	m.items[i].Name = name
}

func (m Model) updateFilter(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m.quit()
	case "esc":
		m.mode = modeList
		m.query = ""
		m.input.Blur()
		m.reorder()
		return m, nil
	case "enter":
		m.mode = modeList
		m.input.Blur()
		return m, nil
	case "down", "ctrl+n":
		m.move(1)
		return m, nil
	case "up", "ctrl+p":
		m.move(-1)
		return m, nil
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	if v := m.input.Value(); v != m.query {
		m.query = v
		m.reorder()
		m.cursor = 0 // best match first
	}
	return m, cmd
}

func (m Model) updatePrompt(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m.quit()
	case "esc":
		m.mode = modeList
		m.input.Blur()
		return m, nil
	case "tab":
		switch m.mode {
		case modeNewName: // 'a' would be typed into the name, so tab switches agent
			if m.newAgent == source.Claude {
				m.newAgent = source.Codex
			} else {
				m.newAgent = source.Claude
			}
		case modeNewDir:
			m.input.SetValue(completeDir(m.input.Value()))
			m.input.CursorEnd()
		}
		return m, nil
	case "enter":
		val := strings.TrimSpace(m.input.Value())
		switch m.mode {
		case modeRename:
			m.setName(val)
			if m.status == "" {
				m.status = "renamed"
				if val == "" {
					m.status = "name cleared"
				}
			}
			m.mode = modeList
			m.input.Blur()
		case modeNewName:
			m.newName = val
			m.openPrompt(modeNewDir, m.defaultDir())
		case modeNewDir:
			dir := expand(val)
			if !isDir(dir) {
				m.err = "not a directory"
				return m, nil
			}
			m.Result = &Launch{Agent: m.newAgent, Dir: dir, Name: m.newName}
			return m.quit()
		}
		return m, nil
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

// defaultDir suggests the highlighted session's folder, else where asp runs.
func (m Model) defaultDir() string {
	if it, ok := m.current(); ok {
		return collapseHome(it.Session.CWD)
	}
	if wd, err := os.Getwd(); err == nil {
		return collapseHome(wd)
	}
	return "~"
}
