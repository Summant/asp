package ui

import (
	"slices"
	"strings"

	"github.com/summant/asp/internal/source"
	"github.com/summant/asp/internal/store"
)

// Tabs: "all" is always first. After it come the user's tabs, each an
// agent ("agent:claude") or a group ("group:arch"), added with + and
// closed with -. They and the current tab are saved in state.json.

const maxGroupTabs = 5

const viewAll = 0

// restoreTabs reads the saved tabs, or makes the defaults: a tab for each
// agent that is installed or already has sessions.
func (m Model) restoreTabs(saved store.State) []string {
	if saved.TabList != nil {
		var out []string
		for _, t := range saved.TabList {
			if validTab(t) && !slices.Contains(out, t) {
				out = append(out, t)
			}
		}
		return out
	}
	var out []string
	for _, a := range []source.Agent{source.Claude, source.Codex} {
		if slices.Contains(m.agents(), a) || m.hasSessions(a) {
			out = append(out, "agent:"+string(a))
		}
	}
	for _, g := range saved.Tabs { // group tabs saved before agent tabs could close
		if t := "group:" + g; g != "" && !slices.Contains(out, t) {
			out = append(out, t)
		}
	}
	return out
}

func validTab(t string) bool {
	switch {
	case t == "agent:claude", t == "agent:codex":
		return true
	case strings.HasPrefix(t, "group:"):
		return len(t) > len("group:")
	}
	return false
}

func (m Model) hasSessions(a source.Agent) bool {
	for _, it := range m.items {
		if it.Session.Agent == a {
			return true
		}
	}
	return false
}

// agents lists the installed agents: those new sessions can use.
func (m Model) agents() []source.Agent {
	if m.deps.Agents == nil {
		return []source.Agent{source.Claude, source.Codex}
	}
	return m.deps.Agents
}

func (m Model) tabCount() int { return 1 + len(m.tabs) }

// tabKey is how tab v is saved: "all", "agent:claude", "group:arch".
func (m Model) tabKey(v int) string {
	if v <= 0 || v > len(m.tabs) {
		return "all"
	}
	return m.tabs[v-1]
}

// tabLabel is how tab v is shown: "all", "claude", or the group's name.
func (m Model) tabLabel(v int) string {
	k := m.tabKey(v)
	if _, rest, ok := strings.Cut(k, ":"); ok {
		return rest
	}
	return k
}

// tabGroup is the group the current tab shows, if it is a group tab.
func (m Model) tabGroup() (string, bool) {
	g, ok := strings.CutPrefix(m.tabKey(m.view), "group:")
	return g, ok
}

// tabAgent is the agent the current tab shows, if it is an agent tab.
func (m Model) tabAgent() (source.Agent, bool) {
	a, ok := strings.CutPrefix(m.tabKey(m.view), "agent:")
	return source.Agent(a), ok
}

func (m Model) groupTabCount() int {
	n := 0
	for _, t := range m.tabs {
		if strings.HasPrefix(t, "group:") {
			n++
		}
	}
	return n
}

// shows reports whether the current tab lists it.
func (m Model) shows(it Item) bool {
	if a, ok := m.tabAgent(); ok {
		return it.Session.Agent == a
	}
	g, ok := m.tabGroup()
	if !ok {
		return true // all
	}
	if it.placeholder() && it.Job != nil {
		return it.Job.Group == g // a new session started from this tab
	}
	return slices.Contains(it.Groups, g)
}

// tabChoices are what + can add: agents without a tab, then groups
// without one. Agent choices are shown with what they are.
func (m Model) tabChoices(q string) []string {
	var cands []string
	for _, a := range []source.Agent{source.Claude, source.Codex} {
		if !slices.Contains(m.tabs, "agent:"+string(a)) {
			cands = append(cands, string(a))
		}
	}
	if m.groupTabCount() < maxGroupTabs {
		for _, g := range m.deps.Store.GroupNames() {
			if !slices.Contains(m.tabs, "group:"+g) && g != "claude" && g != "codex" {
				cands = append(cands, g)
			}
		}
	}
	return rank(cands, q, maxSugg)
}

// addTab opens a tab for an agent name or a group, or switches to it if
// it is already open.
func (m *Model) addTab(name string) {
	key := "group:" + name
	if name == "claude" || name == "codex" {
		key = "agent:" + name
	}
	if i := slices.Index(m.tabs, key); i >= 0 {
		m.setView(i + 1)
		return
	}
	if strings.HasPrefix(key, "group:") && m.groupTabCount() >= maxGroupTabs {
		m.fail("%d group tabs at most; close one with %s first", maxGroupTabs, m.keys.show("list", "tab_close"))
		return
	}
	m.tabs = append(m.tabs, key)
	m.setView(m.tabCount() - 1)
	m.say("added the " + name + " tab")
}
