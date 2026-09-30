package store

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// Data written by earlier versions of asp must load unchanged by this one.
// testdata/v0.1.0 holds files in exactly the formats v0.1.0 wrote.

func copyDir(t *testing.T, from string) string {
	t.Helper()
	dir := t.TempDir()
	entries, err := os.ReadDir(from)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		b, err := os.ReadFile(filepath.Join(from, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, e.Name()), b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestReadsV010Data(t *testing.T) {
	dir := copyDir(t, filepath.Join("testdata", "v0.1.0"))
	s, err := OpenAt(filepath.Join(dir, "names.json"), filepath.Join(dir, "no-legacy.json"))
	if err != nil {
		t.Fatal(err)
	}
	if n, _ := s.Get("claude", "ba59df17-e3ef-4c5f-9ceb-cf77500d9f72"); n != "Arch + Hyprland setup" {
		t.Errorf("name %q", n)
	}
	if n, _ := s.Get("codex", "01a0efa1-9c6d-7141-9237-f46909ef3743"); n != "test" {
		t.Errorf("codex name %q", n)
	}
	if got := s.GroupsOf("claude", "ba59df17-e3ef-4c5f-9ceb-cf77500d9f72"); !reflect.DeepEqual(got, []string{"arch", "wm"}) {
		t.Errorf("groups %v", got)
	}
	st := s.State()
	if st.View != "group:arch" || !reflect.DeepEqual(st.Tabs, []string{"arch", "wm"}) || st.TabList != nil {
		t.Errorf("state %+v", st)
	}
	if p := s.Problems(); len(p) != 0 {
		t.Errorf("problems with valid old data: %v", p)
	}

	// Using the new version must not lose any of it.
	if err := s.Set("claude", "new", "another"); err != nil {
		t.Fatal(err)
	}
	if err := s.AddToGroup("claude", "new", "arch"); err != nil {
		t.Fatal(err)
	}
	re, _ := OpenAt(filepath.Join(dir, "names.json"), filepath.Join(dir, "no-legacy.json"))
	if n, _ := re.Get("codex", "01a0efa1-9c6d-7141-9237-f46909ef3743"); n != "test" {
		t.Error("an old name was lost after saving")
	}
	if got := re.Groups()["codex:01a0efa1-9c6d-7141-9237-f46909ef3743"]; !reflect.DeepEqual(got, []string{"arch"}) {
		t.Errorf("an old group membership was lost: %v", got)
	}
}

// A data file that cannot be read is reported and never overwritten, so a
// bad edit (or a format a future asp misreads) cannot wipe what it holds.
func TestUnreadableFilesAreNeverOverwritten(t *testing.T) {
	for _, file := range []string{"names.json", "groups.json", "group-colors.json", "state.json"} {
		t.Run(file, func(t *testing.T) {
			dir := copyDir(t, filepath.Join("testdata", "v0.1.0"))
			broken := `{"claude:x": "half an edit",`
			if err := os.WriteFile(filepath.Join(dir, file), []byte(broken), 0o644); err != nil {
				t.Fatal(err)
			}
			s, err := OpenAt(filepath.Join(dir, "names.json"), filepath.Join(dir, "no-legacy.json"))
			if err != nil {
				t.Fatal(err)
			}
			p := s.Problems()
			if len(p) != 1 || !strings.Contains(p[0], file) {
				t.Errorf("problems %v", p)
			}
			var writeErr error
			switch file {
			case "names.json":
				writeErr = s.Set("claude", "y", "name")
			case "groups.json":
				writeErr = s.AddToGroup("claude", "y", "g")
				if writeErr == nil {
					writeErr = s.RemoveFromGroup("claude", "y", "g")
				}
			case "group-colors.json":
				writeErr = s.SetGroupColor("arch", "#ffffff")
			case "state.json":
				writeErr = s.SaveState(State{View: "all"})
			}
			if writeErr == nil {
				t.Error("writing over an unreadable file should fail")
			}
			if b, _ := os.ReadFile(filepath.Join(dir, file)); string(b) != broken {
				t.Errorf("%s was overwritten: %q", file, b)
			}
		})
	}
}

func TestEmptyFilesAreFine(t *testing.T) {
	dir := t.TempDir()
	for _, f := range []string{"names.json", "groups.json", "state.json"} {
		if err := os.WriteFile(filepath.Join(dir, f), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	s, err := OpenAt(filepath.Join(dir, "names.json"), filepath.Join(dir, "none.json"))
	if err != nil {
		t.Fatal(err)
	}
	if p := s.Problems(); len(p) != 0 {
		t.Errorf("empty files reported: %v", p)
	}
	if err := s.Set("claude", "a", "x"); err != nil {
		t.Error(err)
	}
}
