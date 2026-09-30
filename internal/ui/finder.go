package ui

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// finder is a folder browser drawn like glow's file list: one folder at a
// time, each entry a two-line item, the selection marked by the gutter bar.
// Typing narrows the entries of the current folder only; entering a folder
// is the only way deeper.
type finder struct {
	from    mode   // what the chosen folder is for
	saved   string // that prompt's input, restored on esc
	dir     string
	entries []entry
	sel     int // index into visible()
}

type entry struct {
	name string
	dir  bool
	size int64
	mod  time.Time
}

// row is an entry as listed: possibly the "use this folder" row, with the
// positions of matched letters in its name.
type row struct {
	entry
	here bool
	pos  []int
}

func (m *Model) openFinder(from mode, dir string) {
	m.find = finder{from: from, saved: m.input.Value()}
	m.mode = modeFind
	m.openInput("")
	m.enter(dir)
}

// enter shows dir, remembering nothing of the previous folder's filter.
func (m *Model) enter(dir string) {
	m.find.dir = dir
	m.find.entries = readEntries(dir)
	m.find.sel = 0
	m.input.SetValue("")
}

// readEntries lists a folder: folders first, then files, by name.
func readEntries(dir string) []entry {
	list, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	out := make([]entry, 0, len(list))
	for _, e := range list {
		p := filepath.Join(dir, e.Name())
		fi, err := os.Stat(p) // follows symlinks
		if err != nil {
			continue
		}
		out = append(out, entry{e.Name(), fi.IsDir(), fi.Size(), fi.ModTime()})
	}
	sort.SliceStable(out, func(a, b int) bool {
		if out[a].dir != out[b].dir {
			return out[a].dir
		}
		return strings.ToLower(out[a].name) < strings.ToLower(out[b].name)
	})
	return out
}

// visible is the "use this folder" row, then the entries matching the typed
// filter — best match first while filtering. Hidden entries appear once the
// filter starts with ".".
func (f finder) visible(q string) []row {
	out := []row{{here: true, entry: entry{name: ".", dir: true}}}
	type scored struct {
		row
		score int
	}
	var hits []scored
	for _, e := range f.entries {
		if strings.HasPrefix(e.name, ".") && !strings.HasPrefix(q, ".") {
			continue
		}
		score, pos, ok := fuzzyName(e.name, q)
		if ok {
			hits = append(hits, scored{row{entry: e, pos: pos}, score})
		}
	}
	if q != "" {
		sort.SliceStable(hits, func(a, b int) bool { return hits[a].score < hits[b].score })
	}
	for _, h := range hits {
		out = append(out, h.row)
	}
	return out
}

// fuzzyName matches q as a subsequence of name, fzf-style: tight matches,
// matches at the start of a word and shorter names rank higher.
func fuzzyName(name, q string) (int, []int, bool) {
	want := []rune(strings.ToLower(q))
	if len(want) == 0 {
		return 0, nil, true
	}
	hay := []rune(strings.ToLower(name))
	best, bestPos, found := 0, []int(nil), false
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
		sc := (p[len(p)-1]-p[0])*4 + start
		if start == 0 || strings.ContainsRune(".-_ ", hay[start-1]) {
			sc -= 8
		}
		if !found || sc < best {
			best, bestPos, found = sc, p, true
		}
	}
	if !found {
		return 0, nil, false
	}
	return best + len(hay), bestPos, true
}

func (m Model) findRows() []row { return m.find.visible(m.input.Value()) }

func (m Model) findSelected() (row, bool) {
	rows := m.findRows()
	if len(rows) == 0 {
		return row{}, false
	}
	return rows[min(m.find.sel, len(rows)-1)], true
}

// findPerPage is how many items fit under the breadcrumb.
func (m Model) findPerPage() int { return max(1, (m.bodyRows()-2)/itemRows) }

