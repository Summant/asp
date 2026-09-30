package ui

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/summant/asp/internal/jobs"
	"github.com/summant/asp/internal/source"
)

// Item is one row: a session plus whatever name and groups the user gave it,
// and the paused process running it, if any.
type Item struct {
	source.Session
	Name   string   // custom name, empty if none
	Groups []string // sorted group names
	Job    *jobs.Job
}

func (i Item) Named() bool { return i.Name != "" }

// placeholder reports whether this row stands for a new session that is
// running under asp but has not been written to disk yet.
func (i Item) placeholder() bool { return i.Session.ID == "" }

// Key identifies the item across reloads.
func (i Item) Key() string {
	if i.placeholder() && i.Job != nil {
		return fmt.Sprintf("job:%d", i.Job.N)
	}
	return string(i.Session.Agent) + ":" + i.Session.ID
}

func (i Item) paused() bool { return i.Job != nil && i.Job.State == jobs.Paused }

// Opening is the first message, cleaned up for display.
func (i Item) Opening() string { return cleanOpening(i.Session.Opening) }

// Title is the custom name if set, else the opening message.
func (i Item) Title() string {
	switch {
	case i.Name != "":
		return i.Name
	case i.placeholder():
		return "new session"
	}
	if o := i.Opening(); o != "" {
		return o
	}
	return "(no messages)"
}

// Haystack is what free filter terms match against: title, path, agent and
// groups, so "codex agent" narrows by agent and folder at once.
func (i Item) Haystack() string {
	return strings.ToLower(i.Title() + " " + i.Session.CWD + " " + i.Session.Agent.Label() + " " + strings.Join(i.Groups, " "))
}

// Item geometry within the list column (SPEC §3.3). The tag column is a
// fixed 7 cells so every title starts in the same column whatever the agent.
const (
	gutterW  = 2                  // "│ " or "  "
	markW    = 2                  // "● " or "○ "
	tagW     = 7                  // "claude " / "codex  "
	metaCol  = gutterW + markW    // metadata aligns under the tag
	titleCol = metaCol + tagW + 1 // one space after the tag column
)

type itemState int

const (
	plain itemState = iota
	matched
	selected
)

// renderItem draws the item's two lines, each exactly width cells.
func (i Item) renderItem(width int, st itemState) [2]string {
	gutter := "  "
	title, meta := titleStyle, metaStyle
	switch st {
	case selected:
		gutter = gutterSel.Render(gutterBar) + " "
		title, meta = titleSel, metaSel
	case matched:
		gutter = gutterMatch.Render(gutterBar) + " "
	}

	mark := autoMark.Render(markAuto)
	if i.Named() {
		mark = namedMark.Render(markNamed)
	}
	tag := tagClaude
	if i.Session.Agent == source.Codex {
		tag = tagCodex
	}

	line1 := gutter + mark + " " + tag.Render(padRight(i.Session.Agent.Label(), tagW)) + " " +
		title.Render(truncate(i.Title(), width-titleCol))

	s := "  " + sep + "  "
	count := humanCount(i.Session.Messages) + " msg"
	if i.placeholder() {
		count = "new"
	}
	tail := s + count + s + ago(i.Session.Modified)
	for n, g := range i.Groups {
		if n == 0 {
			tail += s
		} else {
			tail += " "
		}
		tail += "#" + g
	}
	flag := ""
	if i.paused() {
		flag = s + "paused"
	}
	path := collapseHome(i.Session.CWD)
	avail := width - metaCol
	var body string
	if room := avail - lipgloss.Width(tail+flag); room >= 8 {
		body = truncateLeft(path, room) + tail
	} else { // narrow: keep the leaf folder, cut the rest
		body = truncate(truncateLeft(path, 12)+tail, max(0, avail-lipgloss.Width(flag)))
		flag = truncate(flag, avail-lipgloss.Width(body))
	}
	line2 := gutter + strings.Repeat(" ", metaCol-gutterW) + meta.Render(body)
	if flag != "" {
		line2 += statusStyle.Render(flag)
	}

	return [2]string{padRight(line1, width), padRight(line2, width)}
}

// truncate cuts s to at most w display cells, ending in "…" if cut.
func truncate(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= w {
		return s
	}
	return ansi.Truncate(s, w, ellipsis)
}

// truncateLeft keeps the end of s — for paths, where the leaf folder is the
// identifying part.
func truncateLeft(s string, w int) string {
	if w <= 0 {
		return ""
	}
	n := lipgloss.Width(s)
	if n <= w {
		return s
	}
	if w == 1 {
		return ellipsis
	}
	// Drop cells from the left until the rest fits beside the ellipsis. A
	// wide character straddling the cut may not be removable by one cell,
	// so widen the cut rather than repeating a call that cannot progress.
	for drop := n - w + 1; drop <= n; drop++ {
		if cut := ansi.TruncateLeft(s, drop, ""); lipgloss.Width(cut) <= w-1 {
			return ellipsis + cut
		}
	}
	return ellipsis
}

// padRight pads s with spaces to exactly w cells (truncating if longer).
func padRight(s string, w int) string {
	n := lipgloss.Width(s)
	if n > w {
		return ansi.Truncate(s, w, "")
	}
	return s + strings.Repeat(" ", w-n)
}

func collapseHome(p string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return p
	}
	if p == home {
		return "~"
	}
	if strings.HasPrefix(p, home+"/") {
		return "~" + strings.TrimPrefix(p, home)
	}
	return p
}

func humanCount(n int) string {
	if n < 1000 {
		return fmt.Sprint(n)
	}
	return fmt.Sprintf("%.1fk", float64(n)/1000)
}

func ago(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	case d < 7*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	default:
		return t.Format("Jan 2")
	}
}
