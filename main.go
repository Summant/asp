// Command asp browses, names, groups, resumes and creates Claude Code and
// Codex CLI sessions.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime/debug"
	"strconv"
	"strings"

	"github.com/atotto/clipboard"
	osc52 "github.com/aymanbagabas/go-osc52/v2"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/summant/asp/internal/config"
	"github.com/summant/asp/internal/jobs"
	"github.com/summant/asp/internal/source"
	"github.com/summant/asp/internal/store"
	"github.com/summant/asp/internal/ui"
)

// version is set by the Makefile via -ldflags "-X main.version=…".
var version = ""

// commands is how each agent is started. Adding an agent means adding its
// entry here and its Source in internal/source.
var commands = map[source.Agent]struct {
	bin    string
	resume func(id string) []string
}{
	// Claude takes the id as a flag, Codex as a subcommand (SPEC §8).
	source.Claude: {"claude", func(id string) []string { return []string{"--resume", id} }},
	source.Codex:  {"codex", func(id string) []string { return []string{"resume", id} }},
}

// command returns the binary and arguments that resume id, or start a new
// session when id is empty.
func command(agent source.Agent, id string) (string, []string, error) {
	c, ok := commands[agent]
	if !ok {
		return "", nil, fmt.Errorf("don't know how to launch %q", agent)
	}
	if id == "" {
		return c.bin, nil, nil
	}
	return c.bin, c.resume(id), nil
}

func main() { os.Exit(run()) }

func run() int {
	list := flag.Bool("list", false, "print sessions as tab-separated values and exit\n(agent, id, modified, messages, folder, name, groups, opening message)")
	showVersion := flag.Bool("version", false, "print the version and exit")
	configPath := flag.String("config", config.Path(), "colour and key configuration `file`")
	writeConfig := flag.Bool("write-config", false, "write a commented config file with every default, then exit")
	flag.Parse()

	if *showVersion {
		fmt.Println("asp", versionString())
		return 0
	}
	if *writeConfig {
		if err := config.WriteExample(*configPath); err != nil {
			fmt.Fprintln(os.Stderr, "asp:", err)
			return 1
		}
		fmt.Println("wrote", *configPath)
		return 0
	}

	st, err := store.Open()
	if err != nil {
		fmt.Fprintln(os.Stderr, "asp:", err)
		return 1
	}
	sources := source.Default()
	reload := func() []ui.Item { return gather(sources, st) }

	if *list {
		writeList(os.Stdout, reload())
		return 0
	}

	// Only the picker uses colours and keys, so a config mistake never
	// breaks --list.
	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "asp: config", err)
		return 1
	}
	ui.ApplyTheme(cfg.Colors)

	host := jobs.NewHost(command)
	defer host.Close() // quitting ends paused sessions
	m := ui.New(reload(), ui.Deps{Store: st, Reload: reload, Host: host, Copy: copyText, Keys: cfg.Keys, Agents: installed()})
	if _, err := tea.NewProgram(m, tea.WithAltScreen()).Run(); err != nil {
		fmt.Fprintln(os.Stderr, "asp:", err)
		return 1
	}
	return 0
}

// installed lists the agents whose command is on PATH. Either is enough;
// with neither, asp still lists any sessions left on disk.
func installed() []source.Agent {
	out := []source.Agent{}
	for _, a := range []source.Agent{source.Claude, source.Codex} {
		if _, err := exec.LookPath(commands[a].bin); err == nil {
			out = append(out, a)
		}
	}
	return out
}

// gather reads every agent's sessions, newest first, with names and groups.
func gather(sources []source.Source, st *store.Store) []ui.Item {
	sessions := source.All(sources...)
	groups := st.Groups()
	items := make([]ui.Item, len(sessions))
	for i, s := range sessions {
		name, _ := st.Get(string(s.Agent), s.ID)
		items[i] = ui.Item{Session: s, Name: name}
		items[i].Groups = groups[items[i].Key()]
	}
	return items
}

// writeList prints one tab-separated line per session for scripting.
func writeList(w io.Writer, items []ui.Item) {
	for _, it := range items {
		s := it.Session
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			s.Agent, s.ID, s.Modified.Format("2006-01-02 15:04"), strconv.Itoa(s.Messages),
			field(s.CWD), field(it.Name), field(strings.Join(it.Groups, ",")), field(it.Opening()))
	}
}

// field keeps a value on one line and free of the tab delimiter, without
// otherwise altering it (a folder may legitimately contain double spaces).
var field = strings.NewReplacer("\t", " ", "\n", " ", "\r", " ").Replace

// copyText uses the system clipboard (wl-copy, xclip, xsel or pbcopy) and
// falls back to OSC 52, which asks the terminal itself to set it.
func copyText(s string) error {
	if err := clipboard.WriteAll(s); err == nil {
		return nil
	}
	_, err := osc52.New(s).WriteTo(os.Stderr)
	return err
}

func versionString() string {
	if version != "" {
		return version // set by the Makefile
	}
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return "dev"
	}
	if v := bi.Main.Version; v != "" && v != "(devel)" {
		return v // go install github.com/summant/asp@v0.1.0
	}
	for _, kv := range bi.Settings {
		if kv.Key == "vcs.revision" && len(kv.Value) >= 7 {
			return "dev-" + kv.Value[:7]
		}
	}
	return "dev"
}
