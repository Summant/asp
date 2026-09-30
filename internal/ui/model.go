package ui

import (
	"fmt"
	"io"
	"os"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/summant/asp/internal/jobs"
	"github.com/summant/asp/internal/source"
	"github.com/summant/asp/internal/store"
)

// Host runs agents on the terminal; see package jobs.
type Host interface {
	NewJob(agent source.Agent, dir, resumeID, name string) *jobs.Job
	Start(*jobs.Job) error  // blocks until the agent pauses or exits
	Resume(*jobs.Job) error // likewise
	End(*jobs.Job) error    // finish a paused one
	Paused() []*jobs.Job
}

// Deps is what the model needs from outside. Only Store is required.
type Deps struct {
	Store  *store.Store
	Reload func() []Item // rescan sessions; nil keeps the initial list
	Host   Host          // nil: sessions cannot be opened
	// Exec hands the terminal to run and reports back via done. Default:
	// tea.Exec, which leaves the alt screen for the duration.
	Exec func(run func() error, done func(error) tea.Msg) tea.Cmd
	Copy func(string) error // clipboard
	Home string             // where the folder finder starts; default $HOME
	// Keys maps section → action → keys, as config.Config.Keys; nil uses
	// the defaults.
	Keys map[string]map[string][]string
	// Agents are the agents installed here, whose sessions can be started;
	// nil means both.
	Agents []source.Agent
}

type mode int

const (
	modeList mode = iota
	modeFilter
	modeRename
	modeNewAgent
	modeNewName
	modeNewDir
	modeGroupAdd
	modeGroupRemove
	modeTabAdd
	modeGroupColor
	modeGroupRecolor
	modeFind
	modeRead
	modeHelp
)

type Model struct {
	items  []Item
	order  []int    // indices into items: in the current view and matching query
	cursor int      // index into order; the page is derived from it
	view   int      // current tab: 0 is "all", i is tabs[i-1]
	tabs   []string // every tab after "all": "agent:claude", "group:arch", …

	groupColors map[string]string // group → "#rrggbb"; missing ones are gray
	colorFor    string            // group the colour prompt is for
	query       string            // active filter, live while typing

	mode        mode
	input       textinput.Model
	newName     string       // carried from the name prompt to the folder prompt
	newAgent    source.Agent // agent a new session will use
	newGroup    string       // group a new session joins: the tab it was started from
	status      string       // confirmation or error; fades after statusFor
	statusBad   bool         // status is an error
	statusID    int          // which status a pending fade is for
	err         string       // prompt validation error; cleared on the next keypress
	confirmQuit bool         // q pressed once with paused sessions

	sugg    []string // suggestions for the active prompt, best first
	suggSel int      // highlighted suggestion; -1 when none
	suggFor string   // input value sugg was computed for

	find   finder
	scroll int // reader scroll offset

	deps Deps
	keys keymap
	w, h int
}

type jobMsg struct {
	job *jobs.Job
	err error
}

func New(items []Item, d Deps) Model {
	if d.Exec == nil {
		d.Exec = func(run func() error, done func(error) tea.Msg) tea.Cmd {
			return tea.Exec(execFunc(run), done)
		}
	}
	if d.Home == "" {
		d.Home, _ = os.UserHomeDir()
	}
	ti := textinput.New()
	ti.Prompt = ""
	ti.CharLimit = 300
	ti.Cursor.Style = promptLabel
	ti.TextStyle = fg(text)

	saved := d.Store.State()
	m := Model{items: items, deps: d, keys: newKeymap(d.Keys), input: ti, suggSel: -1}
	m.groupColors = d.Store.GroupColors()
	m.tabs = m.restoreTabs(saved)
	for v := range m.tabCount() {
		if m.tabKey(v) == saved.View || (saved.View == m.tabLabel(v) && v > 0 && strings.HasPrefix(m.tabKey(v), "agent:")) {
			m.view = v // reopen on the tab asp was closed on
		}
	}
	m.attachJobs()
	m.reorder()
	m.selectKey(saved.Last)
	return m
}

