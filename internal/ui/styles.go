// Package ui renders the session picker.
package ui

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/summant/asp/internal/config"
)

// Colour roles are glow's; the values come from config.toml, defaulting to
// the user's kitty/Hyprland palette (SPEC §5). No style sets a background
// except the logo pill: the terminal is translucent and a painted
// background would show as a solid block.
var (
	accent, accentMuted, secondary, text, textStrong lipgloss.Color
	muted, subtle, rule, dotActive, errColour        lipgloss.Color
	groupColour                                      lipgloss.Color

	// Chrome. The logo is the only bold text and the only background.
	logoStyle, summaryStyle, viewOnStyle, ruleStyle lipgloss.Style

	// List items.
	gutterSel, gutterMatch, titleStyle, titleSel lipgloss.Style
	metaStyle, metaSel, namedMark, autoMark      lipgloss.Style
	tagClaude, tagCodex, groupTag                lipgloss.Style

	// Pagination.
	dotOn, dotOff lipgloss.Style

	// Detail pane.
	detailKey, detailVal, detailHead lipgloss.Style

	// Footer and prompts.
	helpKey, helpDesc, promptLabel, statusStyle, errStyle lipgloss.Style
)

func init() { ApplyTheme(config.Default().Colors) }

func fg(c lipgloss.Color) lipgloss.Style { return lipgloss.NewStyle().Foreground(c) }

// ApplyTheme sets every colour from a role → colour map (see config.Colors).
// Call it before the program starts.
func ApplyTheme(c map[string]string) {
	accent = lipgloss.Color(c["accent"])
	accentMuted = lipgloss.Color(c["accent_muted"])
	secondary = lipgloss.Color(c["secondary"])
	text = lipgloss.Color(c["text"])
	textStrong = lipgloss.Color(c["text_strong"])
	muted = lipgloss.Color(c["muted"])
	subtle = lipgloss.Color(c["subtle"])
	rule = lipgloss.Color(c["rule"])
	dotActive = lipgloss.Color(c["dot_active"])
	errColour = lipgloss.Color(c["error"])
	groupColour = lipgloss.Color(c["group"])

	logoStyle = lipgloss.NewStyle().Foreground(textStrong).Background(accent).Bold(true).Padding(0, 1)
	summaryStyle = fg(subtle)
	viewOnStyle = fg(text)
	ruleStyle = fg(rule)

	gutterSel = fg(accent)
	gutterMatch = fg(secondary)
	titleStyle = fg(text)
	titleSel = fg(accent)
	metaStyle = fg(muted)
	metaSel = fg(accentMuted)
	namedMark = fg(accent)
	autoMark = fg(muted)
	tagClaude = fg(accent)
	tagCodex = fg(secondary)
	groupTag = fg(groupColour)

	dotOn = fg(dotActive)
	dotOff = fg(rule)

	detailKey = fg(muted)
	detailVal = fg(text)
	detailHead = fg(secondary)

	helpKey = fg(secondary)
	helpDesc = fg(muted)
	promptLabel = fg(accent)
	statusStyle = fg(secondary)
	errStyle = fg(errColour)
}

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
