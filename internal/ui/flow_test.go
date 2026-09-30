package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/summant/asp/internal/jobs"
	"github.com/summant/asp/internal/source"
	"github.com/summant/asp/internal/store"
)

// fakeHost stands in for the terminal job host. Each Start or Resume runs
// the next scripted step: the job pauses or exits, and optionally a new
// session appears on "disk" (the fake reload).
type fakeHost struct {
	next    int
	jobs    []*jobs.Job
	steps   []jobs.State
	onStart func(*jobs.Job) // e.g. write a new session
	calls   []string
}

func (h *fakeHost) NewJob(a source.Agent, dir, id, name string) *jobs.Job {
	h.next++
	return &jobs.Job{N: h.next, Agent: a, Dir: dir, ResumeID: id, SessionID: id, Name: name, Started: time.Now()}
}

func (h *fakeHost) step(j *jobs.Job) {
	st := jobs.Exited
	if len(h.steps) > 0 {
		st, h.steps = h.steps[0], h.steps[1:]
	}
	j.State = st
	if st == jobs.Exited {
		for i, x := range h.jobs {
			if x == j {
				h.jobs = append(h.jobs[:i], h.jobs[i+1:]...)
			}
		}
	}
}

func (h *fakeHost) Start(j *jobs.Job) error {
	h.calls = append(h.calls, "start "+string(j.Agent)+" "+j.ResumeID+" "+j.Dir)
	h.jobs = append(h.jobs, j)
	if h.onStart != nil {
		h.onStart(j)
	}
	h.step(j)
	return nil
}

func (h *fakeHost) Resume(j *jobs.Job) error {
	h.calls = append(h.calls, "resume "+j.ResumeID)
	h.step(j)
	return nil
}

func (h *fakeHost) Paused() []*jobs.Job {
	var out []*jobs.Job
	for _, j := range h.jobs {
		if j.State == jobs.Paused {
			out = append(out, j)
		}
	}
	return out
}

// syncExec runs the job immediately instead of handing over a terminal.
func syncExec(run func() error, done func(error) tea.Msg) tea.Cmd {
	return func() tea.Msg { return done(run()) }
}

// run feeds a command's message back into the model, as bubbletea would.
func run(m Model, cmd tea.Cmd) Model {
	for cmd != nil {
		msg := cmd()
		if batch, ok := msg.(tea.BatchMsg); ok {
			for _, c := range batch {
				m = run(m, c)
			}
			return m
		}
		next, c := m.Update(msg)
		m, cmd = next.(Model), c
	}
	return m
}

func pressRun(m Model, keys ...string) Model {
	for _, k := range keys {
		next, cmd := m.Update(keyMsg(k))
		m = run(next.(Model), cmd)
	}
	return m
}

type world struct {
	st     *store.Store
	host   *fakeHost
	disk   []Item
	home   string
	model  Model
	copied string
}

func newWorld(t *testing.T, items []Item) *world {
	t.Helper()
	w := &world{st: testStore(t), host: &fakeHost{}, disk: items, home: t.TempDir()}
	reload := func() []Item {
		out := make([]Item, len(w.disk))
		copy(out, w.disk)
		groups := w.st.Groups()
		for i := range out {
			if n, ok := w.st.Get(string(out[i].Session.Agent), out[i].Session.ID); ok {
				out[i].Name = n
			}
			out[i].Groups = groups[out[i].Key()]
		}
		return out
	}
	w.model = sized(New(reload(), Deps{
		Store: w.st, Reload: reload, Host: w.host, Exec: syncExec, Home: w.home,
		Copy: func(s string) error { w.copied = s; return nil },
	}), 120, 30)
	return w
}

func footer(m Model) string { return strings.TrimSpace(plainLines(m)[len(plainLines(m))-1]) }

