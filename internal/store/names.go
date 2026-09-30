// Package store persists the user's custom session names.
//
// Names live in their own file, deliberately outside each agent's data
// directory, so nothing here can corrupt a transcript.
package store

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
)

type Store struct {
	path  string
	mu    sync.Mutex
	names map[string]string // "<agent>:<session-id>" -> name
}

// Open loads ~/.config/asp/names.json, importing the Python prototype's
// ~/.claude/session-names.json the first time.
func Open() (*Store, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	return OpenAt(
		filepath.Join(home, ".config", "asp", "names.json"),
		filepath.Join(home, ".claude", "session-names.json"),
	)
}

// OpenAt loads the names file at path. If that file does not exist yet, every
// entry of the prototype's legacy file ({session-id: name}, Claude only) is
// imported as "claude:<id>" and the result saved. The legacy file is only
// ever read: the prototype still uses it.
func OpenAt(path, legacy string) (*Store, error) {
	s := &Store{path: path, names: map[string]string{}}
	b, err := os.ReadFile(path)
	switch {
	case err == nil:
		_ = json.Unmarshal(b, &s.names) // a corrupt file starts empty rather than failing
		return s, nil
	case !errors.Is(err, fs.ErrNotExist):
		return nil, err
	}

	old, err := readLegacy(legacy)
	if err != nil || len(old) == 0 {
		return s, nil // nothing to import; the file is created on first rename
	}
	for id, name := range old {
		if name != "" {
			s.names[key("claude", id)] = name
		}
	}
	return s, s.save()
}

func readLegacy(path string) (map[string]string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m map[string]string
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	return m, nil
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
// The temp file is unique so two asp instances cannot clobber each other's.
func (s *Store) save() error {
	s.mu.Lock()
	b, err := json.MarshalIndent(s.names, "", "  ")
	s.mu.Unlock()
	if err != nil {
		return err
	}
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".names-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) // no-op once renamed
	if _, err := tmp.Write(append(b, '\n')); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o644); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), s.path)
}

// Path is exposed so the UI can tell the user where names are kept.
func (s *Store) Path() string { return s.path }
