package ui

import (
	"sort"
	"strings"
)

// A query is space-separated terms, all of which must match (AND):
//
//	waybar        title, folder, agent or group
//	f:dotfiles    folder only
//	g:arch        group name only
//
// Double quotes keep spaces inside one term: f:"Arch Ricing".
type query struct {
	free, path, group []string
}

func parseQuery(s string) query {
	var q query
	for _, tok := range tokenize(strings.ToLower(s)) {
		switch {
		case strings.HasPrefix(tok, "f:"):
			if t := tok[2:]; t != "" {
				q.path = append(q.path, t)
			}
		case strings.HasPrefix(tok, "g:"):
			if t := tok[2:]; t != "" {
				q.group = append(q.group, t)
			}
		default:
			q.free = append(q.free, tok)
		}
	}
	return q
}

// tokenize splits on spaces outside double quotes and drops the quotes.
func tokenize(s string) []string {
	var out []string
	var b strings.Builder
	quoted, any := false, false
	for _, r := range s {
		switch {
		case r == '"':
			quoted, any = !quoted, true
		case r == ' ' && !quoted:
			if any {
				out = append(out, b.String())
			}
			b.Reset()
			any = false
		default:
			b.WriteRune(r)
			any = true
		}
	}
	if any {
		out = append(out, b.String())
	}
	return out
}

// quoteTerm quotes a term containing spaces so tokenize keeps it whole.
func quoteTerm(s string) string {
	if strings.ContainsRune(s, ' ') {
		return `"` + s + `"`
	}
	return s
}

// matchItem scores an item against a query; lower is better.
func matchItem(it Item, q query) (int, bool) {
	total := 0
	for _, t := range q.free {
		s, ok := subsequence(it.Haystack(), t)
		if !ok {
			return 0, false
		}
		total += s
	}
	for _, t := range q.path {
		s, ok := best(t, strings.ToLower(collapseHome(it.Session.CWD)), strings.ToLower(it.Session.CWD))
		if !ok {
			return 0, false
		}
		total += s
	}
	for _, t := range q.group {
		lower := make([]string, len(it.Groups))
		for i, g := range it.Groups {
			lower[i] = strings.ToLower(g)
		}
		s, ok := best(t, lower...)
		if !ok {
			return 0, false
		}
		total += s
	}
	return total, true
}

// best is the lowest score of term across several haystacks.
func best(term string, hays ...string) (int, bool) {
	score, found := 0, false
	for _, h := range hays {
		if s, ok := subsequence(h, term); ok && (!found || s < score) {
			score, found = s, true
		}
	}
	return score, found
}

// match reports whether every space-separated term in the query matches the
// haystack (AND), case-insensitively.
func match(haystack, query string) (int, bool) {
	q := strings.Fields(strings.ToLower(query))
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

// subsequence finds term's runes in order within s. A substring hit scores
// by position; a scattered hit ranks below every substring hit and must be
// tight — its letters within a few characters of each other — so that a
// short term does not match somewhere across a 3,000-character message.
func subsequence(s, term string) (int, bool) {
	if idx := strings.Index(s, term); idx >= 0 {
		return idx, true
	}
	want := []rune(term)
	if len(want) == 0 {
		return 0, true
	}
	hay := []rune(s)
	limit := maxSpan(len(want))
	// Try each occurrence of the first rune, scanning at most limit runes
	// ahead, and keep the tightest window: O(len(s) × limit).
	bestSpan := -1
	for start := 0; start < len(hay); start++ {
		if hay[start] != want[0] {
			continue
		}
		end := min(len(hay), start+limit)
		hi, ok := start+1, true
		for _, c := range want[1:] {
			for hi < end && hay[hi] != c {
				hi++
			}
			if hi == end {
				ok = false
				break
			}
			hi++
		}
		if ok && (bestSpan < 0 || hi-start < bestSpan) {
			bestSpan = hi - start
		}
	}
	if bestSpan < 0 {
		return 0, false
	}
	return scattered + bestSpan, true
}

func maxSpan(n int) int { return 3*n + 2 }

// scattered is added to a non-contiguous match's score so it ranks below a
// substring hit at any position, however long the haystack.
const scattered = 1 << 32

func sortStable[T any](s []T, less func(a, b T) bool) {
	sort.SliceStable(s, func(i, j int) bool { return less(s[i], s[j]) })
}