func TestPauseAndGoBack(t *testing.T) {
	w := newWorld(t, testItems())
	w.host.steps = []jobs.State{jobs.Paused, jobs.Exited}
	it := testItems()[1] // codex

	m := pressRun(w.model, "j", "enter")
	if len(w.host.calls) != 1 || w.host.calls[0] != "start codex "+it.Session.ID+" "+it.Session.CWD {
		t.Fatalf("calls %v", w.host.calls)
	}
	cur, _ := m.current()
	if !cur.paused() || cur.Session.ID != it.Session.ID {
		t.Fatalf("selected %q paused=%v after pausing", cur.Session.ID, cur.paused())
	}
	out := ansi.Strip(m.View())
	if !strings.Contains(out, "1 paused") || !strings.Contains(out, "·  paused") || !strings.Contains(footer(m), "↵ goes back") {
		t.Errorf("paused session not shown:\n%s", out)
	}

	// Quitting with a paused session asks first.
	m = pressRun(m, "q")
	if !strings.Contains(footer(m), "1 paused session will end") {
		t.Errorf("footer %q", footer(m))
	}
	m = pressRun(m, "j", "k") // any other key cancels the quit

	m = pressRun(m, "enter")
	if w.host.calls[1] != "resume "+it.Session.ID {
		t.Errorf("enter on a paused session should continue it, calls %v", w.host.calls)
	}
	if strings.Contains(ansi.Strip(m.View()), "paused") || footer(m) != "session ended" {
		t.Errorf("after exit footer %q", footer(m))
	}
}

func TestNewSessionFlowNamesByIDDiff(t *testing.T) {
	w := newWorld(t, testItems())
	dir := filepath.Join(w.home, "proj")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(time.Hour)
	w.host.onStart = func(j *jobs.Job) {
		// Resuming bumps an existing session's mtime in the same folder;
		// the new one must still be found by id, not by recency.
		w.disk = append(w.disk,
			Item{Session: source.Session{ID: "brand-new", Agent: source.Codex, CWD: dir, Modified: time.Now()}},
		)
		w.disk[1].Session.CWD, w.disk[1].Session.Modified = dir, old
	}
	w.host.steps = []jobs.State{jobs.Exited}

	m := pressRun(w.model, "d", "d", "n") // codex view preselects codex
	if m.mode != modeNewAgent || m.newAgent != source.Codex || !strings.Contains(footer(m), "● codex") {
		t.Fatalf("agent step: mode %v agent %v footer %q", m.mode, m.newAgent, footer(m))
	}
	m = pressRun(m, "right") // to claude
	m = pressRun(m, "left")  // and back
	m = pressRun(m, "enter")
	m = typeText(m, "Waybar redesign")
	m = pressRun(m, "enter")
	if m.mode != modeNewDir || m.input.Value() != "" {
		t.Fatalf("folder step: mode %v value %q", m.mode, m.input.Value())
	}
	if !strings.Contains(ansi.Strip(m.View()), "recent folders") {
		t.Error("empty folder prompt should list recent folders")
	}
	m = pressRun(m, "enter")
	if !strings.Contains(footer(m), "type or pick a folder") {
		t.Errorf("empty folder accepted: %q", footer(m))
	}
	m.input.SetValue(filepath.Join(dir, "missing"))
	m = pressRun(m, "enter")
	if !strings.Contains(footer(m), "not a directory") {
		t.Errorf("missing folder accepted: %q", footer(m))
	}
	m.input.SetValue(dir)
	m = pressRun(m, "enter")
	if len(w.host.calls) != 1 || w.host.calls[0] != "start codex  "+dir {
		t.Fatalf("calls %v", w.host.calls)
	}
	if n, _ := w.st.Get("codex", "brand-new"); n != "Waybar redesign" {
		t.Errorf("new session named %q", n)
	}
	if n, ok := w.st.Get("codex", testItems()[1].Session.ID); ok {
		t.Errorf("bumped old session got the name %q", n)
	}
	if cur, _ := m.current(); cur.Session.ID != "brand-new" {
		t.Errorf("selected %q after the new session ended", cur.Session.ID)
	}
}

func TestPausedNewSessionShowsPlaceholderUntilSaved(t *testing.T) {
	w := newWorld(t, testItems())
	w.host.steps = []jobs.State{jobs.Paused, jobs.Paused}
	m := pressRun(w.model, "n", "enter")
	m = typeText(m, "Later")
	m = pressRun(m, "enter")
	m.input.SetValue(w.home)
	m = pressRun(m, "enter")

	cur, _ := m.current()
	if !cur.placeholder() || cur.Title() != "Later" || !cur.paused() {
		t.Fatalf("placeholder %+v", cur)
	}
	if l := plainLines(m)[3]; !strings.Contains(l, "Later") {
		t.Errorf("placeholder not at the top: %q", l)
	}
	// The user comes back, types a message, pauses again: now it exists.
	w.disk = append(w.disk, Item{Session: source.Session{ID: "late", Agent: source.Claude, CWD: w.home, Modified: time.Now()}})
	m = pressRun(m, "enter")
	if w.host.calls[1] != "resume " {
		t.Errorf("calls %v", w.host.calls)
	}
	if n, _ := w.st.Get("claude", "late"); n != "Later" {
		t.Errorf("name %q", n)
	}
	cur, _ = m.current()
	if cur.placeholder() || cur.Session.ID != "late" || !cur.paused() {
		t.Errorf("after save: %+v", cur)
	}
}

