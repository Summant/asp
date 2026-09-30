package ui

import (
	"regexp"
	"strings"
)

var (
	// [AGENTS.md](http://AGENTS.md) — link artefacts that pasting adds.
	mdLink = regexp.MustCompile(`\[([^\]\n]*)\]\([^)\s]*\)`)
	// <user-prompt>, </agent-reply>, … — wrapper tags around injected text.
	xmlTag = regexp.MustCompile(`</?[A-Za-z][A-Za-z0-9_-]*>`)
	// Pasted dividers: runs of box-drawing or rule characters.
	divider = regexp.MustCompile(`[─━═┄┅┈┉╌╍—_=~*#-]{3,}`)
	// 1\. or \* — markdown escapes.
	mdEscape = regexp.MustCompile(`\\([\\.*_#>\[\]()!-])`)
)

// cleanOpening makes a first message readable as a label. It only changes
// what is displayed and searched; transcripts are never touched.
func cleanOpening(s string) string {
	s = mdLink.ReplaceAllString(s, "$1")
	s = xmlTag.ReplaceAllString(s, " ")
	s = divider.ReplaceAllString(s, " ")
	s = mdEscape.ReplaceAllString(s, "$1")
	s = strings.ReplaceAll(s, "**", "")
	s = strings.Join(strings.Fields(s), " ")
	return strings.TrimRight(s, " :;,-–—")
}
