package store

import (
	"encoding/json"
	"path/filepath"
	"slices"
	"sort"
)

// Groups are kept in groups.json beside names.json as
// {"group name": ["<agent>:<session-id>", …]}. A session may be in any
// number of groups; a group exists only while it has members.

func (s *Store) groupsPath() string { return filepath.Join(filepath.Dir(s.path), "groups.json") }

// loadGroups reads groups.json; on error the map is empty and must not be
// saved back.
func (s *Store) loadGroups() (map[string][]string, error) {
	g := map[string][]string{}
	if err := readJSON(s.groupsPath(), &g); err != nil {
		return map[string][]string{}, err
	}
	return g, nil
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
	groups, _ := s.loadGroups()
	for name, members := range groups {
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
	groups, _ := s.loadGroups()
	for name, members := range groups {
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
	groups, _ := s.loadGroups()
	for name := range groups {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// AddToGroup puts a session in a group, creating the group if needed.
func (s *Store) AddToGroup(agent, id, group string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	g, err := s.loadGroups()
	if err != nil {
		return err
	}
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
	g, err := s.loadGroups()
	if err != nil {
		return err
	}
	g[group] = slices.DeleteFunc(g[group], func(m string) bool { return m == key(agent, id) })
	if len(g[group]) == 0 {
		delete(g, group)
		if c, err := s.loadColors(); err == nil && c[group] != "" { // a group that is gone keeps no colour
			delete(c, group)
			if b, err := json.MarshalIndent(c, "", "  "); err == nil {
				_ = writeAtomic(s.colorsPath(), b)
			}
		}
	}
	return s.saveGroups(g)
}

// Group colours live in group-colors.json as {"group": "#rrggbb"}. A group
// without an entry is drawn in the fallback colour.

func (s *Store) colorsPath() string { return filepath.Join(filepath.Dir(s.path), "group-colors.json") }

func (s *Store) loadColors() (map[string]string, error) {
	c := map[string]string{}
	if err := readJSON(s.colorsPath(), &c); err != nil {
		return map[string]string{}, err
	}
	return c, nil
}

// GroupColors maps each group that has one to its colour.
func (s *Store) GroupColors() map[string]string {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, _ := s.loadColors()
	return c
}

// SetGroupColor sets a group's colour; "" removes it.
func (s *Store) SetGroupColor(group, color string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, err := s.loadColors()
	if err != nil {
		return err
	}
	if color == "" {
		delete(c, group)
	} else {
		c[group] = color
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(s.colorsPath(), b)
}
