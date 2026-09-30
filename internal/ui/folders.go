package ui

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// indexMsg carries the folder index built in the background.
type indexMsg []string

const (
	indexDepth = 4
	indexMax   = 20000
)

// skipDirs are never worth suggesting and can be huge.
var skipDirs = map[string]bool{
	".git": true, "node_modules": true, ".cache": true, ".npm": true, ".cargo": true,
	".rustup": true, ".mozilla": true, ".steam": true, ".var": true, ".nv": true,
	".pki": true, ".gnupg": true, "__pycache__": true, ".venv": true, "venv": true,
	"target": true, ".gradle": true, ".m2": true, "Trash": true, "snap": true,
	".local": true, "go": true, ".claude": true, ".codex": true,
}

// startIndex begins indexing folders under Home once per run.
func (m *Model) startIndex() tea.Cmd {
	if m.folders != nil || m.indexing || m.deps.Home == "" {
		return nil
	}
	m.indexing = true
	root := m.deps.Home
	return func() tea.Msg { return indexMsg(indexFolders(root)) }
}

// indexFolders lists folders under root breadth-first, a few levels deep.
func indexFolders(root string) []string {
	out := []string{}
	level := []string{root}
	for depth := 0; depth < indexDepth && len(level) > 0; depth++ {
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

// folderSuggestions offers folders for the new-session prompt: recent
// session folders when empty; children of the typed folder for a path;
// otherwise a fuzzy match over recent folders and the index, recent first.
func (m Model) folderSuggestions(v string) []string {
	recent := m.sessionFolders()
	if v == "" {
		return recent[:min(maxSugg, len(recent))]
	}
	if strings.HasPrefix(v, "/") || strings.HasPrefix(v, "~") || strings.HasPrefix(v, ".") {
		return childSuggestions(v)
	}
	type hit struct {
		s     string
		score int
	}
	seen := map[string]bool{}
	var hits []hit
	add := func(p string, bonus int) {
		if seen[p] {
			return
		}
		seen[p] = true
		s, ok := match(p, v)
		if !ok {
			return
		}
		if strings.Contains(p, "/.") && !strings.Contains(v, ".") {
			s += 1 << 20 // hidden folders rank below visible ones
		}
		hits = append(hits, hit{p, s + bonus + len(p)})
	}
	for _, p := range recent {
		add(p, 0)
	}
	for _, p := range m.folders {
		add(collapseHome(p), 1<<16)
	}
	sort.SliceStable(hits, func(a, b int) bool { return hits[a].score < hits[b].score })
	out := make([]string, 0, maxSugg)
	for _, h := range hits[:min(maxSugg, len(hits))] {
		out = append(out, h.s)
	}
	return out
}

// childSuggestions lists subfolders of the folder being typed whose names
// start with what follows the last "/" — prefix matches first, then fuzzy.
func childSuggestions(v string) []string {
	full := expand(v)
	dir, base := filepath.Dir(full), filepath.Base(full)
	if strings.HasSuffix(v, "/") || v == "~" {
		dir, base = full, ""
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var names []string
	for _, e := range entries {
		n := e.Name()
		if strings.HasPrefix(n, ".") && !strings.HasPrefix(base, ".") {
			continue
		}
		if isDir(filepath.Join(dir, n)) {
			names = append(names, n)
		}
	}
	sort.Slice(names, func(a, b int) bool { return strings.ToLower(names[a]) < strings.ToLower(names[b]) })
	lb := strings.ToLower(base)
	var prefix, fuzzy []string
	for _, n := range names {
		p := collapseHome(filepath.Join(dir, n))
		if strings.HasPrefix(strings.ToLower(n), lb) {
			prefix = append(prefix, p)
		} else if _, ok := match(n, base); ok {
			fuzzy = append(fuzzy, p)
		}
	}
	out := append(prefix, fuzzy...)
	return out[:min(maxSugg, len(out))]
}

// samePath compares folders, allowing for an agent having recorded the
// symlink-resolved form of the folder it was started in.
func samePath(a, b string) bool {
	if filepath.Clean(a) == filepath.Clean(b) {
		return true
	}
	ra, err1 := filepath.EvalSymlinks(a)
	rb, err2 := filepath.EvalSymlinks(b)
	return err1 == nil && err2 == nil && ra == rb
}
