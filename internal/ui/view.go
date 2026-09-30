package ui

import (
	"fmt"
	"path/filepath"
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
	inner := m.w - 2*margin
	rows := []string{"", pad + m.header(), ""}

	switch m.mode {
	case modeRead, modeHelp:
		body := m.readLines()
		if m.mode == modeHelp {
			body = helpLines(inner)
		}
		n := m.bodyRows() + 1
		body = body[min(m.scroll, len(body)):]
		for i := 0; i < n; i++ {
			l := ""
			if i < len(body) {
				l = body[i]
			}
			rows = append(rows, pad+l)
		}
	default:
		listW, detailW := m.columns()
		var body []string
		if m.mode == modeBrowse {
			body = m.browseRows(listW, m.bodyRows()+1)
		} else {
			body = m.listRows(listW, m.bodyRows()+1) // the last row holds the dots
		}
		if detailW > 0 {
			var detail []string
			if m.mode == modeBrowse {
				detail = m.browsePreview(detailW)
			} else {
				detail = m.detailRows(detailW)
			}
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
	}
	rows = append(rows, "", pad+m.footer(inner))

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
	out := logo + "  " + seg(viewAll, len(m.items)) + dotSep +
		seg(viewClaude, counts[source.Claude]) + dotSep + seg(viewCodex, counts[source.Codex])
	if n := m.pausedCount(); n > 0 {
		out += dotSep + statusStyle.Render(fmt.Sprintf("%d paused", n))
	}
	return out
}

// listRows renders exactly n rows of the list column: the current page of
// items, any suggestions for the active prompt at the bottom, and the
// pagination dots on the last row.
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

	if sugg := m.suggestionRows(w, n-2); len(sugg) > 0 {
		copy(rows[len(rows)-len(sugg):], sugg)
		return append(rows, "")
	}
	return append(rows, m.dots(w))
}

// suggestionRows draws the active prompt's suggestions, a blank line and a
// heading above them, to sit directly over the footer. At most limit rows.
func (m Model) suggestionRows(w, limit int) []string {
	if len(m.sugg) == 0 || limit < 3 {
		return nil
	}
	sugg := m.sugg[:min(len(m.sugg), limit-2)]
	rows := []string{"", detailHead.Render(m.promptTitle())}
	for i, s := range sugg {
		if i == m.suggSel {
			rows = append(rows, gutterSel.Render(gutterBar)+" "+titleSel.Render(truncateLeft(s, w-gutterW)))
		} else {
			rows = append(rows, "  "+titleStyle.Render(truncateLeft(s, w-gutterW)))
		}
	}
	return rows
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
	valW := w - detailKeyW
	name := "auto"
	if it.Named() {
		name = "custom"
	}
	id := "not saved yet"
	if !it.placeholder() {
		id = ansi.Truncate(it.Session.ID, min(8, valW), "")
	}
	rows := []string{
		detailVal.Render(truncate(it.Title(), w)),
		"",
		kv("agent", it.Session.Agent.Label()),
		kv("folder", truncateLeft(collapseHome(it.Session.CWD), valW)),
		kv("when", it.Session.Modified.Format("Mon 2 Jan, 15:04")),
		kv("size", humanCount(it.Session.Messages)+" messages"),
		kv("name", name),
		kv("id", id),
	}
	if len(it.Groups) > 0 {
		rows = append(rows, kv("groups", truncate("#"+strings.Join(it.Groups, " #"), valW)))
	}
	if it.paused() {
		rows = append(rows, detailKey.Render(padRight("status", detailKeyW))+statusStyle.Render("paused · ↵ to go back"))
	}
	if o := it.Opening(); o != "" {
		rows = append(rows, "", detailHead.Render("opening message"))
		for _, l := range wrap(o, w, openingRows) {
			rows = append(rows, metaStyle.Render(l))
		}
	}
	return rows
}

// readLines is the reader view: the whole opening message at full width,
// alone on screen so a mouse selection copies only the message.
func (m Model) readLines() []string {
	it, ok := m.current()
	if !ok {
		return nil
	}
	w := m.w - 2*margin
	rows := []string{
		detailVal.Render(truncate(it.Title(), w)),
		metaStyle.Render(truncate(it.Session.Agent.Label()+"  "+sep+"  "+collapseHome(it.Session.CWD)+"  "+sep+"  "+it.Session.Modified.Format("Mon 2 Jan, 15:04"), w)),
		"",
	}
	if it.Session.Opening == "" {
		return append(rows, metaStyle.Render("(no messages)"))
	}
	for _, l := range wrap(it.Session.Opening, w, 1<<30) {
		rows = append(rows, detailVal.Render(l))
	}
	return rows
}

var helpKeys = [][2]string{
	{"↵", "open the session · go back to a paused one"},
	{"ctrl+z", "inside claude or codex: pause it and return here"},
	{"n", "new session: agent, name, folder"},
	{"r  x", "rename · clear the name"},
	{"m  M", "add to a group · remove from a group"},
	{"/", "filter · f:folder  g:group  \"quoted words\""},
	{"←→  a d", "all · claude · codex"},
	{"v  y", "read the opening message · copy it"},
	{"j k  ↑↓", "move"},
	{"h l", "previous · next page"},
	{"g G", "first · last"},
	{"ctrl+o", "in a folder prompt or filter: browse folders"},
	{"q", "quit (ends paused sessions)"},
}

