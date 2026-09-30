package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
	"github.com/summant/asp/internal/source"
	"github.com/summant/asp/internal/store"
)

func testStore(t *testing.T) *store.Store {
	t.Helper()
	dir := t.TempDir()
	st, err := store.OpenAt(filepath.Join(dir, "names.json"), filepath.Join(dir, "none.json"))
	if err != nil {
		t.Fatal(err)
	}
	return st
}

// testItems mixes agents, named and auto-labelled items, wide characters
// and long paths — everything that has broken alignment before.
func testItems() []Item {
	now := time.Now()
	home, _ := os.UserHomeDir()
	mk := func(i int, a source.Agent, title, name, cwd string) Item {
		return Item{
			Session: source.Session{
				ID: fmt.Sprintf("%08d-0000-0000-0000-000000000000", i), Agent: a, CWD: cwd,
				Opening: title, Messages: i * 431, Modified: now.Add(-time.Duration(i) * time.Hour),
			},
			Name: name,
		}
	}
	items := []Item{
		mk(0, source.Claude, "T00 set up waybar", "", "/tmp"),
		mk(1, source.Codex, "T01 fix the build", "", home+"/AgentSessionPicker"),
		mk(2, source.Claude, "T02 日本語のタイトルがとても長い場合にどうなるかを確認するためのテキストです", "", "/"),
		mk(3, source.Codex, "", "T03 named 🚀 rocket launch plan with a long tail of words", home+"/dotfiles"),
		mk(4, source.Claude, "T04 "+strings.Repeat("long ", 40), "", home+"/.superset/worktrees/Arch Ricing/sumptuous-thunbergia-77753f7d"),
		mk(5, source.Codex, "T05 ok", "", "/srv/"+strings.Repeat("deep/", 30)+"leaf"),
		mk(6, source.Claude, "T06 short", "Custom", "/tmp"),
		mk(7, source.Codex, "T07 x", "", "/"),
		mk(8, source.Claude, "T08 y", "", "/"),
		mk(9, source.Codex, "T09 z", "", "/"),
		mk(10, source.Claude, "T10 last", "", "/"),
	}
	items[3].Groups = []string{"arch", "wm"}
	items[4].Groups = []string{"a-very-long-group-name-that-goes-on"}
	return items
}

