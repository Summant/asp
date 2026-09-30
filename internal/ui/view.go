package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/paginator"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/summant/asp/internal/source"
)

// Layout metrics (SPEC §3).
const (
	margin      = 2    // outer horizontal margin, each side
	breakpoint  = 100  // below this width there is no detail pane
	detailFrac  = 0.42 // detail pane share of the inner width
	ruleW       = 3    // " │ " between list and detail
	itemRows    = 3    // two content lines and a gap
	chromeRows  = 6    // blank, header, blank … dots, blank, footer
	detailKeyW  = 9
	openingRows = 6
)

func (m Model) bodyRows() int { return max(itemRows, m.h-chromeRows) }

// columns splits the inner width into list and detail; detail is 0 when
// the terminal is too narrow for it.
func (m Model) columns() (list, detail int) {
	inner := max(1, m.w-2*margin)
	if m.w < breakpoint {
		return inner, 0
	}
	detail = int(float64(inner) * detailFrac)
	return inner - detail - ruleW, detail
}

func (m Model) View() string {
	if m.w == 0 || m.h == 0 {
		return "" // before the first size message
	}
	pad := strings.Repeat(" ", margin)
	listW, detailW := m.columns()
	body := m.listRows(listW, m.bodyRows()+1) // the last row holds the dots

	rows := []string{"", pad + m.header(), ""}
	if detailW > 0 {
		detail := m.detailRows(detailW)
		div := " " + ruleStyle.Render(gutterBar) + " "
		for i, l := range body {
			d := ""
			if i < len(detail) {
				d = detail[i]
			}
			rows = append(rows, pad+padRight(l, listW)+div+d)
		}
	} else {
		for _, l := range body {
			rows = append(rows, pad+l)
		}
	}
	rows = append(rows, "", pad+m.footer(m.w-2*margin))

	// A terminal shorter than the chrome gets the top of the screen; no
	// line may ever exceed the width, whatever went into it.
	if len(rows) > m.h {
		rows = rows[:m.h]
	}
	for i, r := range rows {
		if lipgloss.Width(r) > m.w {
			rows[i] = ansi.Truncate(r, m.w, "")
		}
	}
	return strings.Join(rows, "\n")
}

func (m Model) header() string {
	logo := logoStyle.Render("asp")
	if len(m.items) == 0 {
		return logo
	}
	counts := map[source.Agent]int{}
	for _, it := range m.items {
		counts[it.Session.Agent]++
	}
	seg := func(v agentView, n int) string {
		s := fmt.Sprintf("%s %d", v, n)
		if v == m.view {
			return viewOnStyle.Render(s)
		}
		return summaryStyle.Render(s)
	}
	dotSep := summaryStyle.Render("  " + sep + "  ")
	return logo + "  " + seg(viewAll, len(m.items)) + dotSep +
		seg(viewClaude, counts[source.Claude]) + dotSep + seg(viewCodex, counts[source.Codex])
}

// listRows renders exactly n rows of the list column: the current page of
// items, then the pagination dots on the last row.
func (m Model) listRows(w, n int) []string {
	rows := make([]string, 0, n)
	switch {
	case len(m.items) == 0:
		rows = append(rows,
			titleStyle.Render("No Claude Code or Codex sessions yet."),
			"",
			helpKey.Render("n")+"  "+helpDesc.Render("start one"))
	case len(m.order) == 0 && m.query != "":
		rows = append(rows, metaStyle.Render(truncate(fmt.Sprintf("no sessions match %q", m.query), w)))
	case len(m.order) == 0:
		rows = append(rows, metaStyle.Render(fmt.Sprintf("no %s sessions", m.view)))
	default:
		per := m.perPage()
		start := m.cursor / per * per
		for i := start; i < min(start+per, len(m.order)); i++ {
			st := plain
			if i == m.cursor {
				st = selected
			} else if m.query != "" {
				st = matched
			}
			lines := m.items[m.order[i]].renderItem(w, st)
			rows = append(rows, lines[0], lines[1], "")
		}
	}
	for len(rows) < n-1 {
		rows = append(rows, "")
	}
	rows = rows[:n-1]
	return append(rows, m.dots(w))
}

// dots is the page indicator, hidden when everything fits on one page.
func (m Model) dots(w int) string {
	per := m.perPage()
	if len(m.order) <= per {
		return ""
	}
	p := paginator.New(paginator.WithPerPage(per))
	p.SetTotalPages(len(m.order))
	p.Page = m.cursor / per
	p.Type = paginator.Dots
	p.ActiveDot = dotOn.Render(dot) + " "
	p.InactiveDot = dotOff.Render(dot) + " "
	if v := strings.TrimRight(p.View(), " "); lipgloss.Width(v) <= w {
		return v
	}
	// Too many pages for dots: fall back to "3/12".
	return dotOn.Render(fmt.Sprintf("%d/%d", p.Page+1, p.TotalPages))
}

