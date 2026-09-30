package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
	"github.com/summant/asp/internal/config"
	"github.com/summant/asp/internal/jobs"
	"github.com/summant/asp/internal/source"
)

func header(m Model) string { return plainLines(m)[1] }

func TestGroupTabs(t *testing.T) {
	w := newWorld(t, testItems())
	for i, g := range map[int]string{0: "arch", 3: "arch", 5: "wm"} {
		it := w.disk[i]
		if err := w.st.AddToGroup(string(it.Session.Agent), it.Session.ID, g); err != nil {
			t.Fatal(err)
		}
	}
	m := w.model
	m.reloadGroups() // the groups were written to the store behind asp's back

	m = pressRun(m, "+")
	if m.mode != modeTabAdd || len(m.sugg) != 2 {
		t.Fatalf("+ opened mode %v with suggestions %v", m.mode, m.sugg)
	}
	m = typeText(m, "arch")
	m = pressRun(m, "enter")
	if g, ok := m.tabGroup(); !ok || g != "arch" {
		t.Fatalf("not on the arch tab: %v", m.view)
	}
	if len(m.order) != 2 {
		t.Errorf("arch tab lists %d sessions, want 2", len(m.order))
	}
	h := header(m)
	if !strings.Contains(h, "arch 2") || strings.Contains(h, "#arch") {
		t.Errorf("header %q: want a plain arch tab", h)
	}
	if !strings.Contains(h, "+") {
		t.Errorf("header %q lacks +", h)
	}

	// The + prompt only offers groups without a tab.
	m = pressRun(m, "+")
	if len(m.sugg) != 1 || m.sugg[0] != "wm" {
		t.Errorf("suggestions %v", m.sugg)
	}
	m = pressRun(m, "down", "enter")
	if g, _ := m.tabGroup(); g != "wm" || m.tabCount() != 5 {
		t.Errorf("tab %q, %d tabs", g, m.tabCount())
	}

	// ←/→ cycle through every tab and wrap.
	m = pressRun(m, "right")
	if m.view != viewAll {
		t.Errorf("→ from the last tab should wrap to all, got %d", m.view)
	}
	m = pressRun(m, "left")
	if g, _ := m.tabGroup(); g != "wm" {
		t.Errorf("← from all should reach the last tab, got %q", g)
	}

	// Reopening asp restores the tabs and the one it was on.
	m = pressRun(m, "q")
	again := New(w.disk, Deps{Store: w.st})
	if len(again.groupTabs) != 2 || again.groupTabs[0] != "arch" {
		t.Errorf("tabs not restored: %v", again.groupTabs)
	}
	if g, _ := again.tabGroup(); g != "wm" {
		t.Errorf("reopened on tab %d, want wm", again.view)
	}

	// - closes a group tab, not a fixed one.
	m = pressRun(m, "-")
	if len(m.groupTabs) != 1 || m.groupTabs[0] != "arch" {
		t.Errorf("after -: %v", m.groupTabs)
	}
	m = pressRun(m, "home")
	m.view = viewClaude
	m = pressRun(m, "-")
	if len(m.groupTabs) != 1 || !strings.Contains(status(m), "only group tabs") {
		t.Errorf("- on claude: tabs %v, status %q", m.groupTabs, status(m))
	}

	// At most five group tabs.
	for _, g := range []string{"a1", "a2", "a3", "a4"} {
		m = pressRun(m, "+")
		m = typeText(m, g)
		m = pressRun(m, "enter")
	}
	if len(m.groupTabs) != maxGroupTabs || strings.Contains(header(m), " + ") {
		t.Errorf("%d tabs, header %q", len(m.groupTabs), header(m))
	}
	m = pressRun(m, "+")
	if m.mode == modeTabAdd || !strings.Contains(status(m), "5 group tabs at most") {
		t.Errorf("sixth tab allowed: mode %v status %q", m.mode, status(m))
	}
	// An empty group tab says so.
	if !strings.Contains(plainLines(m)[3], "no sessions in a4 yet") {
		t.Errorf("empty tab row %q", plainLines(m)[3])
	}
}

