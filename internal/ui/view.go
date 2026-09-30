package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/paginator"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/summant/asp/internal/config"
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
		body := m.pageLines()
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
		if m.mode == modeFind {
			body = m.finderRows(listW, m.bodyRows()+1)
		} else {
			body = m.listRows(listW, m.bodyRows()+1) // the last row holds the dots
		}
		if detailW > 0 {
			var detail []string
			if m.mode == modeFind {
				detail = m.finderPreview(detailW)
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
	if m.query != "" && m.mode != modeFilter {
		out += dotSep + statusStyle.Render("filter: "+m.query) + helpDesc.Render("  esc clears")
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
			helpKey.Render(m.keys.show("list", "new"))+"  "+helpDesc.Render("start one"))
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
		gutter, style := "  ", titleStyle
		if i == m.suggSel {
			gutter, style = gutterSel.Render(gutterBar)+" ", titleSel
		}
		if help := prefixHelp[s]; help != "" && m.mode == modeFilter {
			rows = append(rows, gutter+style.Render(padRight(s, 4))+helpDesc.Render(truncate(help, w-gutterW-4)))
			continue
		}
		rows = append(rows, gutter+style.Render(truncateLeft(s, w-gutterW)))
	}
	return rows
}

// dots is the page indicator, hidden when everything fits on one page.
func (m Model) dots(w int) string { return pageDots(len(m.order), m.perPage(), m.cursor, w) }

// pageDots draws glow's pagination for n items, per to a page, with the
// item at cursor on the active page; "3/12" when the dots would not fit.
func pageDots(n, per, cursor, w int) string {
	if n <= per {
		return ""
	}
	p := paginator.New(paginator.WithPerPage(per))
	p.SetTotalPages(n)
	p.Page = cursor / per
	p.Type = paginator.Dots
	p.ActiveDot = dotOn.Render(dot) + " "
	p.InactiveDot = dotOff.Render(dot) + " "
	if v := strings.TrimRight(p.View(), " "); lipgloss.Width(v) <= w {
		return v
	}
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
		rows = append(rows, detailKey.Render(padRight("status", detailKeyW))+statusStyle.Render("paused · "+m.keys.show("list", "open")+" to go back"))
	}
	if o := it.Opening(); o != "" {
		rows = append(rows, "", detailHead.Render("opening message"))
		for _, l := range wrap(o, w, openingRows) {
			rows = append(rows, metaStyle.Render(l))
		}
	}
	return rows
}

// pageLines is the full-width page being shown: help or details.
func (m Model) pageLines() []string {
	if m.mode == modeHelp {
		return m.helpLines(m.w - 2*margin)
	}
	return m.readLines()
}

// readLines is the details view: everything the detail pane shows, at full
// width and untruncated, then the whole opening message — alone on screen,
// so a mouse selection copies only this session's text.
func (m Model) readLines() []string {
	it, ok := m.current()
	if !ok {
		return nil
	}
	w := m.w - 2*margin
	kv := func(k, v string) []string {
		var out []string
		for i, l := range wrap(v, max(1, w-detailKeyW), 1<<30) {
			key := ""
			if i == 0 {
				key = k
			}
			out = append(out, detailKey.Render(padRight(key, detailKeyW))+detailVal.Render(l))
		}
		return out
	}
	name := "auto (from the first message)"
	if it.Named() {
		name = it.Name
	}
	id := "not saved yet"
	if !it.placeholder() {
		id = it.Session.ID
	}
	rows := []string{detailVal.Render(truncate(it.Title(), w)), ""}
	rows = append(rows, kv("agent", it.Session.Agent.Label())...)
	rows = append(rows, kv("folder", collapseHome(it.Session.CWD))...)
	rows = append(rows, kv("when", it.Session.Modified.Format("Mon 2 Jan 2006, 15:04"))...)
	rows = append(rows, kv("size", humanCount(it.Session.Messages)+" messages")...)
	rows = append(rows, kv("name", name)...)
	rows = append(rows, kv("id", id)...)
	if len(it.Groups) > 0 {
		rows = append(rows, kv("groups", "#"+strings.Join(it.Groups, " #"))...)
	}
	if it.paused() {
		rows = append(rows, kv("status", "paused · "+m.keys.show("list", "open")+" to go back")...)
	}
	rows = append(rows, "", detailHead.Render("opening message"))
	if it.Session.Opening == "" {
		return append(rows, metaStyle.Render("(no messages)"))
	}
	for _, l := range wrap(it.Session.Opening, w, 1<<30) {
		rows = append(rows, detailVal.Render(l))
	}
	return rows
}

