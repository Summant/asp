package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/summant/asp/internal/source"
)

// updateAgent is the first step of a new session: which agent.
func (m Model) updateAgent(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = modeList
	case "ctrl+c":
		return m.quit(false)
	case "left", "right", "h", "l", "a", "d", "tab", "shift+tab", "up", "down", "j", "k":
		if m.newAgent == source.Claude {
			m.newAgent = source.Codex
		} else {
			m.newAgent = source.Claude
		}
	case "c":
		m.newAgent = source.Claude
	case "o":
		m.newAgent = source.Codex
	case "enter":
		m.mode = modeNewName
		m.openInput("")
	}
	return m, nil
}

func (m Model) updateFilter(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m.quit(false)
	case "esc":
		m.query = ""
		m.closeInput()
		m.reorder()
		return m, nil
	case "enter":
		m.closeInput()
		return m, nil
	case "ctrl+o":
		m.openBrowser(modeFilter, m.deps.Home)
		return m, nil
	case "tab":
		if len(m.sugg) > 0 {
			m.fillToken(m.sugg[max(0, m.suggSel)])
		}
		return m, nil
	case "down", "ctrl+n":
		if len(m.sugg) > 0 {
			m.suggSel = min(m.suggSel+1, len(m.sugg)-1)
		} else {
			m.move(1)
		}
		return m, nil
	case "up", "ctrl+p":
		if len(m.sugg) > 0 {
			m.suggSel = max(m.suggSel-1, -1)
		} else {
			m.move(-1)
		}
		return m, nil
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	m.applyQuery()
	return m, cmd
}

func (m *Model) applyQuery() {
	if v := m.input.Value(); v != m.query {
		m.query = v
		m.reorder()
		m.cursor = 0 // best match first
	}
}

// fillToken replaces the f:/g: term being typed with a chosen suggestion.
func (m *Model) fillToken(s string) {
	v := m.input.Value()
	i := strings.LastIndex(v, " ") + 1
	if len(v)-i < 2 {
		return
	}
	prefix := v[i : i+2] // "f:" or "g:"
	m.input.SetValue(v[:i] + prefix + quoteTerm(s) + " ")
	m.input.CursorEnd()
	m.applyQuery()
	m.suggSel = -1
}

func (m Model) updatePrompt(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m.quit(false)
	case "esc":
		m.closeInput()
		return m, nil
	case "down", "ctrl+n":
		if len(m.sugg) > 0 {
			m.suggSel = min(m.suggSel+1, len(m.sugg)-1)
		}
		return m, nil
	case "up", "ctrl+p":
		if len(m.sugg) > 0 {
			m.suggSel = max(m.suggSel-1, -1)
		}
		return m, nil
	case "tab":
		if len(m.sugg) > 0 {
			v := m.sugg[max(0, m.suggSel)]
			if m.mode == modeNewDir {
				v = strings.TrimSuffix(v, "/") + "/" // then its subfolders are offered
			}
			m.input.SetValue(v)
			m.input.CursorEnd()
			m.suggSel = -1
		} else if m.mode == modeNewDir {
			m.input.SetValue(completeDir(m.input.Value()))
			m.input.CursorEnd()
		}
		return m, nil
	case "ctrl+o":
		if m.mode == modeNewDir {
			start := expand(m.input.Value())
			if m.input.Value() == "" || !isDir(start) {
				start = m.deps.Home
			}
			m.openBrowser(modeNewDir, start)
		}
		return m, nil
	case "enter":
		val := strings.TrimSpace(m.input.Value())
		if m.suggSel >= 0 && m.suggSel < len(m.sugg) {
			val = m.sugg[m.suggSel]
		}
		return m.submit(val)
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m Model) submit(val string) (tea.Model, tea.Cmd) {
	switch m.mode {
	case modeRename:
		m.setName(val)
		if m.status == "" {
			m.status = "renamed"
			if val == "" {
				m.status = "name cleared"
			}
		}
		m.closeInput()
	case modeNewName:
		m.newName = val
		m.mode = modeNewDir
		m.openInput("") // empty: recent folders are listed to pick from
	case modeNewDir:
		if val == "" {
			m.err = "type or pick a folder"
			return m, nil
		}
		dir := expand(val)
		if !isDir(dir) {
			m.err = "not a directory"
			return m, nil
		}
		return m.startNew(dir)
	case modeGroupAdd:
		it, ok := m.current()
		if !ok || val == "" {
			m.closeInput()
			return m, nil
		}
		val = strings.TrimPrefix(val, "#")
		if err := m.deps.Store.AddToGroup(string(it.Session.Agent), it.Session.ID, val); err != nil {
			m.fail("could not save: %v", err)
		} else {
			m.reloadGroups()
			m.status = "added to #" + val
		}
		m.closeInput()
	case modeGroupRemove:
		it, ok := m.current()
		if !ok || val == "" {
			m.closeInput()
			return m, nil
		}
		val = strings.TrimPrefix(val, "#")
		if err := m.deps.Store.RemoveFromGroup(string(it.Session.Agent), it.Session.ID, val); err != nil {
			m.fail("could not save: %v", err)
		} else {
			m.reloadGroups()
			m.status = "removed from #" + val
		}
		m.closeInput()
	}
	return m, nil
}

