// Package source discovers coding-agent sessions on disk.
//
// Each supported agent stores its transcripts differently, so every agent
// implements Source. Adding an agent means adding one file here.
package source

import (
	"sort"
	"time"
)

// Agent identifies which CLI produced a session.
type Agent string

const (
	Claude Agent = "claude"
	Codex  Agent = "codex"
)

// Label is the short badge shown in the UI.
func (a Agent) Label() string {
	switch a {
	case Claude:
		return "claude"
	case Codex:
		return "codex"
	}
	return string(a)
}

// Session is one resumable conversation.
type Session struct {
	ID       string    // agent-native session id, used to resume
	Agent    Agent     //
	CWD      string    // directory the session ran in
	Opening  string    // first user message, used as a fallback title
	Messages int       // transcript line count, a rough size signal
	Modified time.Time //
	Path     string    // transcript file, for diagnostics
}

// Source finds the sessions belonging to one agent.
type Source interface {
	Agent() Agent
	Sessions() ([]Session, error)
}

// All gathers sessions from every source, newest first. A source that fails
// is skipped rather than failing the whole listing: one broken agent install
// should never hide the other agent's sessions.
func All(sources ...Source) []Session {
	var out []Session
	for _, s := range sources {
		found, err := s.Sessions()
		if err != nil {
			continue
		}
		out = append(out, found...)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Modified.After(out[j].Modified) })
	return out
}

// Default returns the sources we know how to read.
func Default() []Source { return []Source{NewClaude(), NewCodex()} }