func TestGroupsAndPrefixedFilter(t *testing.T) {
	w := newWorld(t, testItems())
	m := pressRun(w.model, "b")
	m = typeText(m, "arch")
	m = pressRun(m, "enter")
	if footer(m) != "added to #arch" {
		t.Errorf("footer %q", footer(m))
	}
	m = pressRun(m, "j", "j", "b")
	if len(m.sugg) != 1 || m.sugg[0] != "arch" {
		t.Errorf("existing groups not suggested: %v", m.sugg)
	}
	m = pressRun(m, "down", "enter")
	m = pressRun(m, "j", "b")
	m = typeText(m, "waybar")
	m = pressRun(m, "enter")

	m = pressRun(m, "/")
	m = typeText(m, "g:arc")
	if len(m.sugg) != 1 || m.sugg[0] != "arch" {
		t.Errorf("g: suggestions %v", m.sugg)
	}
	if len(m.order) != 2 {
		t.Errorf("g:arc matched %d, want 2", len(m.order))
	}
	m = pressRun(m, "tab")
	if m.input.Value() != "g:arch " {
		t.Errorf("tab filled %q", m.input.Value())
	}
	m = typeText(m, "f:/tmp")
	if len(m.order) != 1 {
		t.Errorf("g:arch f:/tmp matched %d, want 1", len(m.order))
	}
	m = pressRun(m, "esc")

	// Remove from a group; the emptied group disappears.
	m = pressRun(m, "g", "j", "j", "j", "B")
	if len(m.sugg) != 1 || m.sugg[0] != "waybar" {
		t.Fatalf("remove suggestions %v", m.sugg)
	}
	m = pressRun(m, "down", "enter")
	for _, g := range w.st.GroupNames() {
		if g == "waybar" {
			t.Error("emptied group still exists")
		}
	}
	if !strings.Contains(ansi.Strip(m.View()), "#arch") {
		t.Error("group not shown on the item")
	}
}

func TestFilterByQuotedFolder(t *testing.T) {
	w := newWorld(t, testItems())
	m := pressRun(w.model, "/")
	m = typeText(m, `f:"arch ricing"`)
	if len(m.order) != 1 || !strings.Contains(m.items[m.order[0]].Session.CWD, "Arch Ricing") {
		t.Errorf("quoted folder matched %d", len(m.order))
	}
	m.input.SetValue("")
	m = typeText(m, "f:agentsess")
	if len(m.sugg) == 0 || !strings.HasSuffix(m.sugg[0], "AgentSessionPicker") {
		t.Errorf("f: suggestions %v", m.sugg)
	}
}

