package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
	"github.com/summant/asp/internal/config"
)

func TestConfiguredKeys(t *testing.T) {
	cfg, err := config.Parse(`
[keys.list]
quit = ["Q"]
details = ["i"]
back = ["ctrl+g"]
view_next = ["tab"]
view_prev = ["shift+tab"]
[keys.prompt]
cancel = ["ctrl+g"]
[keys.finder]
open = ["ctrl+l"]
parent = ["ctrl+h"]
`, "t")
	if err != nil {
		t.Fatal(err)
	}
	w := newWorld(t, testItems())
	w.model.keys = newKeymap(cfg.Keys)

	m := pressRun(w.model, "q") // no longer quits: nothing happens
	if m.mode != modeList || m.confirmQuit {
		t.Error("q still acts after being unbound")
	}
	f := footer(m)
	for _, want := range []string{"i details", "Q quit", "shift+tab/tab view"} {
		if !strings.Contains(f, want) {
			t.Errorf("footer %q lacks %q", f, want)
		}
	}
	m = pressRun(m, "tab")
	if m.view != viewClaude {
		t.Errorf("tab did not switch view: %v", m.view)
	}
	m = pressRun(m, "i")
	if m.mode != modeRead {
		t.Errorf("i did not open details: %v", m.mode)
	}
	m = pressRun(m, "esc", "/")
	m = typeText(m, "zz")
	m = pressRun(m, "esc") // esc is no longer cancel in prompts…
	if m.mode != modeFilter {
		t.Errorf("esc still cancels: %v", m.mode)
	}
	m = pressRun(m, "ctrl+g") // …ctrl+g is
	if m.mode != modeList || m.query != "" {
		t.Errorf("ctrl+g: mode %v query %q", m.mode, m.query)
	}
	m = pressRun(m, "?", "G") // help scrolls like the details view
	out := ansi.Strip(m.View())
	if !strings.Contains(out, "Q ") || !strings.Contains(out, "config.toml") {
		t.Errorf("help does not reflect the config:\n%s", out)
	}
}

func TestConfiguredColors(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() {
		lipgloss.SetColorProfile(termenv.Ascii)
		ApplyTheme(config.Default().Colors)
	})
	cfg, err := config.Parse("[colors]\naccent = \"#ff0000\"\ntext = \"#00ff00\"\n", "t")
	if err != nil {
		t.Fatal(err)
	}
	ApplyTheme(cfg.Colors)
	m := sized(New(testItems(), Deps{Store: testStore(t)}), 120, 30)
	out := m.View()
	if !strings.Contains(out, "38;2;255;0;0") {
		t.Error("accent colour not used for the selection")
	}
	if !strings.Contains(out, "48;2;255;0;0") {
		t.Error("accent colour not used for the logo background")
	}
	if !strings.Contains(out, "38;2;0;255;0") {
		t.Error("text colour not used for titles")
	}
	if strings.Contains(out, "38;2;122;31;255") {
		t.Error("the default accent is still in use")
	}
}
