package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"

	"github.com/summant/asp/internal/source"
	"github.com/summant/asp/internal/store"
	"github.com/summant/asp/internal/ui"
)

// agentCommand is how to start each agent. Adding an agent means adding its
// entry here and its Source in internal/source.
type agentCommand struct {
	bin    string
	resume func(id string) []string
}

var commands = map[source.Agent]agentCommand{
	// Claude takes the id as a flag, Codex as a subcommand (SPEC §8).
	source.Claude: {bin: "claude", resume: func(id string) []string { return []string{"--resume", id} }},
	source.Codex:  {bin: "codex", resume: func(id string) []string { return []string{"resume", id} }},
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

// launch runs the agent as a child process on this terminal and waits for it.
// asp stays alive so that a new session can be named once it exists. The
// returned int is the agent's exit status.
//
// src must be the Source for l.Agent; it is scanned before and after the run
// to find the id of a newly created session.
func launch(l *ui.Launch, src source.Source, st *store.Store, stderr io.Writer) (int, error) {
	bin, args, err := command(l.Agent, l.ResumeID)
	if err != nil {
		return 1, err
	}
	path, err := exec.LookPath(bin)
	if err != nil {
		return 1, fmt.Errorf("%s not found in PATH", bin)
	}
	if fi, err := os.Stat(l.Dir); err != nil || !fi.IsDir() {
		return 1, fmt.Errorf("not a directory: %s", l.Dir)
	}

	naming := l.ResumeID == "" && l.Name != ""
	var before map[string]bool
	if naming {
		before = map[string]bool{}
		existing, _ := src.Sessions()
		for _, s := range existing {
			before[s.ID] = true
		}
	}

	cmd := exec.Command(path, args...)
	cmd.Dir = l.Dir
	cmd.Env = append(os.Environ(), "PWD="+l.Dir)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr

	// Ctrl+C reaches the whole foreground process group. The agent handles
	// it; asp must survive it. Catch and drain rather than signal.Ignore:
	// SIG_IGN is inherited across exec and would make the agent deaf to
	// Ctrl+C, whereas a Go handler is reset to default in the child.
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt)
	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-sigs:
			case <-done:
				return
			}
		}
	}()
	runErr := cmd.Run()
	signal.Stop(sigs)
	close(done)

	code := 0
	if runErr != nil {
		var exit *exec.ExitError
		if !errors.As(runErr, &exit) {
			return 1, runErr
		}
		code = exit.ExitCode()
		if code < 0 { // killed by a signal
			code = 1
		}
	}

	if naming {
		after, _ := src.Sessions()
		s, ok := newSession(before, after, l.Dir)
		if !ok {
			fmt.Fprintf(stderr, "asp: no new %s session found in %s; name %q not saved\n", l.Agent, l.Dir, l.Name)
			return code, nil
		}
		if err := st.Set(string(l.Agent), s.ID, l.Name); err != nil {
			return code, fmt.Errorf("saving name: %w", err)
		}
	}
	return code, nil
}

// newSession picks the session that appeared during the run: an id absent
// from before whose folder is dir, newest if several (e.g. after /clear).
// It deliberately ignores mtime on known ids — resuming an old session in the
// same folder bumps that too.
func newSession(before map[string]bool, after []source.Session, dir string) (source.Session, bool) {
	var best source.Session
	found := false
	for _, s := range after {
		if before[s.ID] || !samePath(s.CWD, dir) {
			continue
		}
		if !found || s.Modified.After(best.Modified) {
			best, found = s, true
		}
	}
	return best, found
}

// samePath compares folders, allowing for the agent having recorded the
// symlink-resolved form of the directory it was started in.
func samePath(a, b string) bool {
	if filepath.Clean(a) == filepath.Clean(b) {
		return true
	}
	ra, err1 := filepath.EvalSymlinks(a)
	rb, err2 := filepath.EvalSymlinks(b)
	return err1 == nil && err2 == nil && ra == rb
}

// sourceFor finds the Source that reads agent's sessions.
func sourceFor(sources []source.Source, agent source.Agent) (source.Source, bool) {
	for _, s := range sources {
		if s.Agent() == agent {
			return s, true
		}
	}
	return nil, false
}
