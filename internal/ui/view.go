package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/summant/asp/internal/source"
)

const (
	minWidth   = 60
	detailFrac = 0.42
)

func (m Model) View() string {
	if m.w == 0 {
		return "" // first frame, before the size message arrives
	}

	// Chrome costs: outer padding (2) + both pane borders (4) + gutter (1).
	inner := m.w - 2
	detailW := 0
	if m.w >= 100 {
		detailW = int(float64(inner) * detailFrac)
	}
	listW := inner - detailW
	if detailW > 0 {
		listW-- // gutter between panes
	}

	bodyH := m.h - 4 // title row, blank, footer, prompt/status
	if bodyH < 4 {
		bodyH = 4
	}

	list := Pane.Width(listW - 2).Height(bodyH).Render(m.listBody(listW-4, bodyH))
	body := list
	if detailW > 0 {
		detail := Pane.Width(detailW - 3).Height(bodyH).Render(m.detailBody(detailW - 7))
		body = lipgloss.JoinHorizontal(lipgloss.Top, list, " ", detail)
	}

	return strings.Join([]string{
		" " + m.titleBar(),
		body,
		" " + m.footer(),
	}, "\n")
}

func (m Model) titleBar() string {
	left := AppTitle.Render("asp")
	counts := map[source.Agent]int{}
	for _, it := range m.items {
		counts[it.Session.Agent]++
	}
	sub := Help.Render(fmt.Sprintf("  %d sessions  %s  %s %d  %s %d",
		len(m.items), Sep,
		BadgeClaude.Render("claude"), counts[source.Claude],
		BadgeCodex.Render("codex"), counts[source.Codex]))
	if len(m.order) != len(m.items) {
		sub += Help.Render(fmt.Sprintf("  %s  %d shown", Sep, len(m.order)))
	}
	return left + sub
}

// listBody renders the visible window of rows, scrolling to keep the cursor in view.
func (m Model) listBody(w, h int) string {
	if len(m.order) == 0 {
		return "\n" + Help.Render("  no sessions match")
	}
	rows := h / 2 // two lines per item
	top := m.top
	if m.cursor < top {
		top = m.cursor
	}
	if m.cursor >= top+rows {
		top = m.cursor - rows + 1
	}

	var b strings.Builder
	for i := top; i < len(m.order) && i < top+rows; i++ {
		b.WriteString(m.items[m.order[i]].Render(w, i == m.cursor))
		if i < len(m.order)-1 && i < top+rows-1 {
			b.WriteString("\n")
		}
	}
	return b.String()
}

func (m Model) detailBody(w int) string {
	it, ok := m.current()
	if !ok {
		return Help.Render("nothing selected")
	}
	kv := func(k, v string) string {
		return DetailKey.Render(fmt.Sprintf("%-8s", k)) + DetailVal.Render(truncate(v, w-8))
	}
	name := "auto (from first message)"
	if it.Named() {
		name = "custom"
	}
	parts := []string{
		DetailTitle.Render(truncate(it.Title(), w)),
		"",
		kv("agent", it.Session.Agent.Label()),
		kv("folder", collapseHome(it.Session.CWD)),
		kv("when", it.Session.Modified.Format("Mon 2 Jan, 15:04")),
		kv("size", fmt.Sprintf("%s messages", humanCount(it.Session.Messages))),
		kv("name", name),
		kv("id", it.Session.ID[:min(8, len(it.Session.ID))]),
	}
	if it.Session.Opening != "" {
		parts = append(parts, "", DetailHead.Render("opening message"),
			ItemMeta.Render(wrap(it.Session.Opening, w, 6)))
	}
	return strings.Join(parts, "\n")
}

func (m Model) footer() string {
	if m.status != "" && m.mode == modeList {
		return FilterHit.Render(m.status)
	}
	switch m.mode {
	case modeFilter:
		return Prompt.Render("filter ") + m.input.View()
	case modeInput:
		label := map[inputKind]string{
			inputRename:  "rename ",
			inputNewName: "name the new session ",
			inputNewDir:  "folder ",
		}[m.kind]
		out := Prompt.Render(label) + m.input.View()
		if m.status != "" {
			out += "   " + Err.Render(m.status)
		}
		return out
	}
	k := func(key, desc string) string { return HelpKey.Render(key) + Help.Render(" "+desc) }
	return strings.Join([]string{
		k("↵", "resume"), k("n", "new"), k("r", "rename"),
		k("x", "unname"), k("/", "filter"), k("a", "agent"), k("q", "quit"),
	}, Help.Render("   "))
}

// wrap hard-wraps text to width, capped at maxLines.
func wrap(s string, width, maxLines int) string {
	if width < 10 {
		width = 10
	}
	var lines []string
	for len(s) > 0 && len(lines) < maxLines {
		if lipgloss.Width(s) <= width {
			lines = append(lines, s)
			break
		}
		cut := width
		if idx := strings.LastIndex(s[:min(len(s), width)], " "); idx > width/2 {
			cut = idx
		}
		if cut > len(s) {
			cut = len(s)
		}
		lines = append(lines, strings.TrimSpace(s[:cut]))
		s = strings.TrimSpace(s[cut:])
	}
	if len(s) > 0 && len(lines) == maxLines {
		lines[maxLines-1] = truncate(lines[maxLines-1]+" "+s, width)
	}
	return strings.Join(lines, "\n")
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func expand(p string) string {
	p = strings.TrimSpace(p)
	if strings.HasPrefix(p, "~") {
		if home, err := os.UserHomeDir(); err == nil {
			p = filepath.Join(home, strings.TrimPrefix(p, "~"))
		}
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return p
	}
	return abs
}

func isDir(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}