// helpLines lists every list key, as configured, plus ctrl+z.
func (m Model) helpLines(w int) []string {
	rows := []string{detailHead.Render("keys"), ""}
	line := func(keys, desc string) {
		rows = append(rows, helpKey.Render(padRight(keys, 14))+helpDesc.Render(truncate(desc, w-14)))
	}
	line("ctrl+z", "inside claude or codex: pause it and return here")
	for _, s := range config.Sections {
		if s.Name != "list" {
			continue
		}
		for _, b := range s.Bindings {
			if keys := m.keys.all("list", b.Action); keys != "" {
				line(keys, b.Desc)
			}
		}
	}
	return append(rows, "", helpDesc.Render("change any of these in "+collapseHome(config.Path())))
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

// footer is the help line, a status message, or the active prompt. Every
// key shown comes from the keymap, so it matches config.toml.
func (m Model) footer(w int) string {
	k := m.keys
	newLabel := func(step string) string {
		tag := tagClaude
		if m.newAgent == source.Codex {
			tag = tagCodex
		}
		return promptLabel.Render("new ") + tag.Render(m.newAgent.Label()) + promptLabel.Render(" session "+sep+" "+step+"  ")
	}
	pick := k.pair("prompt", "prev", "next")
	switch m.mode {
	case modeFilter:
		right := helpDesc.Render(fmt.Sprintf("%d of %d", len(m.order), m.viewTotal()))
		if len(m.sugg) > 0 {
			right = hint(pick, "pick", k.show("prompt", "fill"), "fill")
		}
		return m.promptLine(w, promptLabel.Render("filter  "), right)
	case modeRename:
		return m.promptLine(w, promptLabel.Render("rename  "), hint(k.show("prompt", "confirm"), "save", k.show("prompt", "cancel"), "cancel"))
	case modeNewAgent:
		left := promptLabel.Render("new session  ") + m.agentChoice()
		return spread(w, left, hint(k.show("agent", "toggle"), "switch", k.show("agent", "confirm"), "next", k.show("agent", "cancel"), "cancel"))
	case modeNewName:
		return m.promptLine(w, newLabel("name"), hint(k.show("prompt", "confirm"), "next", k.show("prompt", "cancel"), "cancel"))
	case modeNewDir:
		right := hint(pick, "pick", k.show("prompt", "fill"), "fill", k.show("prompt", "browse"), "browse", k.show("prompt", "confirm"), "start")
		if m.err != "" {
			right = errStyle.Render(m.err)
		}
		return m.promptLine(w, newLabel("folder"), right)
	case modeGroupAdd:
		return m.promptLine(w, promptLabel.Render("add to group  "), hint(pick, "pick", k.show("prompt", "confirm"), "add", k.show("prompt", "cancel"), "cancel"))
	case modeGroupRemove:
		return m.promptLine(w, promptLabel.Render("remove from group  "), hint(pick, "pick", k.show("prompt", "confirm"), "remove", k.show("prompt", "cancel"), "cancel"))
	case modeFind:
		r, _ := m.findSelected()
		var keys string
		switch {
		case r.here:
			keys = hint(k.show("finder", "open"), "use this folder", k.show("finder", "parent"), "back", k.show("finder", "cancel"), "close")
		case r.dir:
			keys = hint(k.show("finder", "open"), "open", k.show("finder", "use"), "use", k.show("finder", "parent"), "back", k.show("finder", "cancel"), "close")
		default:
			keys = hint(k.show("finder", "parent"), "back", k.show("finder", "cancel"), "close")
		}
		return m.promptLine(w, promptLabel.Render("find  "), keys)
	case modeRead:
		return hint(k.pair("details", "up", "down"), "scroll", k.show("details", "copy"), "copy", k.show("details", "back"), "back")
	case modeHelp:
		return hint(k.pair("details", "up", "down"), "scroll", k.show("details", "back"), "back")
	}

	if m.status != "" {
		if m.statusBad {
			return errStyle.Render(truncate(m.status, w))
		}
		return statusStyle.Render(truncate(m.status, w))
	}
	if len(m.items) == 0 {
		return ""
	}
	entries := [][2]string{
		{k.show("list", "open"), "open"},
		{k.show("list", "new"), "new"},
		{k.show("list", "filter"), "filter"},
		{k.show("list", "folders"), "folders"},
		{k.show("list", "group_add"), "group"},
		{k.show("list", "rename"), "rename"},
		{k.show("list", "details"), "details"},
		{k.pair("list", "view_prev", "view_next"), "view"},
		{k.show("list", "help"), "keys"},
		{k.show("list", "quit"), "quit"},
	}
	var out string
	for _, e := range entries {
		if e[0] == "" {
			continue // unbound
		}
		next := helpKey.Render(e[0]) + " " + helpDesc.Render(e[1])
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
