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

func TestTabs(t *testing.T) {
	w := newWorld(t, testItems())
	for i, g := range map[int]string{0: "arch", 3: "arch", 5: "wm"} {
		it := w.disk[i]
		if err := w.st.AddToGroup(string(it.Session.Agent), it.Session.ID, g); err != nil {
			t.Fatal(err)
		}
	}
	m := w.model
	m.reloadGroups() // the groups were written to the store behind asp's back
	if m.tabCount() != 3 || m.tabLabel(1) != "claude" || m.tabLabel(2) != "codex" {
		t.Fatalf("default tabs %v", m.tabs)
	}

	// + offers agents without a tab and groups without one.
	m = pressRun(m, "+")
	if m.mode != modeTabAdd || strings.Join(m.sugg, ",") != "arch,wm" {
		t.Fatalf("+ offered %v", m.sugg)
	}
	m = typeText(m, "arch")
	m = pressRun(m, "enter")
	if g, ok := m.tabGroup(); !ok || g != "arch" || len(m.order) != 2 {
		t.Fatalf("arch tab: %q %v, %d sessions", g, ok, len(m.order))
	}
	h := header(m)
	if !strings.Contains(h, "arch 2") || strings.Contains(h, "#arch") || !strings.Contains(h, "+") {
		t.Errorf("header %q", h)
	}

	// Any tab but all can be closed — claude and codex included — and
	// added back with +.
	m = pressRun(m, "home")
	m.view = viewClaude
	m = pressRun(m, "-")
	if strings.Join(m.tabs, ",") != "agent:codex,group:arch" {
		t.Fatalf("after closing claude: %v", m.tabs)
	}
	m.view = viewAll
	m = pressRun(m, "-")
	if m.tabCount() != 3 || !strings.Contains(status(m), "all tab stays") {
		t.Errorf("closing all: %v, %q", m.tabs, status(m))
	}
	m = pressRun(m, "+")
	if len(m.sugg) == 0 || m.sugg[0] != "claude" {
		t.Fatalf("claude not offered back: %v", m.sugg)
	}
	if !strings.Contains(ansi.Strip(m.View()), "every claude session") {
		t.Error("agent choices should say what they are")
	}
	m = pressRun(m, "down", "enter")
	if a, ok := m.tabAgent(); !ok || a != source.Claude || m.tabLabel(m.view) != "claude" {
		t.Fatalf("claude tab not re-added: %v", m.tabs)
	}

	// Reopening asp restores the tabs, in order, and the current one.
	m = pressRun(m, "q")
	again := New(w.disk, Deps{Store: w.st})
	if strings.Join(again.tabs, ",") != "agent:codex,group:arch,agent:claude" {
		t.Errorf("tabs not restored: %v", again.tabs)
	}
	if a, _ := again.tabAgent(); a != source.Claude {
		t.Errorf("reopened on %q", again.tabKey(again.view))
	}

	// Closing everything but all survives a restart.
	for again.tabCount() > 1 {
		again.view = 1
		next, _ := again.Update(keyMsg("-"))
		again = next.(Model)
	}
	again.saveState()
	if third := New(w.disk, Deps{Store: w.st}); third.tabCount() != 1 {
		t.Errorf("closed tabs came back: %v", third.tabs)
	}

	// At most five group tabs.
	m = pressRun(w.model, "")
	for _, g := range []string{"a1", "a2", "a3", "a4", "a5"} {
		m = pressRun(m, "+")
		m = typeText(m, g)
		m = pressRun(m, "enter")
	}
	if m.groupTabCount() != maxGroupTabs {
		t.Fatalf("%d group tabs", m.groupTabCount())
	}
	m = pressRun(m, "+")
	m = typeText(m, "a6")
	m = pressRun(m, "enter")
	if m.groupTabCount() != maxGroupTabs || !strings.Contains(status(m), "5 group tabs at most") {
		t.Errorf("sixth group tab: %d, %q", m.groupTabCount(), status(m))
	}
	if !strings.Contains(plainLines(m)[3], "no sessions in a5 yet") {
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

func TestGroupColors(t *testing.T) {
	w := newWorld(t, testItems())
	m := pressRun(w.model, "g")
	m = typeText(m, "waybar")
	m = pressRun(m, "enter")
	// A new group asks for its colour straight away.
	if m.mode != modeGroupColor || m.colorFor != "waybar" {
		t.Fatalf("mode %v for %q", m.mode, m.colorFor)
	}
	m = typeText(m, "hot")
	if len(m.sugg) == 0 || m.sugg[0] != "hotpink" {
		t.Errorf("colour suggestions %v", m.sugg)
	}
	out := ansi.Strip(m.View())
	if !strings.Contains(out, "hotpink") || !strings.Contains(out, "#ff69b4") {
		t.Errorf("suggestions should show the hex:\n%s", out)
	}
	m = pressRun(m, "tab", "enter")
	if got := w.st.GroupColors()["waybar"]; got != "#ff69b4" {
		t.Errorf("waybar colour %q", got)
	}

	// Hex, with the # — and a bad value keeps the prompt open.
	m = pressRun(m, "j", "g")
	m = typeText(m, "arch")
	m = pressRun(m, "enter")
	m = typeText(m, "#12")
	m = pressRun(m, "enter")
	if m.mode != modeGroupColor || !strings.Contains(footer(m), "not a colour") {
		t.Errorf("bad colour accepted: %q", footer(m))
	}
	m = typeText(m, "3")
	if !strings.Contains(footer(m), "● #112233") {
		t.Errorf("no live preview: %q", footer(m))
	}
	m = pressRun(m, "enter")
	if got := w.st.GroupColors()["arch"]; got != "#112233" {
		t.Errorf("arch colour %q", got)
	}

	// Adding to an existing group does not ask again; skipping leaves it gray.
	m = pressRun(m, "j", "g")
	m = typeText(m, "arch")
	m = pressRun(m, "enter")
	if m.mode != modeList {
		t.Errorf("asked for a colour for an existing group: %v", m.mode)
	}
	m = pressRun(m, "g")
	m = typeText(m, "plain")
	m = pressRun(m, "enter", "esc")
	if _, ok := w.st.GroupColors()["plain"]; ok || m.mode != modeList {
		t.Error("skipping should leave the group without a colour")
	}

	// p recolours: straight away with one group, after a pick with several.
	m = pressRun(m, "home", "p")
	if m.mode != modeGroupColor || m.colorFor != "waybar" {
		t.Errorf("p on a one-group session: %v %q", m.mode, m.colorFor)
	}
	m = typeText(m, "light blue")
	m = pressRun(m, "enter")
	if got := w.st.GroupColors()["waybar"]; got != "#add8e6" {
		t.Errorf("recoloured to %q", got)
	}

	// Rendered: each tag in its colour; no colour → gray; tabs plain.
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(termenv.Ascii) })
	m = pressRun(m, "home")
	lines := strings.Split(m.View(), "\n")
	var tagLine string
	for _, l := range lines {
		if strings.Contains(ansi.Strip(l), "#waybar") && strings.Contains(ansi.Strip(l), " msg ") {
			tagLine = l
		}
	}
	if !strings.Contains(tagLine, "38;2;173;216;230") && !strings.Contains(tagLine, "38;2;172;216;230") {
		t.Errorf("#waybar not light blue: %q", tagLine)
	}
	if config.Default().Colors["group"] != "#9aa4b1" {
		t.Error("the fallback group colour should be gray")
	}
}

