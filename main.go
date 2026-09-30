// Command asp browses, names, resumes and creates Claude Code and Codex CLI
// sessions.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"runtime/debug"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/summant/asp/internal/source"
	"github.com/summant/asp/internal/store"
	"github.com/summant/asp/internal/ui"
)

// version is set by the Makefile via -ldflags "-X main.version=…".
var version = ""

func main() { os.Exit(run()) }

func run() int {
	list := flag.Bool("list", false, "print sessions as tab-separated values and exit\n(agent, id, modified, messages, folder, name, opening message)")
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println("asp", versionString())
		return 0
	}

	st, err := store.Open()
	if err != nil {
		fmt.Fprintln(os.Stderr, "asp:", err)
		return 1
	}
	sources := source.Default()
	items := gather(sources, st)

	if *list {
		writeList(os.Stdout, items)
		return 0
	}

	final, err := tea.NewProgram(ui.New(items, st), tea.WithAltScreen()).Run()
	if err != nil {
		fmt.Fprintln(os.Stderr, "asp:", err)
		return 1
	}
	m, ok := final.(ui.Model)
	if !ok || m.Result == nil {
		return 0 // quit without choosing
	}

	src, ok := sourceFor(sources, m.Result.Agent)
	if !ok {
		fmt.Fprintf(os.Stderr, "asp: no source for %s\n", m.Result.Agent)
		return 1
	}
	code, err := launch(m.Result, src, st, os.Stderr)
	if err != nil {
		fmt.Fprintln(os.Stderr, "asp:", err)
		if code == 0 {
			code = 1
		}
	}
	return code
}

// gather reads every agent's sessions, newest first, with their custom names.
func gather(sources []source.Source, st *store.Store) []ui.Item {
	sessions := source.All(sources...)
	items := make([]ui.Item, len(sessions))
	for i, s := range sessions {
		name, _ := st.Get(string(s.Agent), s.ID)
		items[i] = ui.Item{Session: s, Name: name}
	}
	return items
}

// writeList prints one tab-separated line per session for scripting.
func writeList(w io.Writer, items []ui.Item) {
	for _, it := range items {
		s := it.Session
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			s.Agent, s.ID, s.Modified.Format("2006-01-02 15:04"), strconv.Itoa(s.Messages),
			field(s.CWD), field(it.Name), field(it.Opening()))
	}
}

// field keeps a value on one line and free of the tab delimiter, without
// otherwise altering it (a folder may legitimately contain double spaces).
var field = strings.NewReplacer("\t", " ", "\n", " ", "\r", " ").Replace

func versionString() string {
	if version != "" {
		return version
	}
	if bi, ok := debug.ReadBuildInfo(); ok {
		for _, kv := range bi.Settings {
			if kv.Key == "vcs.revision" && len(kv.Value) >= 7 {
				return "dev-" + kv.Value[:7]
			}
		}
	}
	return "dev"
}