func (m *Model) reloadGroups() {
	all := m.deps.Store.Groups()
	for i := range m.items {
		m.items[i].Groups = all[m.items[i].Key()]
	}
	m.reorder()
}

// updateSuggestions recomputes the suggestion list when the input changed.
func (m *Model) updateSuggestions() {
	v := m.input.Value()
	if v == m.suggFor {
		return
	}
	m.suggFor, m.suggSel = v, -1
	switch m.mode {
	case modeNewDir:
		m.sugg = m.folderSuggestions(v)
	case modeFilter:
		m.sugg = m.filterSuggestions(v)
	case modeGroupAdd:
		m.sugg = rank(m.deps.Store.GroupNames(), strings.TrimPrefix(v, "#"), maxSugg)
		if len(m.sugg) == 1 && m.sugg[0] == v {
			m.sugg = nil
		}
	case modeGroupRemove:
		if it, ok := m.current(); ok {
			m.sugg = rank(it.Groups, strings.TrimPrefix(v, "#"), maxSugg)
		}
	default:
		m.sugg = nil
	}
}

const maxSugg = 8

// filterSuggestions offers session folders while an f: term is being typed
// and group names for a g: term.
func (m Model) filterSuggestions(v string) []string {
	tok := v[strings.LastIndex(v, " ")+1:]
	switch {
	case strings.HasPrefix(tok, "f:"):
		return rank(m.sessionFolders(), strings.Trim(tok[2:], `"`), maxSugg)
	case strings.HasPrefix(tok, "g:"):
		return rank(m.deps.Store.GroupNames(), strings.Trim(tok[2:], `"`), maxSugg)
	}
	return nil
}

// sessionFolders lists the folders sessions ran in, most recent first.
func (m Model) sessionFolders() []string {
	seen := map[string]bool{}
	var out []string
	for _, it := range m.items {
		f := collapseHome(it.Session.CWD)
		if f != "" && !seen[f] {
			seen[f] = true
			out = append(out, f)
		}
	}
	return out
}

// rank filters candidates by q (all when empty), best first, keeping the
// given order among equals.
func rank(cands []string, q string, limit int) []string {
	type hit struct {
		s     string
		score int
	}
	var hits []hit
	for _, c := range cands {
		if s, ok := match(c, q); ok {
			hits = append(hits, hit{c, s})
		}
	}
	sortStable(hits, func(a, b hit) bool { return a.score < b.score })
	out := make([]string, 0, min(limit, len(hits)))
	for _, h := range hits[:min(limit, len(hits))] {
		out = append(out, h.s)
	}
	return out
}

func (m Model) promptTitle() string {
	switch m.mode {
	case modeNewDir:
		if m.input.Value() == "" {
			return "recent folders"
		}
		return "folders"
	case modeGroupAdd:
		return "groups"
	case modeGroupRemove:
		return "remove from"
	case modeFilter:
		if tok := m.input.Value()[strings.LastIndex(m.input.Value(), " ")+1:]; strings.HasPrefix(tok, "g:") {
			return "groups"
		}
		return "session folders"
	}
	return ""
}

func (m Model) agentChoice() string {
	opt := func(a source.Agent) string {
		style := tagClaude
		if a == source.Codex {
			style = tagCodex
		}
		if a == m.newAgent {
			return style.Render(markNamed + " " + a.Label())
		}
		return autoMark.Render(markAuto+" ") + helpDesc.Render(a.Label())
	}
	return fmt.Sprintf("%s   %s", opt(source.Claude), opt(source.Codex))
}