func TestFolderSuggestionsAndBrowser(t *testing.T) {
	w := newWorld(t, testItems())
	for _, d := range []string{"dotfiles/.config/waybar", "dotfiles/.config/kitty", "projects/asp", "projects/web"} {
		if err := os.MkdirAll(filepath.Join(w.home, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(w.home, "projects", "notes.txt"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	m := pressRun(w.model, "n", "enter", "enter") // claude, no name → folder
	if m.folders == nil {
		t.Fatal("folder index was not built")
	}
	m = typeText(m, "dotwayb")
	if len(m.sugg) == 0 || !strings.HasSuffix(m.sugg[0], "dotfiles/.config/waybar") {
		t.Errorf("fuzzy suggestions %v", m.sugg)
	}
	m.input.SetValue(w.home + "/pro")
	m.input.CursorEnd()
	m = typeText(m, "j")
	m = pressRun(m, "tab") // first suggestion filled in
	if m.input.Value() != collapseHome(w.home+"/projects")+"/" {
		t.Errorf("tab filled %q", m.input.Value())
	}
	m = typeText(m, "a")
	if len(m.sugg) != 1 || !strings.HasSuffix(m.sugg[0], "projects/asp") {
		t.Errorf("child suggestions %v", m.sugg)
	}

	// ctrl+o opens the finder scoped to the typed folder.
	m = pressRun(m, "backspace", "ctrl+o")
	if m.mode != modeFind || m.find.scope != filepath.Join(w.home, "projects") {
		t.Fatalf("finder mode %v scope %q", m.mode, m.find.scope)
	}
	out := ansi.Strip(m.View())
	for _, want := range []string{"./  (this folder)", "asp/", "web/", "notes.txt"} {
		if !strings.Contains(out, want) {
			t.Errorf("finder missing %q:\n%s", want, out)
		}
	}
	m = pressRun(m, "left") // widen to home: every folder, fuzzily
	m = typeText(m, "dotwayb")
	if h, _ := m.findSelected(); h.path != filepath.Join(w.home, "dotfiles/.config/waybar") {
		t.Errorf("best match %q", h.path)
	}
	// The best match sits at the bottom, next to the prompt.
	lines := plainLines(m)
	if !strings.Contains(lines[len(lines)-3], "dotfiles/.config/waybar/") {
		t.Errorf("best match not above the prompt: %q", lines[len(lines)-3])
	}
	m = pressRun(m, "enter")
	if m.mode != modeNewDir || m.input.Value() != collapseHome(filepath.Join(w.home, "dotfiles/.config/waybar")) {
		t.Errorf("chosen: mode %v value %q", m.mode, m.input.Value())
	}
}

func TestFinderFiltersTheList(t *testing.T) {
	items := testItems()
	w := newWorld(t, items)
	proj := filepath.Join(w.home, "work", "proj")
	if err := os.MkdirAll(proj, 0o755); err != nil {
		t.Fatal(err)
	}
	w.disk[2].Session.CWD = proj
	m := pressRun(w.model, "f")
	if m.mode != modeFind || m.find.scope != w.home {
		t.Fatalf("f: mode %v scope %q", m.mode, m.find.scope)
	}
	m = typeText(m, "wp")
	m = pressRun(m, "enter")
	if m.mode != modeList || m.query != "f:"+collapseHome(proj) {
		t.Fatalf("mode %v query %q", m.mode, m.query)
	}
	// The key hints stay; the filter is shown in the header.
	if !strings.Contains(footer(m), "↵ open") {
		t.Errorf("footer lost its keys: %q", footer(m))
	}
	if !strings.Contains(plainLines(m)[1], "filter: f:") {
		t.Errorf("header %q", plainLines(m)[1])
	}
}

func TestFilterPrefixHints(t *testing.T) {
	w := newWorld(t, testItems())
	m := pressRun(w.model, "/")
	out := ansi.Strip(m.View())
	if !strings.Contains(out, "f:  filters file paths") || !strings.Contains(out, "g:  filters groups") {
		t.Errorf("prefix hints missing:\n%s", out)
	}
	m = typeText(m, "g")
	if len(m.sugg) != 1 || m.sugg[0] != "g:" {
		t.Errorf("after g: %v", m.sugg)
	}
	m = pressRun(m, "tab")
	if m.input.Value() != "g:" {
		t.Errorf("tab filled %q", m.input.Value())
	}
	m = pressRun(m, "backspace", "backspace", "down", "enter") // pick f: from the list
	if m.input.Value() != "f:" || m.mode != modeFilter {
		t.Errorf("enter on hint: %q mode %v", m.input.Value(), m.mode)
	}
}

func TestReadCopyHelp(t *testing.T) {
	items := testItems()
	items[0].Session.Opening = "Read /x/[AGENTS.md](http://AGENTS.md) " + strings.Repeat("word ", 200) + "END"
	w := newWorld(t, items)
	m := pressRun(w.model, "y")
	if w.copied != items[0].Session.Opening || footer(m) != "copied the opening message" {
		t.Errorf("copied %q, footer %q", w.copied[:20], footer(m))
	}
	m = pressRun(m, "v")
	out := ansi.Strip(m.View())
	if strings.Contains(out, "│") {
		t.Error("details view should be a single column, with no rule to catch in a selection")
	}
	for _, want := range []string{"agent", "folder   /tmp", "size", "name", "id       " + items[0].Session.ID} {
		if !strings.Contains(out, want) {
			t.Errorf("details view missing %q", want)
		}
	}
	m = pressRun(m, "G")
	if !strings.Contains(ansi.Strip(m.View()), "END") {
		t.Error("reader cannot scroll to the end of the message")
	}
	m = pressRun(m, "esc", "?")
	if !strings.Contains(ansi.Strip(m.View()), "ctrl+z") {
		t.Error("help does not explain ctrl+z")
	}
	m = pressRun(m, "x")
	if m.mode != modeList {
		t.Error("any key should close help")
	}
}