// execFunc adapts a function to tea.ExecCommand. The agent uses the real
// terminal directly, so the redirections bubbletea offers are ignored.
type execFunc func() error

func (f execFunc) Run() error        { return f() }
func (execFunc) SetStdin(io.Reader)  {}
func (execFunc) SetStdout(io.Writer) {}
func (execFunc) SetStderr(io.Writer) {}

func (m Model) Init() tea.Cmd { return nil }

// reorder rebuilds the visible list for the current view and query, keeping
// the selected session selected if it is still listed.
func (m *Model) reorder() {
	prev := ""
	if it, ok := m.current(); ok {
		prev = it.Key()
	}
	q := parseQuery(m.query)
	type hit struct{ idx, score int }
	var hits []hit
	for i, it := range m.items {
		if !m.shows(it) {
			continue
		}
		if s, ok := matchItem(it, q); ok {
			hits = append(hits, hit{i, s})
		}
	}
	sort.SliceStable(hits, func(a, b int) bool { return hits[a].score < hits[b].score })
	m.order = m.order[:0]
	for _, h := range hits {
		m.order = append(m.order, h.idx)
	}
	m.cursor = 0
	m.selectKey(prev)
}

func (m *Model) selectKey(k string) {
	for i, idx := range m.order {
		if m.items[idx].Key() == k {
			m.cursor = i
			return
		}
	}
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

// refresh rescans sessions and re-attaches paused jobs.
func (m *Model) refresh() {
	if m.deps.Reload != nil {
		m.items = m.deps.Reload()
	}
	m.groupColors = m.deps.Store.GroupColors()
	if m.deps.Host != nil {
		for _, j := range m.deps.Host.Paused() {
			m.resolve(j)
		}
	}
	m.attachJobs()
	m.reorder()
}

// attachJobs links paused jobs to their sessions. A new session that has
// not reached the disk yet is shown as a placeholder at the top.
func (m *Model) attachJobs() {
	m.items = slices.DeleteFunc(m.items, func(it Item) bool { return it.placeholder() })
	for i := range m.items {
		m.items[i].Job = nil
	}
	if m.deps.Host == nil {
		return
	}
	var placeholders []Item
	for _, j := range m.deps.Host.Paused() {
		if j.SessionID != "" {
			if i := m.indexOf(string(j.Agent) + ":" + j.SessionID); i >= 0 {
				m.items[i].Job = j
				continue
			}
		}
		placeholders = append(placeholders, Item{
			Session: source.Session{Agent: j.Agent, CWD: j.Dir, Modified: j.Started},
			Name:    j.Name,
			Job:     j,
		})
	}
	m.items = append(placeholders, m.items...)
}

func (m Model) indexOf(key string) int {
	for i, it := range m.items {
		if it.Key() == key {
			return i
		}
	}
	return -1
}

// resolve finds the session a new job created: an id absent before the
// launch, in the launch folder, newest if several. Not by mtime — resuming
// an old session in the same folder bumps that too.
func (m *Model) resolve(j *jobs.Job) {
	if j.SessionID != "" || j.ResumeID != "" {
		return
	}
	var found *Item
	for i := range m.items {
		it := &m.items[i]
		if it.placeholder() || it.Session.Agent != j.Agent || j.Before[it.Session.ID] || !samePath(it.Session.CWD, j.Dir) {
			continue
		}
		if found == nil || it.Session.Modified.After(found.Session.Modified) {
			found = it
		}
	}
	if found == nil {
		return
	}
	j.SessionID = found.Session.ID
	if j.Name != "" {
		if err := m.deps.Store.Set(string(j.Agent), j.SessionID, j.Name); err == nil {
			found.Name = j.Name
		}
	}
	if j.Group != "" {
		if err := m.deps.Store.AddToGroup(string(j.Agent), j.SessionID, j.Group); err == nil && !slices.Contains(found.Groups, j.Group) {
			found.Groups = append(found.Groups, j.Group)
		}
	}
}

// statusFor is how long a status message stays up.
const statusFor = 4 * time.Second

// clearStatusMsg retires the status message it was scheduled for; a newer
// message has a newer id and stays.
type clearStatusMsg int

// Update handles a message, then schedules any new status message to fade.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if id, ok := msg.(clearStatusMsg); ok {
		if int(id) == m.statusID && !m.confirmQuit {
			m.status, m.statusBad = "", false
		}
		return m, nil
	}
	before := m.status
	next, cmd := m.update(msg)
	nm := next.(Model)
	if nm.status != "" && nm.status != before && !nm.confirmQuit {
		nm.statusID++
		id := nm.statusID
		cmd = tea.Batch(cmd, tea.Tick(statusFor, func(time.Time) tea.Msg { return clearStatusMsg(id) }))
	}
	return nm, cmd
}

