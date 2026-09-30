package ui

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// skipDirs are never worth listing and can be huge.
var skipDirs = map[string]bool{".git": true, "node_modules": true}

// folderSuggestions offers folders for the new-session prompt, never
// deeper than one level below what is typed: recent session folders when
// empty; the subfolders of a typed path; otherwise recent folders and the
// folders directly in ~ that match the typed word.
func (m Model) folderSuggestions(v string) []string {
	recent := m.sessionFolders()
	if v == "" {
		return recent[:min(maxSugg, len(recent))]
	}
	if strings.HasPrefix(v, "/") || strings.HasPrefix(v, "~") || strings.HasPrefix(v, ".") {
		return childSuggestions(v)
	}
	cands := append([]string{}, recent...)
	for _, e := range readEntries(m.deps.Home) {
		if e.dir && !strings.HasPrefix(e.name, ".") {
			cands = append(cands, collapseHome(filepath.Join(m.deps.Home, e.name)))
		}
	}
	seen := map[string]bool{}
	var uniq []string
	for _, c := range cands {
		if !seen[c] {
			seen[c] = true
			uniq = append(uniq, c)
		}
	}
	return rank(uniq, v, maxSugg)
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