func TestParseColor(t *testing.T) {
	cases := map[string]string{
		"pink": "#ffc0cb", "Hot Pink": "#ff69b4", "HOTPINK": "#ff69b4", "#FFF": "#ffffff",
		"#ff7ac6": "#ff7ac6", "grey": "#808080",
	}
	for in, want := range cases {
		if got, ok := parseColor(in); !ok || got != want {
			t.Errorf("parseColor(%q) = %q, %v", in, got, ok)
		}
	}
	for _, bad := range []string{"", "#12", "#ggg", "notacolour", "ff7ac6"} {
		if _, ok := parseColor(bad); ok {
			t.Errorf("parseColor(%q) accepted", bad)
		}
	}
}

func TestOneOrNoAgentInstalled(t *testing.T) {
	st := testStore(t)
	onlyCodex := sized(New(testItems(), Deps{Store: st, Agents: []source.Agent{source.Codex}, Host: &fakeHost{}, Exec: syncExec}), 120, 30)
	m := pressRun(onlyCodex, "n")
	if m.mode != modeNewName || m.newAgent != source.Codex {
		t.Errorf("with only codex, n should skip to naming a codex session: %v %v", m.mode, m.newAgent)
	}

	none := sized(New(nil, Deps{Store: testStore(t), Agents: []source.Agent{}}), 80, 20)
	out := ansi.Strip(none.View())
	if !strings.Contains(out, "Neither Claude Code nor Codex CLI is installed") {
		t.Errorf("empty state:\n%s", out)
	}
	if none.tabCount() != 1 {
		t.Errorf("tabs for agents that are not there: %v", none.tabs)
	}
	m = pressRun(none, "n")
	if m.mode != modeList || !strings.Contains(status(m), "neither claude nor codex") {
		t.Errorf("n with no agents: %v %q", m.mode, status(m))
	}

	// Sessions left by an agent that is gone still list, with a tab.
	gone := sized(New(testItems(), Deps{Store: testStore(t), Agents: []source.Agent{}}), 120, 30)
	if gone.tabCount() != 3 || len(gone.order) != 11 {
		t.Errorf("tabs %v, %d sessions", gone.tabs, len(gone.order))
	}
}
