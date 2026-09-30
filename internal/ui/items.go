package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/summant/asp/internal/source"
)

// Item is one row: a session plus whatever name the user gave it.
type Item struct {
	source.Session
	Name string // custom name, empty if none
}

func (i Item) Named() bool { return i.Name != "" }

// Title is the custom name if set, else the opening message.
func (i Item) Title() string {
	if i.Name != "" {
		return i.Name
	}
	if i.Session.Opening != "" {
		return i.Session.Opening
	}
	return "(no messages)"
}

// Haystack is what the fuzzy filter matches against: title, path and agent,
// so "codex dotfiles" narrows by both at once.
func (i Item) Haystack() string {
	return strings.ToLower(i.Title() + " " + i.Session.CWD + " " + i.Session.Agent.Label())
}

func (i Item) badge() string {
	switch i.Session.Agent {
	case source.Codex:
		return BadgeCodex.Render("codex")
	default:
		return BadgeClaude.Render("claude")
	}
}

// Render draws the two-line entry. width is the usable inner width.
func (i Item) Render(width int, selected bool) string {
	dot := AutoDot.Render(DotHollow)
	if i.Named() {
		dot = NamedDot.Render(DotFilled)
	}
	bar := "  "
	titleStyle, metaStyle := ItemTitle, ItemMeta
	if selected {
		bar = Cursor.Render(CursorBar) + " "
		titleStyle, metaStyle = ItemTitleSel, ItemMetaSel
	}

	// Line 1: cursor, marker, badge, title (truncated to fit).
	head := bar + dot + " " + i.badge() + " "
	room := width - lipgloss.Width(head)
	title := truncate(i.Title(), room)

	// Line 2: path and metadata, indented under the title.
	meta := fmt.Sprintf("%s  %s  %s msg  %s  %s",
		collapseHome(i.Session.CWD), Sep, humanCount(i.Session.Messages), Sep, ago(i.Session.Modified))
	meta = truncate(meta, width-4)

	return head + titleStyle.Render(title) + "\n" +
		"    " + metaStyle.Render(meta)
}

// truncate cuts to a display width, not a byte or rune count, so wide
// characters and ANSI-styled text line up correctly.
func truncate(s string, max int) string {
	if max <= 1 {
		return ""
	}
	if lipgloss.Width(s) <= max {
		return s
	}
	r := []rune(s)
	for len(r) > 0 && lipgloss.Width(string(r))+1 > max {
		r = r[:len(r)-1]
	}
	return string(r) + Ellipsis
}

func collapseHome(p string) string {
	if home, err := homeDir(); err == nil && strings.HasPrefix(p, home) {
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