func (m Model) detailRows(w int) []string {
	it, ok := m.current()
	if !ok {
		return nil
	}
	kv := func(k, v string) string {
		return detailKey.Render(padRight(k, detailKeyW)) + detailVal.Render(v)
	}
	name := "auto"
	if it.Named() {
		name = "custom"
	}
	valW := w - detailKeyW
	rows := []string{
		detailVal.Render(truncate(it.Title(), w)),
		"",
		kv("agent", it.Session.Agent.Label()),
		kv("folder", truncateLeft(collapseHome(it.Session.CWD), valW)),
		kv("when", it.Session.Modified.Format("Mon 2 Jan, 15:04")),
		kv("size", humanCount(it.Session.Messages)+" messages"),
		kv("name", name),
		kv("id", ansi.Truncate(it.Session.ID, min(8, valW), "")),
	}
	if o := it.Opening(); o != "" {
		rows = append(rows, "", detailHead.Render("opening message"))
		for _, l := range wrap(o, w, openingRows) {
			rows = append(rows, metaStyle.Render(l))
		}
	}
	return rows
}

// wrap word-wraps s to width cells, at most maxLines, ending in "…" if cut.
func wrap(s string, width, maxLines int) []string {
	if width <= 0 {
		return nil
	}
	lines := strings.Split(ansi.Wrap(s, width, ""), "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " ")
	}
	if len(lines) > maxLines {
		lines = lines[:maxLines]
		last := lines[maxLines-1]
		if lipgloss.Width(last) >= width {
			last = ansi.Truncate(last, width-1, "")
		}
		lines[maxLines-1] = last + ellipsis
	}
	return lines
}

// footer is the help line, a status message, or the active prompt.
func (m Model) footer(w int) string {
	agentLabel := func() string {
		tag := tagClaude
		if m.newAgent == source.Codex {
			tag = tagCodex
		}
		return promptLabel.Render("new ") + tag.Render(m.newAgent.Label()) + promptLabel.Render(" session "+sep+" ")
	}
	switch m.mode {
	case modeFilter:
		right := helpDesc.Render(fmt.Sprintf("%d of %d", len(m.order), m.viewTotal()))
		return m.promptLine(w, promptLabel.Render("filter  "), right)
	case modeRename:
		return m.promptLine(w, promptLabel.Render("rename  "), helpDesc.Render("enter save "+sep+" esc cancel"))
	case modeNewName:
		return m.promptLine(w, agentLabel()+promptLabel.Render("name  "), helpDesc.Render("tab switch agent "+sep+" esc cancel"))
	case modeNewDir:
		right := helpDesc.Render("tab complete " + sep + " enter start")
		if m.err != "" {
			right = errStyle.Render(m.err)
		}
		return m.promptLine(w, agentLabel()+promptLabel.Render("folder  "), right)
	}

	if m.status != "" {
		return statusStyle.Render(truncate(m.status, w))
	}
	if m.query != "" {
		return helpDesc.Render(truncate(fmt.Sprintf("filter: %s  (esc to clear)", m.query), w))
	}
	if len(m.items) == 0 {
		return ""
	}
	entries := []string{"↵ resume", "n new", "r rename", "x unname", "/ filter", "←→ view", "q quit"}
	var out string
	for _, e := range entries {
		k, d, _ := strings.Cut(e, " ")
		next := helpKey.Render(k) + " " + helpDesc.Render(d)
		if out != "" {
			next = "   " + next
		}
		if lipgloss.Width(out+next) > w {
			break // drop what does not fit rather than wrap
		}
		out += next
	}
	return out
}

// promptLine lays out label, text input and a right-aligned hint on one row.
// The hint is dropped before the input is squeezed.
func (m Model) promptLine(w int, label, right string) string {
	gap := 3
	room := w - lipgloss.Width(label) - lipgloss.Width(right) - gap
	if room < 12 {
		right, room = "", w-lipgloss.Width(label)
	}
	in := m.input
	in.Width = max(1, room-1) // the cursor takes a cell
	left := label + in.View()
	if right == "" {
		return left
	}
	fill := max(gap, w-lipgloss.Width(left)-lipgloss.Width(right))
	return left + strings.Repeat(" ", fill) + right
}

func (m Model) viewTotal() int {
	n := 0
	for _, it := range m.items {
		if m.view.shows(it.Session.Agent) {
			n++
		}
	}
	return n
}
