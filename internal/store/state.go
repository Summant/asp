package store

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// State is what asp remembers between runs, kept in state.json beside
// names.json.
type State struct {
	View string   `json:"view,omitempty"` // "all", "claude", "codex" or "group:<name>"
	Tabs []string `json:"tabs,omitempty"` // group tabs, in order
	Last string   `json:"last,omitempty"` // "<agent>:<session-id>" last selected
}

func (s *Store) statePath() string { return filepath.Join(filepath.Dir(s.path), "state.json") }

// State returns the saved state; a missing or corrupt file gives the zero State.
func (s *Store) State() State {
	var st State
	if b, err := os.ReadFile(s.statePath()); err == nil {
		_ = json.Unmarshal(b, &st)
	}
	return st
}

func (s *Store) SaveState(st State) error {
	b, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(s.statePath(), b)
}