func (m Model) update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		return m, nil
	case jobMsg:
		return m.jobDone(msg)
	case tea.KeyMsg:
		m.err = ""
		confirm := m.confirmQuit
		if confirm {
			m.status, m.confirmQuit = "", false // answered, one way or the other
		}
		var cmd tea.Cmd
		var next tea.Model
		switch m.mode {
		case modeFilter:
			next, cmd = m.updateFilter(msg)
		case modeRename, modeNewName, modeNewDir, modeGroupAdd, modeGroupRemove, modeTabAdd, modeGroupColor, modeGroupRecolor:
			next, cmd = m.updatePrompt(msg)
		case modeNewAgent:
			next, cmd = m.updateAgent(msg)
		case modeFind:
			next, cmd = m.updateFind(msg)
		case modeRead, modeHelp:
			next, cmd = m.updateRead(msg)
		default:
			next, cmd = m.updateList(msg, confirm)
		}
		nm := next.(Model)
		nm.updateSuggestions()
		return nm, cmd
	}
	return m, nil
}

func (m Model) updateList(msg tea.KeyMsg, confirm bool) (tea.Model, tea.Cmd) {
	if msg.String() == "ctrl+c" {
		return m.quit(confirm)
	}
	per := m.perPage()
	switch m.keys.action("list", msg) {
	case "quit":
		return m.quit(confirm)
	case "back": // clears a filter; never quits (q and ctrl+c do)
		if m.query != "" {
			m.query = ""
			m.reorder()
		}
	case "down":
		m.move(1)
	case "up":
		m.move(-1)
	case "next_page":
		if next := (m.cursor/per + 1) * per; next < len(m.order) {
			m.cursor = next
		}
	case "prev_page":
		m.cursor = max(0, (m.cursor/per-1)*per)
	case "first":
		m.cursor = 0
	case "last":
		m.cursor = max(0, len(m.order)-1)
	case "view_next":
		m.setView((m.view + 1) % m.tabCount())
	case "view_prev":
		m.setView((m.view + m.tabCount() - 1) % m.tabCount())
	case "tab_add":
		m.mode = modeTabAdd
		m.openInput("")
	case "tab_close":
		if m.view == 0 {
			m.say("the all tab stays")
			break
		}
		label := m.tabLabel(m.view)
		m.tabs = slices.Delete(m.tabs, m.view-1, m.view)
		m.setView(min(m.view, m.tabCount()-1))
		m.say("closed the " + label + " tab")
	case "end":
		return m.end()
	case "filter":
		m.mode = modeFilter
		m.query = ""
		m.reorder()
		m.openInput("")
	case "open":
		return m.open()
	case "new":
		agents := m.agents()
		if len(agents) == 0 {
			m.fail("neither claude nor codex is installed (not found on your PATH)")
			break
		}
		m.newAgent = agents[0]
		if a, ok := m.tabAgent(); ok && slices.Contains(agents, a) {
			m.newAgent = a
		}
		m.newGroup, _ = m.tabGroup() // a new session from a group tab joins that group
		m.mode = modeNewAgent
		if len(agents) == 1 { // nothing to choose
			m.mode = modeNewName
			m.openInput("")
		}
	case "rename":
		if it, ok := m.current(); ok && !it.placeholder() {
			m.mode = modeRename
			m.openInput(it.Name)
		}
	case "unname":
		if it, ok := m.current(); ok && it.Named() && !it.placeholder() {
			if m.setName("") {
				m.say("name cleared")
			}
		}
	case "folders":
		m.openFinder(modeList, m.deps.Home)
	case "group_add":
		if it, ok := m.current(); ok && !it.placeholder() {
			m.mode = modeGroupAdd
			m.openInput("")
		}
	case "group_color":
		if it, ok := m.current(); ok && !it.placeholder() {
			switch len(it.Groups) {
			case 0:
				m.say("not in any group")
			case 1:
				m.askColor(it.Groups[0])
			default:
				m.mode = modeGroupRecolor
				m.openInput("")
			}
		}
	case "group_remove":
		if it, ok := m.current(); ok && !it.placeholder() {
			if len(it.Groups) == 0 {
				m.say("not in any group")
				break
			}
			m.mode = modeGroupRemove
			m.openInput("")
		}
	case "copy":
		m.copyOpening()
	case "details":
		if _, ok := m.current(); ok {
			m.mode, m.scroll = modeRead, 0
		}
	case "help":
		m.mode, m.scroll = modeHelp, 0
	}
	return m, nil
}

