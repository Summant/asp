package ui

import "strings"

// match reports whether every space-separated term in the query appears as a
// case-insensitive subsequence of the haystack (AND), so "cdx dot" finds a
// codex session in dotfiles. The score is lower for tighter matches: any
// substring hit outranks any scattered one.
func match(haystack, query string) (int, bool) {
	q := strings.Fields(strings.ToLower(query))
	if len(q) == 0 {
		return 0, true
	}
	haystack = strings.ToLower(haystack)
	total := 0
	for _, term := range q {
		score, ok := subsequence(haystack, term)
		if !ok {
			return 0, false
		}
		total += score
	}
	return total, true
}

// subsequence finds term's runes in order within s, scoring by how spread
// out the match is. An exact substring scores by position alone.
func subsequence(s, term string) (int, bool) {
	if idx := strings.Index(s, term); idx >= 0 {
		return idx, true
	}
	hay := []rune(s)
	hi, start, last := 0, -1, -1
	for _, c := range term {
		for hi < len(hay) && hay[hi] != c {
			hi++
		}
		if hi == len(hay) {
			return 0, false
		}
		if start < 0 {
			start = hi
		}
		last = hi
		hi++
	}
	return scattered + (last - start), true
}

// scattered is added to a non-contiguous match's score so it ranks below a
// substring hit at any position, however long the haystack.
const scattered = 1 << 32
