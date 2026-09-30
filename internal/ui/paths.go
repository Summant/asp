package ui

import (
	"os"
	"path/filepath"
	"strings"
)

// expand resolves "~" and relative paths to an absolute path.
func expand(p string) string {
	p = strings.TrimSpace(p)
	if p == "~" || strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			p = home + p[1:]
		}
	}
	if abs, err := filepath.Abs(p); err == nil {
		return abs
	}
	return p
}

func isDir(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}

// completeDir extends a folder path as far as it is unambiguous, shell-style:
// a unique match gets a trailing "/", several extend to their common prefix.
// Hidden folders are offered only once a "." has been typed.
func completeDir(in string) string {
	full := expand(in)
	dir, base := filepath.Dir(full), filepath.Base(full)
	if in == "" || strings.HasSuffix(in, "/") || in == "~" {
		dir, base = full, ""
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return in
	}
	var names []string
	for _, e := range entries {
		n := e.Name()
		if !strings.HasPrefix(n, base) || (strings.HasPrefix(n, ".") && !strings.HasPrefix(base, ".")) {
			continue
		}
		if isDir(filepath.Join(dir, n)) { // follows symlinks, unlike e.IsDir()
			names = append(names, n)
		}
	}
	if len(names) == 0 {
		return in
	}
	prefix := []rune(names[0]) // trimmed by rune so a name never splits mid-character
	for _, n := range names[1:] {
		for !strings.HasPrefix(n, string(prefix)) {
			prefix = prefix[:len(prefix)-1]
		}
	}
	out := filepath.Join(dir, string(prefix))
	if len(names) == 1 {
		out += "/"
	}
	return collapseHome(out)
}