func (m Model) updateFind(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	f := &m.find
	rows := m.findRows()
	sel, _ := m.findSelected()
	per := m.findPerPage()
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
	case "down", "ctrl+n", "ctrl+j":
		f.sel = min(f.sel+1, len(rows)-1)
		return m, nil
	case "up", "ctrl+p", "ctrl+k":
		f.sel = max(f.sel-1, 0)
		return m, nil
	case "pgdown":
		if next := (f.sel/per + 1) * per; next < len(rows) {
			f.sel = next
		}
		return m, nil
	case "pgup":
		f.sel = max(0, (f.sel/per-1)*per)
		return m, nil
	case "enter", "right":
		switch {
		case sel.here:
			if msg.String() == "enter" {
				return m.chooseFolder(f.dir)
			}
		case sel.dir:
			m.enter(filepath.Join(f.dir, sel.name))
		}
		return m, nil
	case "tab": // use the highlighted folder without going into it
		if sel.dir {
			if sel.here {
				return m.chooseFolder(f.dir)
			}
			return m.chooseFolder(filepath.Join(f.dir, sel.name))
		}
		return m, nil
	case "left":
		m.up()
		return m, nil
	case "backspace":
		if m.input.Value() == "" {
			m.up()
			return m, nil
		}
	}
	before := m.input.Value()
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	if m.input.Value() != before {
		// Typing jumps to the best match, past "use this folder".
		f.sel = 0
		if m.input.Value() != "" && len(m.findRows()) > 1 {
			f.sel = 1
		}
	}
	return m, cmd
}

