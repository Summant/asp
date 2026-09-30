package ui

import "strings"

// match reports whether every space-separated term in the query appears as a
// subsequence of the haystack, so "cdx dot" finds a codex session in dotfiles.
// Returns a score: lower is better (tighter matches rank higher).
func match(haystack, query string) (int, bool) {
	q := strings.Fields(strings.ToLower(query))
	if len(q) == 0 {
		return 0, true
	}
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

// subsequence finds term's characters in order within s, scoring by how
// spread out the match is. An exact substring scores best.
func subsequence(s, term string) (int, bool) {
	if idx := strings.Index(s, term); idx >= 0 {
		return idx, true // contiguous match: score by how early it appears
	}
	si, start, last := 0, -1, -1
	for _, c := range term {
		found := false
		for ; si < len(s); si++ {
			if rune(s[si]) == c {
				if start < 0 {
					start = si
				}
				last = si
				si++
				found = true
				break
			}
		}
		if !found {
			return 0, false
		}
	}
	return 1000 + (last - start), true // scattered match ranks below any substring
}