func keyMsg(s string) tea.KeyMsg {
	switch s {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "tab":
		return tea.KeyMsg{Type: tea.KeyTab}
	case "left":
		return tea.KeyMsg{Type: tea.KeyLeft}
	case "right":
		return tea.KeyMsg{Type: tea.KeyRight}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case "backspace":
		return tea.KeyMsg{Type: tea.KeyBackspace}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func press(m Model, keys ...string) Model {
	for _, k := range keys {
		next, _ := m.Update(keyMsg(k))
		m = next.(Model)
	}
	return m
}

func sized(m Model, w, h int) Model {
	next, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	return next.(Model)
}

func typeText(m Model, s string) Model {
	for _, r := range s {
		m = press(m, string(r))
	}
	return m
}

func plainLines(m Model) []string { return strings.Split(ansi.Strip(m.View()), "\n") }

// col is the display column at which byte offset i of line starts.
func col(line string, i int) int { return lipgloss.Width(line[:i]) }

func TestAlignment(t *testing.T) {
	for _, size := range [][2]int{{80, 30}, {120, 30}} {
		w, h := size[0], size[1]
		for _, keys := range [][]string{nil, {"j", "j", "j"}, {"l"}, {"/", "t", "0"}} {
			t.Run(fmt.Sprintf("%dx%d/%v", w, h, keys), func(t *testing.T) {
				m := press(sized(New(testItems(), Deps{Store: testStore(t)}), w, h), keys...)
				lines := plainLines(m)
				if len(lines) != h {
					t.Errorf("%d lines, want %d", len(lines), h)
				}
				titleCols, metaCols, ruleCols := map[int]bool{}, map[int]bool{}, map[int]bool{}
				items := 0
				for i, l := range lines {
					if lw := lipgloss.Width(l); lw > w {
						t.Errorf("line %d is %d cells wide, max %d: %q", i, lw, w, l)
					}
					if idx := strings.Index(l, " T"); idx >= 0 && (strings.Contains(l, "claude ") || strings.Contains(l, "codex ")) {
						titleCols[col(l, idx+1)] = true
						items++
					}
					if strings.Contains(l, " msg ") {
						rest := strings.TrimLeft(l, " "+gutterBar) // margin and gutter
						metaCols[col(l, len(l)-len(rest))] = true
					}
					if w >= breakpoint && i >= 3 && i <= h-3 {
						idx := strings.LastIndex(l, gutterBar)
						if idx < 0 {
							t.Errorf("body row %d has no rule: %q", i, l)
							continue
						}
						ruleCols[col(l, idx)] = true
					}
				}
				if items == 0 {
					t.Fatal("no item rows found")
				}
				if len(titleCols) != 1 || !titleCols[margin+titleCol] {
					t.Errorf("titles start at columns %v, want only %d", titleCols, margin+titleCol)
				}
				if len(metaCols) != 1 || !metaCols[margin+metaCol] {
					t.Errorf("metadata starts at columns %v, want only %d", metaCols, margin+metaCol)
				}
				if w >= breakpoint && len(ruleCols) != 1 {
					t.Errorf("rule at columns %v, want one", ruleCols)
				}
				if w < breakpoint && strings.Contains(strings.Join(lines, ""), "opening message") {
					t.Error("detail pane shown below the breakpoint")
				}
			})
		}
	}
}

func TestSelectionGutterSpansBothLines(t *testing.T) {
	m := press(sized(New(testItems(), Deps{Store: testStore(t)}), 120, 30), "j")
	lines := plainLines(m)
	for i, l := range lines {
		if strings.Contains(l, "codex   T01 ") { // the list row, not the detail title
			for _, row := range lines[i : i+2] {
				if !strings.HasPrefix(row, "  "+gutterBar+" ") {
					t.Errorf("selected row lacks gutter: %q", row)
				}
			}
			if strings.HasPrefix(lines[i+2], "  "+gutterBar) {
				t.Errorf("gap row has a gutter: %q", lines[i+2])
			}
			return
		}
	}
	t.Fatal("selected item not found")
}

func TestPagination(t *testing.T) {
	m := sized(New(testItems(), Deps{Store: testStore(t)}), 120, 30) // 24 body rows → 8 per page
	if got := m.perPage(); got != 8 {
		t.Fatalf("perPage = %d", got)
	}
	has := func(m Model, s string) bool { return strings.Contains(ansi.Strip(m.View()), s) }
	if !has(m, "T07 ") || has(m, "T08 ") {
		t.Error("page 1 should show T00–T07")
	}
	if dots := plainLines(m)[27]; strings.Count(dots, dot) != 2 {
		t.Errorf("dots row = %q, want two dots", dots)
	}
	m = press(m, "l")
	if m.cursor != 8 || !has(m, "T10 ") || has(m, "T00 ") {
		t.Errorf("after l: cursor %d", m.cursor)
	}
	m = press(m, "h")
	if m.cursor != 0 {
		t.Errorf("after h: cursor %d", m.cursor)
	}
	// Moving past the page edge turns the page.
	m = press(m, "j", "j", "j", "j", "j", "j", "j", "j")
	if !has(m, "T08 ") {
		t.Error("cursor on item 8 should show page 2")
	}
	// One page: no dots.
	small := sized(New(testItems()[:3], Deps{Store: testStore(t)}), 120, 30)
	if strings.Contains(plainLines(small)[27], dot) {
		t.Error("dots shown for a single page")
	}
}

func TestViewSwitchingIsRemembered(t *testing.T) {
	st := testStore(t)
	m := sized(New(testItems(), Deps{Store: st}), 120, 30)
	if m.view != viewAll || len(m.order) != 11 {
		t.Fatalf("default view %v with %d items", m.view, len(m.order))
	}
	m = press(m, "right")
	if m.view != viewClaude || len(m.order) != 6 {
		t.Errorf("right: view %v, %d items", m.view, len(m.order))
	}
	m = press(m, "d")
	if m.view != viewCodex || len(m.order) != 5 {
		t.Errorf("d: view %v, %d items", m.view, len(m.order))
	}
	m = press(m, "d")
	if m.view != viewAll {
		t.Errorf("d wraps to all, got %v", m.view)
	}
	m = press(m, "left", "a")
	if m.view != viewClaude {
		t.Errorf("left, a: got %v", m.view)
	}
	m = press(m, "j", "q")
	sel, _ := m.current()

	again := New(testItems(), Deps{Store: st})
	if again.view != viewClaude {
		t.Errorf("reopened in view %v, want claude", again.view)
	}
	if cur, _ := again.current(); cur.Session.ID != sel.Session.ID {
		t.Errorf("reopened on %s, want %s", cur.Session.ID, sel.Session.ID)
	}
}

func TestFilterFlow(t *testing.T) {
	m := sized(New(testItems(), Deps{Store: testStore(t)}), 120, 30)
	m = typeText(press(m, "/"), "dotfiles")
	if len(m.order) != 1 {
		t.Fatalf("dotfiles matched %d", len(m.order))
	}
	if f := plainLines(m)[29]; !strings.Contains(f, "filter") || !strings.HasSuffix(strings.TrimRight(f, " "), "1 of 11") {
		t.Errorf("filter footer %q", f)
	}
	m = press(m, "enter")
	if f := plainLines(m)[29]; !strings.Contains(f, "filter: dotfiles  (esc to clear)") {
		t.Errorf("confirmed filter footer %q", f)
	}
	// "/" again starts afresh: no stale results.
	m = press(m, "/")
	if len(m.order) != 11 || m.input.Value() != "" {
		t.Errorf("after / again: %d items, input %q", len(m.order), m.input.Value())
	}
	m = typeText(m, "zzzq")
	if l := plainLines(m)[3]; !strings.Contains(l, `no sessions match "zzzq"`) {
		t.Errorf("no-match row %q", l)
	}
	if strings.Contains(plainLines(m)[27], dot) {
		t.Error("pagination shown with no matches")
	}
	m = press(m, "enter", "esc")
	if m.query != "" || len(m.order) != 11 || m.mode != modeList {
		t.Error("esc should clear the filter before quitting")
	}
}

func TestRenameAndStatusClears(t *testing.T) {
	st := testStore(t)
	m := sized(New(testItems(), Deps{Store: st}), 120, 30)
	m = typeText(press(m, "r"), "Fresh name")
	m = press(m, "enter")
	if n, _ := st.Get("claude", testItems()[0].Session.ID); n != "Fresh name" {
		t.Errorf("stored %q", n)
	}
	if f := plainLines(m)[29]; strings.TrimSpace(f) != "renamed" {
		t.Errorf("footer %q", f)
	}
	m = press(m, "j")
	if f := plainLines(m)[29]; !strings.Contains(f, "open") {
		t.Errorf("status did not clear on next key: %q", f)
	}
	m = press(m, "k", "x")
	if n, ok := st.Get("claude", testItems()[0].Session.ID); ok {
		t.Errorf("x left name %q", n)
	}
	if f := plainLines(m)[29]; strings.TrimSpace(f) != "name cleared" {
		t.Errorf("footer %q", f)
	}
	m = press(m, "r")
	if m.input.Value() != "" {
		t.Errorf("rename of auto-labelled item prefilled %q", m.input.Value())
	}
}

func TestEmptyState(t *testing.T) {
	m := sized(New(nil, Deps{Store: testStore(t)}), 80, 20)
	out := ansi.Strip(m.View())
	for _, want := range []string{"asp", "No Claude Code or Codex sessions yet.", "n  start one"} {
		if !strings.Contains(out, want) {
			t.Errorf("empty state missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "claude 0") {
		t.Error("empty state should not show a summary")
	}
}

func TestEmptyView(t *testing.T) {
	m := sized(New(testItems()[:1], Deps{Store: testStore(t)}), 80, 20) // claude only
	m = press(m, "d", "d")
	if l := plainLines(m)[3]; !strings.Contains(l, "no codex sessions") {
		t.Errorf("row %q", l)
	}
}

func TestTinyTerminalNeverOverflows(t *testing.T) {
	for _, s := range [][2]int{{20, 5}, {40, 8}, {59, 12}, {99, 10}, {100, 10}, {200, 60}} {
		for _, keys := range [][]string{nil, {"/"}, {"n"}, {"r"}} {
			m := press(sized(New(testItems(), Deps{Store: testStore(t)}), s[0], s[1]), keys...)
			for i, l := range plainLines(m) {
				if lipgloss.Width(l) > s[0] {
					t.Errorf("%v %v line %d: %d cells > %d", s, keys, i, lipgloss.Width(l), s[0])
				}
			}
		}
	}
}

func TestMatch(t *testing.T) {
	sub, ok1 := match("zzzz zzzz zzzz zzzz waybar", "waybar")
	scat, ok2 := match("w a y b a r", "waybar")
	if !ok1 || !ok2 || sub >= scat {
		t.Errorf("substring %d (%v) should outrank scattered %d (%v)", sub, ok1, scat, ok2)
	}
	hay := "fix the build /home/u/agentsessionpicker codex"
	if _, ok := match(hay, "codex agent"); !ok {
		t.Error("codex agent should match (AND of both terms)")
	}
	if _, ok := match(hay, "codex dotfiles"); ok {
		t.Error("every term must match")
	}
	if _, ok := match("日本語のタイトル", "日タ"); !ok {
		t.Error("subsequence must work on runes, not bytes")
	}
	if _, ok := match("Waybar", "WAYBAR"); !ok {
		t.Error("matching is case-insensitive")
	}
}

func TestTruncate(t *testing.T) {
	styled := lipgloss.NewStyle().Foreground(lipgloss.Color("#7a1fff")).Render("styled text that is long")
	cases := []struct{ in string }{
		{"日本語のタイトルです"},
		{"rocket 🚀 launch 🚀🚀🚀"},
		{"👩‍💻 developer"},
		{styled},
		{"plain ascii that is long"},
	}
	for _, c := range cases {
		for w := 1; w <= 12; w++ {
			got := truncate(c.in, w)
			if lipgloss.Width(got) > w {
				t.Errorf("truncate(%q, %d) = %q, %d cells", c.in, w, got, lipgloss.Width(got))
			}
			if lipgloss.Width(c.in) > w && !strings.HasSuffix(ansi.Strip(got), ellipsis) {
				t.Errorf("truncate(%q, %d) = %q, want trailing …", c.in, w, got)
			}
			left := truncateLeft(c.in, w)
			if lipgloss.Width(left) > w {
				t.Errorf("truncateLeft(%q, %d) = %q, %d cells", c.in, w, left, lipgloss.Width(left))
			}
		}
		if got := truncate(c.in, 100); got != c.in {
			t.Errorf("fitting text changed: %q", got)
		}
	}
	if got := truncateLeft("/home/u/sumptuous-thunbergia", 20); got != "…sumptuous-thunbergia" && lipgloss.Width(got) != 20 {
		t.Errorf("truncateLeft kept %q", got)
	}
	if got := truncateLeft("/a/b/leaf", 6); got != "…/leaf" {
		t.Errorf("truncateLeft = %q, want …/leaf", got)
	}
}

func TestWrap(t *testing.T) {
	lines := wrap(strings.Repeat("word ", 100), 20, 6)
	if len(lines) != 6 || !strings.HasSuffix(lines[5], ellipsis) {
		t.Errorf("got %d lines, last %q", len(lines), lines[len(lines)-1])
	}
	for _, l := range append(lines, wrap("日本語のタイトルがとても長い場合にどうなるか", 10, 6)...) {
		if lipgloss.Width(l) > 20 {
			t.Errorf("line %q too wide", l)
		}
	}
	if got := wrap("short", 20, 6); len(got) != 1 || got[0] != "short" {
		t.Errorf("got %q", got)
	}
}

func TestCleanOpening(t *testing.T) {
	cases := map[string]string{
		"here: ────────────────────────────── This is my conf": "here: This is my conf",
		"Read /x/[AGENTS.md](http://AGENTS.md) first":     "Read /x/AGENTS.md first",
		"1\\. rules 2\\. handoff":                         "1. rules 2. handoff",
		"<user-prompt> Can you read this? </user-prompt>": "Can you read this?",
		"a **bold** move ---":                             "a bold move",
		"plain text stays":                                "plain text stays",
		"keep-hyphens and snake_case":                     "keep-hyphens and snake_case",
	}
	for in, want := range cases {
		if got := cleanOpening(in); got != want {
			t.Errorf("cleanOpening(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCompleteDir(t *testing.T) {
	root := t.TempDir()
	for _, d := range []string{"dotfiles", "documents", "downloads", ".hidden", "solo"} {
		if err := os.Mkdir(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "sol-file"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	cases := map[string]string{
		root + "/so":  root + "/solo/",
		root + "/do":  root + "/do",
		root + "/dot": root + "/dotfiles/",
		root + "/.h":  root + "/.hidden/",
		root + "/zz":  root + "/zz",
	}
	for in, want := range cases {
		if got := completeDir(in); got != want {
			t.Errorf("completeDir(%q) = %q, want %q", in, got, want)
		}
	}
}

// The terminal is translucent: nothing but the logo pill may paint a
// background (SPEC §5).
func TestNoBackgroundExceptLogo(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(termenv.Ascii) })

	sgr := regexp.MustCompile(`\x1b\[([0-9;]*)m`)
	for _, keys := range [][]string{nil, {"j"}, {"/", "t"}, {"n"}, {"r"}} {
		m := press(sized(New(testItems(), Deps{Store: testStore(t)}), 120, 30), keys...)
		out := m.View()
		if !strings.Contains(out, "\x1b[") {
			t.Fatal("no colour in output; profile not applied")
		}
		for i, line := range strings.Split(out, "\n") {
			for _, loc := range sgr.FindAllStringSubmatchIndex(line, -1) {
				params := line[loc[2]:loc[3]]
				if !strings.Contains(";"+params+";", ";48;") {
					continue
				}
				// lipgloss styles the pill's padding separately, so allow any
				// background that starts within the pill's cells: " asp ".
				at := lipgloss.Width(ansi.Strip(line[:loc[0]]))
				if i != 1 || at < margin || at >= margin+lipgloss.Width(" asp ") {
					t.Errorf("%v: background on row %d: %q", keys, i, ansi.Strip(line))
				}
			}
		}
	}
}