func (m *Model) move(d int) {
	if len(m.order) > 0 {
		m.cursor = min(max(m.cursor+d, 0), len(m.order)-1)
	}
}

func (m *Model) setView(v int) {
	m.view = v
	m.reorder()
	m.saveState()
}

func (m *Model) openInput(value string) {
	m.input.SetValue(value)
	m.input.CursorEnd()
	m.input.Focus()
	m.sugg, m.suggSel, m.suggFor = nil, -1, "\x00"
}

func (m *Model) closeInput() {
	m.mode = modeList
	m.input.Blur()
	m.sugg, m.suggSel = nil, -1
}

// quit asks for confirmation first when sessions are paused, since quitting
// ends them.
func (m Model) quit(confirmed bool) (tea.Model, tea.Cmd) {
	if n := m.pausedCount(); n > 0 && !confirmed {
		m.confirmQuit = true
		m.say(fmt.Sprintf("%d paused session%s will end  %s  %s again to quit", n, plural(n), sep, m.keys.show("list", "quit")))
		return m, nil
	}
	m.saveState()
	return m, tea.Quit
}

func (m Model) pausedCount() int {
	if m.deps.Host == nil {
		return 0
	}
	return len(m.deps.Host.Paused())
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

func (m *Model) saveState() {
	st := store.State{View: m.tabKey(m.view), TabList: append([]string{}, m.tabs...)}
	if it, ok := m.current(); ok && !it.placeholder() {
		st.Last = it.Key()
	}
	_ = m.deps.Store.SaveState(st) // losing the view preference is not worth an error
}

// say shows a confirmation in the status row.
func (m *Model) say(s string) { m.status, m.statusBad = s, false }

func (m *Model) fail(format string, a ...any) {
	m.status, m.statusBad = fmt.Sprintf(format, a...), true
}

// setName saves the selected session's name, reporting whether it worked.
func (m *Model) setName(name string) bool {
	i, ok := m.currentIndex()
	if !ok {
		return false
	}
	s := m.items[i].Session
	if err := m.deps.Store.Set(string(s.Agent), s.ID, name); err != nil {
		m.fail("could not save: %v", err)
		return false
	}
	m.items[i].Name = name
	return true
}

func (m *Model) copyOpening() {
	it, ok := m.current()
	if !ok || it.Session.Opening == "" {
		m.say("nothing to copy")
		return
	}
	if m.deps.Copy == nil {
		m.fail("no clipboard available")
		return
	}
	if err := m.deps.Copy(it.Session.Opening); err != nil {
		m.fail("copy failed: %v", err)
		return
	}
	m.say("copied the opening message")
}

// open resumes the selected session: continues its paused process, or
// starts the agent with its resume command.
func (m Model) open() (tea.Model, tea.Cmd) {
	it, ok := m.current()
	if !ok {
		return m, nil
	}
	if m.deps.Host == nil {
		m.fail("cannot run agents here")
		return m, nil
	}
	m.saveState()
	if it.paused() {
		j := it.Job
		return m, m.deps.Exec(func() error { return m.deps.Host.Resume(j) }, func(err error) tea.Msg { return jobMsg{j, err} })
	}
	if !isDir(it.Session.CWD) {
		m.fail("folder no longer exists: %s", collapseHome(it.Session.CWD))
		return m, nil
	}
	j := m.deps.Host.NewJob(it.Session.Agent, it.Session.CWD, it.Session.ID, "")
	return m, m.deps.Exec(func() error { return m.deps.Host.Start(j) }, func(err error) tea.Msg { return jobMsg{j, err} })
}

// end finishes the selected paused session without going into it. The
// agent gets the terminal for its goodbye, so the UI steps aside for it.
func (m Model) end() (tea.Model, tea.Cmd) {
	it, ok := m.current()
	if !ok || !it.paused() {
		m.say("only a paused session can be ended here")
		return m, nil
	}
	j := it.Job
	return m, m.deps.Exec(func() error { return m.deps.Host.End(j) }, func(err error) tea.Msg { return jobMsg{j, err} })
}

// startNew launches a new session in dir, remembering which of the agent's
// sessions already existed so the new one can be found and named.
func (m Model) startNew(dir string) (tea.Model, tea.Cmd) {
	m.closeInput()
	if m.deps.Host == nil {
		m.fail("cannot run agents here")
		return m, nil
	}
	m.refresh()
	before := map[string]bool{}
	for _, it := range m.items {
		if it.Session.Agent == m.newAgent && !it.placeholder() {
			before[it.Session.ID] = true
		}
	}
	j := m.deps.Host.NewJob(m.newAgent, dir, "", m.newName)
	j.Before = before
	j.Group = m.newGroup
	return m, m.deps.Exec(func() error { return m.deps.Host.Start(j) }, func(err error) tea.Msg { return jobMsg{j, err} })
}

// jobDone runs when an agent hands the terminal back.
func (m Model) jobDone(msg jobMsg) (tea.Model, tea.Cmd) {
	j := msg.job
	if m.deps.Reload != nil {
		m.items = m.deps.Reload()
	}
	m.resolve(j)
	m.attachJobs()
	m.reorder()
	switch {
	case j.SessionID != "":
		m.selectKey(string(j.Agent) + ":" + j.SessionID)
	case j.State == jobs.Paused:
		m.selectKey(fmt.Sprintf("job:%d", j.N))
	}
	switch {
	case msg.err != nil:
		m.fail("%s: %v", j.Agent, msg.err)
	case j.State == jobs.Paused:
		m.say("paused  " + sep + "  " + m.keys.show("list", "open") + " goes back to it")
	case j.Name != "" && j.SessionID == "":
		m.fail("no new %s session was saved, so the name %q was not kept", j.Agent, j.Name)
	default:
		m.say("session ended")
	}
	return m, nil
}

func (m Model) updateRead(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "ctrl+c" {
		return m.quit(false)
	}
	switch m.keys.action("details", msg) {
	case "back":
		m.mode = modeList
	case "down":
		m.scroll++
	case "up":
		m.scroll--
	case "page_down":
		m.scroll += max(1, m.bodyRows()-2)
	case "page_up":
		m.scroll -= max(1, m.bodyRows()-2)
	case "top":
		m.scroll = 0
	case "bottom":
		m.scroll = 1 << 30
	case "copy":
		if m.mode == modeRead {
			m.copyOpening()
		}
	}
	m.scroll = min(max(0, m.scroll), max(0, len(m.pageLines())-m.bodyRows()-1))
	return m, nil
}