func TestEscNeverQuits(t *testing.T) {
	w := newWorld(t, testItems())
	next, cmd := w.model.Update(keyMsg("esc"))
	if isQuit(cmd) {
		t.Error("esc quit asp")
	}
	m := next.(Model)
	m = pressRun(m, "/")
	m = typeText(m, "tmp")
	m = pressRun(m, "enter", "esc")
	if m.query != "" {
		t.Error("esc should still clear the filter")
	}
	for _, k := range []string{"q", "ctrl+c"} {
		_, cmd := w.model.Update(keyMsg(k))
		if !isQuit(cmd) {
			t.Errorf("%s does not quit", k)
		}
	}
}

// isQuit reports whether cmd (or any command in a batch) quits.
func isQuit(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	switch msg := cmd().(type) {
	case tea.QuitMsg:
		return true
	case tea.BatchMsg:
		for _, c := range msg {
			if isQuit(c) {
				return true
			}
		}
	}
	return false
}

func TestEndPausedSession(t *testing.T) {
	w := newWorld(t, testItems())
	w.host.steps = []jobs.State{jobs.Paused}
	m := pressRun(w.model, "enter")
	if !strings.Contains(footer(m), "c end") {
		t.Errorf("footer on a paused session lacks c end: %q", footer(m))
	}
	m = pressRun(m, "c")
	if last := w.host.calls[len(w.host.calls)-1]; last != "end "+testItems()[0].Session.ID {
		t.Fatalf("calls %v", w.host.calls)
	}
	if cur, _ := m.current(); cur.paused() || len(w.host.Paused()) != 0 || status(m) != "session ended" {
		t.Errorf("after c: paused %v, status %q", cur.paused(), status(m))
	}
	m = pressRun(m, "c")
	if !strings.Contains(status(m), "only a paused session") {
		t.Errorf("c on a closed session: %q", status(m))
	}
}

func TestNewSessionFromGroupTabJoinsGroup(t *testing.T) {
	w := newWorld(t, testItems())
	dir := filepath.Join(w.home, "p")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	w.host.steps = []jobs.State{jobs.Paused}
	m := pressRun(w.model, "+")
	m = typeText(m, "waybar")
	m = pressRun(m, "enter", "n", "enter", "enter")
	m.input.SetValue(dir)
	m = pressRun(m, "enter")
	// Paused before it reached the disk: shown in this tab as a placeholder.
	if cur, ok := m.current(); !ok || !cur.placeholder() {
		t.Fatalf("placeholder not listed in the group tab")
	}
	w.disk = append(w.disk, Item{Session: source.Session{ID: "fresh", Agent: source.Claude, CWD: dir, Modified: time.Now()}})
	w.host.steps = []jobs.State{jobs.Exited}
	m = pressRun(m, "enter")
	if got := w.st.GroupsOf("claude", "fresh"); len(got) != 1 || got[0] != "waybar" {
		t.Errorf("new session groups %v", got)
	}
	if cur, _ := m.current(); cur.Session.ID != "fresh" {
		t.Errorf("selected %q", cur.Session.ID)
	}
}

func TestGroupTagsArePink(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(termenv.Ascii) })
	items := testItems()
	m := sized(New(items, Deps{Store: testStore(t)}), 120, 30)
	out := m.View()
	// #ff7ac6; lipgloss's colour conversion may round 122 to 121.
	isPink := func(l string) bool {
		return strings.Contains(l, "38;2;255;122;198") || strings.Contains(l, "38;2;255;121;198")
	}
	lines := strings.Split(out, "\n")
	found := false
	for _, l := range lines {
		if strings.Contains(ansi.Strip(l), "#arch #wm") {
			found = isPink(l)
		}
	}
	if !found {
		t.Error("#group tags are not in the group colour")
	}
	if isPink(lines[1]) {
		t.Error("tabs at the top should not use the group colour")
	}
	if config.Default().Colors["group"] != "#ff7ac6" {
		t.Error("default group colour changed")
	}
}