// up goes to the parent folder with the folder just left selected.
func (m *Model) up() {
	f := &m.find
	parent := filepath.Dir(f.dir)
	if parent == f.dir {
		return
	}
	left := filepath.Base(f.dir)
	m.enter(parent)
	for i, r := range m.findRows() {
		if r.name == left && !r.here {
			f.sel = i
		}
	}
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

// finderRows draws the browser in the list column, glow-style: the folder
// as a breadcrumb, a page of two-line items, pagination dots at the bottom.
func (m Model) finderRows(w, n int) []string {
	f := m.find
	rows := []string{m.breadcrumb(w), ""}
	all := m.findRows()
	per := m.findPerPage()
	start := f.sel / per * per
	filtering := m.input.Value() != ""
	for i := start; i < min(start+per, len(all)); i++ {
		st := plain
		if i == f.sel {
			st = selected
		} else if filtering && !all[i].here {
			st = matched
		}
		l := m.finderItem(all[i], w, st)
		rows = append(rows, l[0], l[1], "")
	}
	if len(all) == 1 {
		msg := "empty folder"
		if filtering {
			msg = fmt.Sprintf("nothing here matches %q", m.input.Value())
		}
		rows = append(rows, "  "+metaStyle.Render(msg))
	}
	for len(rows) < n-1 {
		rows = append(rows, "")
	}
	rows = rows[:n-1]
	return append(rows, pageDots(len(all), per, f.sel, w))
}

// breadcrumb shows the folder being browsed: the path in muted text with
// its last part picked out.
func (m Model) breadcrumb(w int) string {
	p := collapseHome(m.find.dir)
	parent, leaf := filepath.Split(p)
	if leaf == "" { // "/" or "~"
		parent, leaf = "", p
	}
	count := summaryStyle.Render(fmt.Sprintf("  %s  %s", sep, countLabel(m.find.entries)))
	parent = truncateLeft(parent, max(0, w-lipgloss.Width(leaf)-lipgloss.Width(count)))
	return metaStyle.Render(parent) + metaSel.Render(leaf) + count
}

func (m Model) finderItem(r row, w int, st itemState) [2]string {
	gutter := "  "
	title, meta := titleStyle, metaStyle
	switch st {
	case selected:
		gutter = gutterSel.Render(gutterBar) + " "
		title, meta = titleSel, metaSel
	case matched:
		gutter = gutterMatch.Render(gutterBar) + " "
	}
	var name, info string
	switch {
	case r.here:
		name = "Use this folder"
		info = truncateLeft(collapseHome(m.find.dir), w-gutterW)
	case r.dir:
		name = r.name + "/"
		info = countLabel(readEntries(filepath.Join(m.find.dir, r.name))) + "  " + sep + "  " + ago(r.mod)
	default:
		name = r.name
		info = humanSize(r.size) + "  " + sep + "  " + ago(r.mod)
	}
	if !r.dir && st == plain {
		title = metaStyle // files are context, folders are the choices
	}
	line1 := gutter + highlight(truncate(name, w-gutterW), r.pos, title)
	line2 := gutter + meta.Render(truncate(info, w-gutterW))
	return [2]string{padRight(line1, w), padRight(line2, w)}
}

// highlight renders s in style with the runes at pos in the secondary colour.
func highlight(s string, pos []int, style lipgloss.Style) string {
	if len(pos) == 0 {
		return style.Render(s)
	}
	hl := map[int]bool{}
	for _, p := range pos {
		hl[p] = true
	}
	var b strings.Builder
	var run []rune
	flush := func() {
		if len(run) > 0 {
			b.WriteString(style.Render(string(run)))
			run = run[:0]
		}
	}
	for i, r := range []rune(s) {
		if hl[i] {
			flush()
			b.WriteString(statusStyle.Render(string(r)))
		} else {
			run = append(run, r)
		}
	}
	flush()
	return b.String()
}

// finderPreview fills the detail pane: a folder's contents, or the start
// of a text file.
func (m Model) finderPreview(w int) []string {
	r, ok := m.findSelected()
	if !ok {
		return nil
	}
	path := filepath.Join(m.find.dir, r.name)
	if r.here {
		path = m.find.dir
	}
	kv := func(k, v string) string {
		return detailKey.Render(padRight(k, detailKeyW)) + detailVal.Render(truncate(v, w-detailKeyW))
	}
	title := filepath.Base(path)
	if r.dir {
		title += "/"
	}
	rows := []string{detailVal.Render(truncate(title, w)), ""}
	room := m.bodyRows() - 5
	if !r.dir {
		rows = append(rows, kv("size", humanSize(r.size)), kv("modified", ago(r.mod)), "", detailHead.Render("preview"))
		for _, l := range filePreview(path, room-2) {
			rows = append(rows, metaStyle.Render(truncate(l, w)))
		}
		return rows
	}
	entries := readEntries(path)
	rows = append(rows, kv("folder", truncateLeft(collapseHome(path), w-detailKeyW)), kv("holds", countLabel(entries)), "", detailHead.Render("contents"))
	shown := 0
	for _, e := range entries {
		if strings.HasPrefix(e.name, ".") {
			continue
		}
		if shown == room-4 {
			rows = append(rows, metaStyle.Render(ellipsis))
			break
		}
		if e.dir {
			rows = append(rows, detailVal.Render(truncate(e.name+"/", w)))
		} else {
			rows = append(rows, metaStyle.Render(truncate(e.name, w)))
		}
		shown++
	}
	if shown == 0 {
		rows = append(rows, metaStyle.Render("empty"))
	}
	return rows
}

// filePreview returns the first lines of a text file; nothing for binary
// or unreadable files.
func filePreview(path string, lines int) []string {
	fd, err := os.Open(path)
	if err != nil {
		return []string{"(unreadable)"}
	}
	defer fd.Close()
	buf := make([]byte, 8192)
	n, _ := fd.Read(buf)
	buf = buf[:n]
	if bytes.IndexByte(buf, 0) >= 0 || !utf8.Valid(bytes.TrimRight(buf, "\x80\xbf")) {
		return []string{"(binary file)"}
	}
	out := strings.Split(strings.ReplaceAll(string(buf), "\t", "    "), "\n")
	if len(out) > lines {
		out = out[:max(0, lines)]
	}
	return out
}

// countLabel describes a folder's visible contents: "3 folders · 5 files".
func countLabel(entries []entry) string {
	dirs, files := 0, 0
	for _, e := range entries {
		if strings.HasPrefix(e.name, ".") {
			continue
		}
		if e.dir {
			dirs++
		} else {
			files++
		}
	}
	switch {
	case dirs == 0 && files == 0:
		return "empty"
	case files == 0:
		return plural2(dirs, "folder")
	case dirs == 0:
		return plural2(files, "file")
	}
	return plural2(dirs, "folder") + "  " + sep + "  " + plural2(files, "file")
}

func plural2(n int, noun string) string { return fmt.Sprintf("%d %s%s", n, noun, plural(n)) }

func humanSize(n int64) string {
	switch {
	case n < 1024:
		return fmt.Sprintf("%d B", n)
	case n < 1024*1024:
		return fmt.Sprintf("%.1f KB", float64(n)/1024)
	case n < 1024*1024*1024:
		return fmt.Sprintf("%.1f MB", float64(n)/(1024*1024))
	}
	return fmt.Sprintf("%.1f GB", float64(n)/(1024*1024*1024))
}
