// Package store persists the user's custom session names.
//
// Names live in their own file, deliberately outside each agent's data
// directory, so nothing here can corrupt a transcript.
package store

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
)

type Store struct {
	path     string
	mu       sync.Mutex
	names    map[string]string // "<agent>:<session-id>" -> name
	namesErr error             // names.json exists but could not be read
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
	if _, err := os.Stat(path); err == nil {
		// An unreadable file is kept as it is and never overwritten; asp
		// runs without names and says so (see Problems).
		s.namesErr = readJSON(path, &s.names)
		if s.namesErr != nil {
			s.names = map[string]string{}
		}
		return s, nil
	} else if !errors.Is(err, fs.ErrNotExist) {
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

func (s *Store) save() error {
	if s.namesErr != nil {
		return s.namesErr
	}
	s.mu.Lock()
	b, err := json.MarshalIndent(s.names, "", "  ")
	s.mu.Unlock()
	if err != nil {
		return err
	}
	return writeAtomic(s.path, b)
}

// readJSON loads path into v. A missing or empty file is fine — v stays
// as it is. A file that exists but does not parse is an error: callers
// must then not write to it, so nothing a user saved is ever replaced by
// less (after a bad edit by hand, or a format a future asp misreads).
func readJSON(path string, v any) error {
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if len(bytes.TrimSpace(b)) == 0 {
		return nil
	}
	if err := json.Unmarshal(b, v); err != nil {
		return fmt.Errorf("%s could not be read (%v); asp will not change it until it is fixed or moved", path, err)
	}
	return nil
}

// Problems lists data files that exist but could not be read. asp keeps
// working without them and never writes to them.
func (s *Store) Problems() []string {
	var out []string
	check := func(path string, v any) {
		if err := readJSON(path, v); err != nil {
			out = append(out, err.Error())
		}
	}
	check(s.path, &map[string]string{})
	check(s.groupsPath(), &map[string][]string{})
	check(s.colorsPath(), &map[string]string{})
	check(s.statePath(), &State{})
	return out
}

// writeAtomic writes b to path so a crash mid-write leaves the previous file
// intact. The temp file is unique so two asp instances cannot clobber each
// other's.
func writeAtomic(path string, b []byte) error {
	dir := filepath.Dir(path)
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
	return os.Rename(tmp.Name(), path)
}

// Path is exposed so the UI can tell the user where names are kept.
func (s *Store) Path() string { return s.path }