func helpLines(w int) []string {
	rows := []string{detailHead.Render("keys"), ""}
	for _, k := range helpKeys {
		rows = append(rows, helpKey.Render(padRight(k[0], 10))+helpDesc.Render(truncate(k[1], w-10)))
	}
	return append(rows, "", helpDesc.Render("any key to close"))
}

// browseRows draws the folder browser in the list column.
func (m Model) browseRows(w, n int) []string {
	sel, vis := m.browseSelected()
	rows := []string{metaSel.Render(truncateLeft(collapseHome(m.browse.dir)+"/", w)), ""}
	room := n - len(rows) - 1
	start := max(0, m.browse.sel-room+1)
	for i := start; i < min(len(vis), start+room); i++ {
		e := vis[i]
		label := e.name
		style := metaStyle
		if e.dir && e.name != here {
			label += "/"
			style = titleStyle
		}
		if e.name == here {
			label = here + "  (this folder)"
		}
		if e == sel && i == m.browse.sel {
			rows = append(rows, gutterSel.Render(gutterBar)+" "+titleSel.Render(truncate(label, w-gutterW)))
		} else {
			rows = append(rows, "  "+style.Render(truncate(label, w-gutterW)))
		}
	}
	if len(vis) == 1 && m.input.Value() != "" {
		rows = append(rows, "  "+metaStyle.Render("nothing matches"))
	}
	for len(rows) < n {
		rows = append(rows, "")
	}
	return rows[:n]
}

// browsePreview lists the highlighted folder's contents in the detail pane.
func (m Model) browsePreview(w int) []string {
	sel, _ := m.browseSelected()
	dir := m.browse.dir
	if sel.dir && sel.name != here {
		dir = filepath.Join(dir, sel.name)
	} else if !sel.dir {
		return []string{detailHead.Render("file"), metaStyle.Render(truncate(sel.name, w))}
	}
	rows := []string{detailHead.Render(truncateLeft(collapseHome(dir)+"/", w)), ""}
	entries := readEntries(dir)
	shown := 0
	for _, e := range entries {
		if strings.HasPrefix(e.name, ".") {
			continue
		}
		if shown == m.bodyRows()-3 {
			rows = append(rows, metaStyle.Render(ellipsis))
			break
		}
		if e.dir {
			rows = append(rows, detailVal.Render(truncate(e.name+"/", w)))
		} else {
			rows = append(rows, metaStyle.Render(truncate(e.name, w)))
		}
		shown++
	}
	if shown == 0 {
		rows = append(rows, metaStyle.Render("empty"))
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
	newLabel := func(step string) string {
		tag := tagClaude
		if m.newAgent == source.Codex {
			tag = tagCodex
		}
		return promptLabel.Render("new ") + tag.Render(m.newAgent.Label()) + promptLabel.Render(" session "+sep+" "+step+"  ")
	}
	hint := func(s string) string { return helpDesc.Render(s) }
	switch m.mode {
	case modeFilter:
		right := hint(fmt.Sprintf("%d of %d", len(m.order), m.viewTotal()))
		if len(m.sugg) > 0 {
			right = hint("↑↓ pick " + sep + " tab fill")
		}
		return m.promptLine(w, promptLabel.Render("filter  "), right)
	case modeRename:
		return m.promptLine(w, promptLabel.Render("rename  "), hint("enter save "+sep+" esc cancel"))
	case modeNewAgent:
		left := promptLabel.Render("new session  ") + m.agentChoice()
		right := hint("←→ choose " + sep + " enter next " + sep + " esc cancel")
		return spread(w, left, right)
	case modeNewName:
		return m.promptLine(w, newLabel("name"), hint("enter next "+sep+" empty keeps the auto label"))
	case modeNewDir:
		right := hint("↑↓ pick " + sep + " tab fill " + sep + " ctrl+o browse " + sep + " enter start")
		if m.err != "" {
			right = errStyle.Render(m.err)
		}
		return m.promptLine(w, newLabel("folder"), right)
	case modeGroupAdd:
		return m.promptLine(w, promptLabel.Render("add to group  "), hint("↑↓ pick "+sep+" enter add "+sep+" esc cancel"))
	case modeGroupRemove:
		return m.promptLine(w, promptLabel.Render("remove from group  "), hint("↑↓ pick "+sep+" enter remove "+sep+" esc cancel"))
	case modeBrowse:
		return m.promptLine(w, promptLabel.Render("browse  "), hint("←→ up/into "+sep+" enter choose "+sep+" esc back"))
	case modeRead:
		return spread(w, hint("j k scroll "+sep+" y copy "+sep+" esc back"), "")
	case modeHelp:
		return ""
	}

	if m.status != "" {
		if m.statusBad {
			return errStyle.Render(truncate(m.status, w))
		}
		return statusStyle.Render(truncate(m.status, w))
	}
	if m.query != "" {
		return helpDesc.Render(truncate(fmt.Sprintf("filter: %s  (esc to clear)", m.query), w))
	}
	if len(m.items) == 0 {
		return ""
	}
	entries := []string{"↵ open", "n new", "r rename", "m group", "/ filter", "←→ view", "? keys", "q quit"}
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

// spread puts left and right at the two ends of a w-cell line, dropping
// right when both do not fit.
func spread(w int, left, right string) string {
	gap := w - lipgloss.Width(left) - lipgloss.Width(right)
	if right == "" || gap < 3 {
		return left
	}
	return left + strings.Repeat(" ", gap) + right
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
