// Package ui renders the session picker.
package ui

import "github.com/charmbracelet/lipgloss"

// Palette matches the user's kitty/Hyprland theme: dark, purple accent,
// light-blue secondary. Background is left unset so the terminal's own
// (translucent) background shows through.
var (
	Purple = lipgloss.Color("#7a1fff")
	Blue   = lipgloss.Color("#8ec9ff")
	Text   = lipgloss.Color("#cfd8e3")
	Muted  = lipgloss.Color("#6b7280")
	Dim    = lipgloss.Color("#9aa4b1")
	Line   = lipgloss.Color("#2e2a3d")
	Red    = lipgloss.Color("#ff5555")
	White  = lipgloss.Color("#ffffff")
)

var (
	// Chrome
	AppTitle = lipgloss.NewStyle().Foreground(White).Background(Purple).
			Bold(true).Padding(0, 1)
	Pane = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).
		BorderForeground(Line).Padding(0, 1)
	PaneLabel = lipgloss.NewStyle().Foreground(Purple).Bold(true)

	// List items
	Cursor       = lipgloss.NewStyle().Foreground(Purple).Bold(true)
	ItemTitle    = lipgloss.NewStyle().Foreground(Text)
	ItemTitleSel = lipgloss.NewStyle().Foreground(White).Bold(true)
	ItemMeta     = lipgloss.NewStyle().Foreground(Muted)
	ItemMetaSel  = lipgloss.NewStyle().Foreground(Dim)
	NamedDot     = lipgloss.NewStyle().Foreground(Purple)
	AutoDot      = lipgloss.NewStyle().Foreground(Muted)

	// Agent badges
	BadgeClaude = lipgloss.NewStyle().Foreground(Purple).Bold(true)
	BadgeCodex  = lipgloss.NewStyle().Foreground(Blue).Bold(true)

	// Detail pane
	DetailKey   = lipgloss.NewStyle().Foreground(Muted)
	DetailVal   = lipgloss.NewStyle().Foreground(Text)
	DetailTitle = lipgloss.NewStyle().Foreground(Purple).Bold(true)
	DetailHead  = lipgloss.NewStyle().Foreground(Blue)

	// Footer / prompts
	Help      = lipgloss.NewStyle().Foreground(Muted)
	HelpKey   = lipgloss.NewStyle().Foreground(Blue)
	Prompt    = lipgloss.NewStyle().Foreground(Purple).Bold(true)
	Err       = lipgloss.NewStyle().Foreground(Red)
	FilterHit = lipgloss.NewStyle().Foreground(Blue).Bold(true)
)

// Symbols used across the UI.
const (
	DotFilled = "●"
	DotHollow = "○"
	CursorBar = "▌"
	Sep       = "·"
	Ellipsis  = "…"
)
