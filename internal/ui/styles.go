// Package ui renders the session picker.
package ui

import "github.com/charmbracelet/lipgloss"

// Colour roles are glow's; the values are the user's kitty/Hyprland palette
// (SPEC §5). No style sets a background except the logo pill: the terminal is
// translucent and a painted background would show as a solid block.
var (
	accent      = lipgloss.Color("#7a1fff")
	accentMuted = lipgloss.Color("#9d7bff")
	secondary   = lipgloss.Color("#8ec9ff")
	text        = lipgloss.Color("#cfd8e3")
	textStrong  = lipgloss.Color("#ffffff")
	muted       = lipgloss.Color("#6b7280")
	subtle      = lipgloss.Color("#4a4560")
	rule        = lipgloss.Color("#2e2a3d")
	dotActive   = lipgloss.Color("#9aa4b1")
	errColour   = lipgloss.Color("#ff5555")
)

func fg(c lipgloss.Color) lipgloss.Style { return lipgloss.NewStyle().Foreground(c) }

var (
	// Chrome. The logo is the only bold text and the only background.
	logoStyle    = lipgloss.NewStyle().Foreground(textStrong).Background(accent).Bold(true).Padding(0, 1)
	summaryStyle = fg(subtle)
	viewOnStyle  = fg(text)
	ruleStyle    = fg(rule)

	// List items.
	gutterSel   = fg(accent)
	gutterMatch = fg(secondary)
	titleStyle  = fg(text)
	titleSel    = fg(accent)
	metaStyle   = fg(muted)
	metaSel     = fg(accentMuted)
	namedMark   = fg(accent)
	autoMark    = fg(muted)
	tagClaude   = fg(accent)
	tagCodex    = fg(secondary)

	// Pagination.
	dotOn  = fg(dotActive)
	dotOff = fg(rule)

	// Detail pane.
	detailKey  = fg(muted)
	detailVal  = fg(text)
	detailHead = fg(secondary)

	// Footer and prompts.
	helpKey     = fg(secondary)
	helpDesc    = fg(muted)
	promptLabel = fg(accent)
	statusStyle = fg(secondary)
	errStyle    = fg(errColour)
)

// Every symbol here renders at one cell in JetBrainsMono Nerd Font; no Nerd
// Font private-use icons, which widen inconsistently.
const (
	gutterBar = "│"
	markNamed = "●"
	markAuto  = "○"
	dot       = "•"
	sep       = "·"
	ellipsis  = "…"
)
