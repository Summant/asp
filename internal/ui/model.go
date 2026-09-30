package ui

import (
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/summant/asp/internal/source"
	"github.com/summant/asp/internal/store"
)

type mode int

const (
	modeList mode = iota
	modeFilter
	modeInput
)

type inputKind int

const (
	inputRename inputKind = iota
	inputNewName
	inputNewDir
)

// Launch is what main does after the UI exits: either resume a session or
// start a fresh one, optionally remembering a name for it.
type Launch struct {
	Agent    source.Agent
	Dir      string
	ResumeID string // empty means "start a new session"
	Name     string // name to bind once the new session exists
}

type Model struct {
	items    []Item
	order    []int // indices into items, after filtering
	cursor   int
	top      int // first visible row, for scrolling
	mode     mode
	input    textinput.Model
	kind     inputKind
	store    *store.Store
	w, h     int
	status   string
	newName  string // carried from the name prompt to the folder prompt
	Result   *Launch
	agentFor source.Agent
}

func New(items []Item, st *store.Store) Model {
	ti := textinput.New()
	ti.Prompt = ""
	ti.CharLimit = 120
	m := Model{items: items, store: st, input: ti, agentFor: source.Claude}
	m.reorder("")
	return m
}

func (m Model) Init() tea.Cmd { return nil }

// reorder rebuilds the visible index list for a query.
func (m *Model) reorder(query string) {
	type scored struct {
		idx, score int
	}
	var hits []scored
	for i, it := range m.items {
		if s, ok := match(it.Haystack(), query); ok {
			hits = append(hits, scored{i, s})
		}
	}
	if query != "" {
		sort.SliceStable(hits, func(a, b int) bool { return hits[a].score < hits[b].score })
	}
	m.order = m.order[:0]
	for _, h := range hits {
		m.order = append(m.order, h.idx)
	}
	if m.cursor >= len(m.order) {
		m.cursor = max(0, len(m.order)-1)
	}
}

func (m Model) current() (Item, bool) {
	if m.cursor < 0 || m.cursor >= len(m.order) {
		return Item{}, false
	}
	return m.items[m.order[m.cursor]], true
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		return m, nil

	case tea.KeyMsg:
		switch m.mode {
		case modeInput:
			return m.updateInput(msg)
		case modeFilter:
			return m.updateFilter(msg)
		default:
			return m.updateList(msg)
		}
	}
	return m, nil
}

func (m Model) updateList(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "esc", "ctrl+c":
		return m, tea.Quit
	case "j", "down":
		m.moveCursor(1)
	case "k", "up":
		m.moveCursor(-1)
	case "g", "home":
		m.cursor, m.top = 0, 0
	case "G", "end":
		m.cursor = max(0, len(m.order)-1)
	case "ctrl+d", "pgdown":
		m.moveCursor(5)
	case "ctrl+u", "pgup":
		m.moveCursor(-5)
	case "/":
		m.mode = modeFilter
		m.input.SetValue("")
		m.input.Focus()
	case "enter":
		if it, ok := m.current(); ok {
			m.Result = &Launch{Agent: it.Session.Agent, Dir: it.Session.CWD, ResumeID: it.Session.ID}
			return m, tea.Quit
		}
	case "n":
		m.mode, m.kind = modeInput, inputNewName
		m.input.SetValue("")
		m.input.Focus()
	case "r":
		if it, ok := m.current(); ok {
			m.mode, m.kind = modeInput, inputRename
			m.input.SetValue(it.Name)
			m.input.Focus()
		}
	case "x":
		if it, ok := m.current(); ok && it.Named() {
			_ = m.store.Set(string(it.Session.Agent), it.Session.ID, "")
			m.items[m.order[m.cursor]].Name = ""
			m.status = "name cleared"
		}
	case "a":
		// cycle the agent used for a new session
		if m.agentFor == source.Claude {
			m.agentFor = source.Codex
		} else {
			m.agentFor = source.Claude
		}
		m.status = "new sessions will use " + m.agentFor.Label()
	}
	return m, nil
}

func (m *Model) moveCursor(d int) {
	if len(m.order) == 0 {
		return
	}
	m.cursor = clamp(m.cursor+d, 0, len(m.order)-1)
}

func (m Model) updateFilter(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = modeList
		m.input.SetValue("")
		m.reorder("")
		return m, nil
	case "enter":
		m.mode = modeList
		return m, nil
	case "ctrl+c":
		return m, tea.Quit
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	m.cursor = 0
	m.reorder(m.input.Value())
	return m, cmd
}

func (m Model) updateInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = modeList
		m.status = ""
		return m, nil
	case "ctrl+c":
		return m, tea.Quit
	case "enter":
		val := strings.TrimSpace(m.input.Value())
		switch m.kind {
		case inputRename:
			if it, ok := m.current(); ok {
				_ = m.store.Set(string(it.Session.Agent), it.Session.ID, val)
				m.items[m.order[m.cursor]].Name = val
				m.status = "renamed"
			}
			m.mode = modeList
		case inputNewName:
			m.newName = val
			m.kind = inputNewDir
			m.input.SetValue(defaultDir(m))
			m.status = ""
		case inputNewDir:
			dir := expand(val)
			if !isDir(dir) {
				m.status = "not a directory: " + val
				return m, nil
			}
			m.Result = &Launch{Agent: m.agentFor, Dir: dir, Name: m.newName}
			return m, tea.Quit
		}
		return m, nil
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

// defaultDir suggests the directory of the highlighted session.
func defaultDir(m Model) string {
	if it, ok := m.current(); ok {
		return collapseHome(it.Session.CWD)
	}
	return "~"
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
