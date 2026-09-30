package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/summant/asp/internal/store"
)

// asp v0.1.0 saved only group tabs (claude and codex were fixed). Its
// state must come back as the same tabs, on the same one.
func TestV010StateKeepsTabs(t *testing.T) {
	dir := t.TempDir()
	for _, f := range []string{"names.json", "groups.json", "state.json"} {
		b, err := os.ReadFile(filepath.Join("..", "store", "testdata", "v0.1.0", f))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, f), b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	st, err := store.OpenAt(filepath.Join(dir, "names.json"), filepath.Join(dir, "none.json"))
	if err != nil {
		t.Fatal(err)
	}
	m := New(testItems(), Deps{Store: st})
	if got := strings.Join(m.tabs, ","); got != "agent:claude,agent:codex,group:arch,group:wm" {
		t.Errorf("tabs %s", got)
	}
	if g, _ := m.tabGroup(); g != "arch" {
		t.Errorf("opened on %q, want the arch tab it was closed on", m.tabKey(m.view))
	}
	// Saved in the new format, it reads back the same.
	m.saveState()
	again := New(testItems(), Deps{Store: st})
	if strings.Join(again.tabs, ",") != strings.Join(m.tabs, ",") || again.view != m.view {
		t.Errorf("after re-save: %v on %d", again.tabs, again.view)
	}
}

func TestUnreadableDataIsReported(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "groups.json"), []byte("{oops"), 0o644); err != nil {
		t.Fatal(err)
	}
	st, _ := store.OpenAt(filepath.Join(dir, "names.json"), filepath.Join(dir, "none.json"))
	m := sized(New(testItems(), Deps{Store: st}), 200, 30)
	if !strings.Contains(ansi.Strip(m.View()), "groups.json could not be read") {
		t.Error("an unreadable groups.json is not reported")
	}
}

func TestRecolourFromGroupTab(t *testing.T) {
	w := newWorld(t, testItems())
	if err := w.st.AddToGroup("claude", w.disk[0].Session.ID, "arch"); err != nil {
		t.Fatal(err)
	}
	if err := w.st.AddToGroup("claude", w.disk[0].Session.ID, "wm"); err != nil {
		t.Fatal(err)
	}
	m := w.model
	m.reloadGroups()
	m = pressRun(m, "+")
	m = typeText(m, "wm")
	m = pressRun(m, "enter", "p")
	if m.mode != modeGroupColor || m.colorFor != "wm" {
		t.Errorf("p on the wm tab: mode %v for %q", m.mode, m.colorFor)
	}
}
