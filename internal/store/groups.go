package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"sort"
)

// Groups are kept in groups.json beside names.json as
// {"group name": ["<agent>:<session-id>", …]}. A session may be in any
// number of groups; a group exists only while it has members.

func (s *Store) groupsPath() string { return filepath.Join(filepath.Dir(s.path), "groups.json") }

func (s *Store) loadGroups() map[string][]string {
	g := map[string][]string{}
	if b, err := os.ReadFile(s.groupsPath()); err == nil {
		_ = json.Unmarshal(b, &g)
	}
	return g
}

func (s *Store) saveGroups(g map[string][]string) error {
	b, err := json.MarshalIndent(g, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(s.groupsPath(), b)
}

// GroupsOf lists the groups a session belongs to, sorted.
func (s *Store) GroupsOf(agent, id string) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	k := key(agent, id)
	var out []string
	for name, members := range s.loadGroups() {
		if slices.Contains(members, k) {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}

// Groups maps every session key to its sorted groups, for loading a list.
func (s *Store) Groups() map[string][]string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := map[string][]string{}
	for name, members := range s.loadGroups() {
		for _, k := range members {
			out[k] = append(out[k], name)
		}
	}
	for k := range out {
		sort.Strings(out[k])
	}
	return out
}

// GroupNames lists every group, sorted.
func (s *Store) GroupNames() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []string
	for name := range s.loadGroups() {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// AddToGroup puts a session in a group, creating the group if needed.
func (s *Store) AddToGroup(agent, id, group string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	g := s.loadGroups()
	k := key(agent, id)
	if slices.Contains(g[group], k) {
		return nil
	}
	g[group] = append(g[group], k)
	return s.saveGroups(g)
}

// RemoveFromGroup takes a session out of a group; an emptied group is deleted.
func (s *Store) RemoveFromGroup(agent, id, group string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	g := s.loadGroups()
	g[group] = slices.DeleteFunc(g[group], func(m string) bool { return m == key(agent, id) })
	if len(g[group]) == 0 {
		delete(g, group)
	}
	return s.saveGroups(g)
}
