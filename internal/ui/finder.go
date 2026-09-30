package ui

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// finder is a Telescope-style folder picker: one fuzzy prompt over every
// folder under a scope, best match at the bottom next to the prompt, and a
// preview of the highlighted folder on the right. → narrows the scope to
// the highlighted folder, ← widens it to the parent.
type finder struct {
	from    mode   // what the chosen folder is for
	saved   string // that prompt's input, restored on esc
	scope   string
	all     []string // folders under scope, absolute
	results []hit    // best first
	sel     int      // index into results
}

type hit struct {
	path  string // absolute
	pos   []int  // matched rune positions in the displayed (relative) path
	score int
}

const finderDepth = 6

// rel is how a result is shown: relative to the scope, "./" for the scope.
func (f finder) rel(p string) string {
	if p == f.scope {
		return "./"
	}
	r, err := filepath.Rel(f.scope, p)
	if err != nil {
		return p
	}
	return r + "/"
}

func (m *Model) openFinder(from mode, scope string) {
	m.find = finder{from: from, saved: m.input.Value()}
	m.mode = modeFind
	m.openInput("")
	m.setScope(scope)
}

func (m *Model) setScope(dir string) {
	m.find.scope = dir
	m.find.all = walkFolders(dir, finderDepth)
	m.input.SetValue("")
	m.refind()
}

// walkFolders lists folders under root breadth-first, nearest first.
func walkFolders(root string, depth int) []string {
	out := []string{}
	level := []string{root}
	for d := 0; d < depth && len(level) > 0; d++ {
		var next []string
		for _, dir := range level {
			entries, err := os.ReadDir(dir)
			if err != nil {
				continue
			}
			for _, e := range entries {
				if !e.IsDir() || skipDirs[e.Name()] {
					continue
				}
				p := filepath.Join(dir, e.Name())
				out = append(out, p)
				next = append(next, p)
				if len(out) >= indexMax {
					return out
				}
			}
		}
		level = next
	}
	return out
}

// refind ranks the scope's folders against the typed query.
func (m *Model) refind() {
	q := strings.TrimSpace(m.input.Value())
	f := &m.find
	f.results = f.results[:0]
	add := func(p string) {
		r := f.rel(p)
		if q == "" {
			// Nothing typed: the scope, then visible folders nearest first.
			if strings.Contains("/"+r, "/.") && p != f.scope {
				return
			}
			f.results = append(f.results, hit{path: p, score: len(f.results)})
			return
		}
		score, pos, ok := fuzzyPath(r, q)
		if !ok {
			return
		}
		if strings.Contains("/"+r, "/.") && !strings.Contains(q, ".") {
			score += 1000 // hidden folders rank below visible ones
		}
		f.results = append(f.results, hit{p, pos, score})
	}
	add(f.scope)
	for _, p := range f.all {
		add(p)
	}
	sort.SliceStable(f.results, func(a, b int) bool { return f.results[a].score < f.results[b].score })
	f.sel = 0
}

// fuzzyPath matches space-separated terms (all must match) as
// subsequences of s, fzf-style: tighter matches, matches that start a path
// segment and hits in the last segment rank higher, as do shorter paths.
func fuzzyPath(s, query string) (int, []int, bool) {
	hay := []rune(strings.ToLower(s))
	lastSeg := strings.LastIndex(strings.TrimSuffix(s, "/"), "/") + 1
	score := len(hay)
	var pos []int
	for _, term := range strings.Fields(strings.ToLower(query)) {
		want := []rune(term)
		best, bestPos := -1, []int(nil)
		for start := range hay {
			if hay[start] != want[0] {
				continue
			}
			p := []int{start}
			hi := start + 1
			for _, c := range want[1:] {
				for hi < len(hay) && hay[hi] != c {
					hi++
				}
				if hi == len(hay) {
					p = nil
					break
				}
				p = append(p, hi)
				hi++
			}
			if p == nil {
				break // later starts cannot match either
			}
			sc := (p[len(p)-1] - p[0]) * 4 // spread
			if start == 0 || hay[start-1] == '/' || hay[start-1] == '.' || hay[start-1] == '-' || hay[start-1] == '_' {
				sc -= 8 // starts a segment or word
			}
			if start >= lastSeg {
				sc -= 12 // in the folder's own name
			}
			if best < 0 || sc < best {
				best, bestPos = sc, p
			}
		}
		if best < 0 {
			return 0, nil, false
		}
		score += best
		pos = append(pos, bestPos...)
	}
	return score, pos, true
}

func (m Model) findSelected() (hit, bool) {
	if len(m.find.results) == 0 {
		return hit{}, false
	}
	return m.find.results[min(m.find.sel, len(m.find.results)-1)], true
}

func (m Model) updateFind(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	f := &m.find
	switch msg.String() {
	case "ctrl+c":
		return m.quit(false)
	case "esc":
		m.mode = f.from
		if f.from == modeList {
			m.closeInput()
		} else {
			m.openInput(f.saved)
		}
		return m, nil
	case "up", "ctrl+p", "ctrl+k": // up the screen: away from the best match
		f.sel = min(f.sel+1, max(0, len(f.results)-1))
		return m, nil
	case "down", "ctrl+n", "ctrl+j":
		f.sel = max(f.sel-1, 0)
		return m, nil
	case "right", "tab":
		if h, ok := m.findSelected(); ok && h.path != f.scope {
			m.setScope(h.path)
		}
		return m, nil
	case "left":
		if parent := filepath.Dir(f.scope); parent != f.scope {
			m.setScope(parent)
		}
		return m, nil
	case "enter":
		if h, ok := m.findSelected(); ok {
			return m.chooseFolder(h.path)
		}
		return m, nil
	}
	var cmd tea.Cmd
	before := m.input.Value()
	m.input, cmd = m.input.Update(msg)
	if m.input.Value() != before {
		m.refind()
	}
	return m, cmd
}

// chooseFolder hands the folder to whatever opened the finder: a new
// session's folder prompt, or the filter as an f: term.
func (m Model) chooseFolder(dir string) (tea.Model, tea.Cmd) {
	switch m.find.from {
	case modeNewDir:
		m.mode = modeNewDir
		m.openInput(collapseHome(dir))
	default: // list or filter: narrow the list to sessions in that folder
		v := strings.TrimSpace(m.find.saved)
		if v != "" {
			v += " "
		}
		m.query = v + "f:" + quoteTerm(collapseHome(dir))
		m.closeInput()
		m.reorder()
	}
	return m, nil
}

type dirEntry struct {
	name string
	dir  bool
}

// readEntries lists a folder, folders first, case-insensitively sorted.
func readEntries(dir string) []dirEntry {
	list, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	out := make([]dirEntry, 0, len(list))
	for _, e := range list {
		out = append(out, dirEntry{e.Name(), isDir(filepath.Join(dir, e.Name()))})
	}
	sort.SliceStable(out, func(a, b int) bool {
		if out[a].dir != out[b].dir {
			return out[a].dir
		}
		return strings.ToLower(out[a].name) < strings.ToLower(out[b].name)
	})
	return out
}
