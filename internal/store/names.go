// Package store persists the user's custom session names.
//
// Names live in their own file, deliberately outside each agent's data
// directory, so nothing here can corrupt a transcript.
package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

type Store struct {
	path  string
	mu    sync.Mutex
	names map[string]string // "<agent>:<session-id>" -> name
}

func Open() (*Store, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	s := &Store{
		path:  filepath.Join(home, ".config", "asp", "names.json"),
		names: map[string]string{},
	}
	b, err := os.ReadFile(s.path)
	if err == nil {
		_ = json.Unmarshal(b, &s.names) // a corrupt file starts empty rather than failing
	}
	return s, nil
}

func key(agent, id string) string { return agent + ":" + id }

func (s *Store) Get(agent, id string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	n, ok := s.names[key(agent, id)]
	return n, ok
}

func (s *Store) Set(agent, id, name string) error {
	s.mu.Lock()
	if name == "" {
		delete(s.names, key(agent, id))
	} else {
		s.names[key(agent, id)] = name
	}
	s.mu.Unlock()
	return s.save()
}

// save writes atomically: a crash mid-write leaves the previous file intact.
func (s *Store) save() error {
	s.mu.Lock()
	b, err := json.MarshalIndent(s.names, "", "  ")
	s.mu.Unlock()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

// Path is exposed so the UI can tell the user where names are kept.
func (s *Store) Path() string { return s.path }
