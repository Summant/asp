package store

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

const archID = "ba59df17-e3ef-4c5f-9ceb-cf77500d9f72"

func write(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestMigrationImportsLegacyNames(t *testing.T) {
	dir := t.TempDir()
	names := filepath.Join(dir, "config", "asp", "names.json")
	legacy := filepath.Join(dir, "session-names.json")
	legacyBody := `{"` + archID + `": "Arch + Hyprland setup", "1edebed0-19e2-4052-8e94-4cee444525d7": "Test"}`
	write(t, legacy, legacyBody)

	s, err := OpenAt(names, legacy)
	if err != nil {
		t.Fatal(err)
	}
	if n, ok := s.Get("claude", archID); !ok || n != "Arch + Hyprland setup" {
		t.Errorf("claude:%s = %q, %v; want the migrated name", archID, n, ok)
	}
	if _, ok := s.Get("codex", archID); ok {
		t.Error("legacy names are Claude-only; must not appear under codex")
	}

	// Persisted, and the legacy file untouched.
	if _, err := os.Stat(names); err != nil {
		t.Fatalf("names.json not written: %v", err)
	}
	if b, _ := os.ReadFile(legacy); string(b) != legacyBody {
		t.Error("legacy file was modified")
	}
	reopened, err := OpenAt(names, legacy)
	if err != nil {
		t.Fatal(err)
	}
	if n, _ := reopened.Get("claude", archID); n != "Arch + Hyprland setup" {
		t.Errorf("after reopen got %q", n)
	}
}

func TestMigrationRunsOnlyOnce(t *testing.T) {
	dir := t.TempDir()
	names := filepath.Join(dir, "names.json")
	legacy := filepath.Join(dir, "session-names.json")
	write(t, legacy, `{"`+archID+`": "Arch + Hyprland setup"}`)

	s, err := OpenAt(names, legacy)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Set("claude", archID, ""); err != nil { // user clears the name
		t.Fatal(err)
	}
	s, err = OpenAt(names, legacy)
	if err != nil {
		t.Fatal(err)
	}
	if n, ok := s.Get("claude", archID); ok {
		t.Errorf("cleared name came back from the legacy file: %q", n)
	}
}

func TestNoLegacyFile(t *testing.T) {
	dir := t.TempDir()
	names := filepath.Join(dir, "names.json")
	s, err := OpenAt(names, filepath.Join(dir, "missing.json"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(names); err == nil {
		t.Error("names.json created with nothing to write")
	}
	if err := s.Set("codex", "abc", "x"); err != nil {
		t.Fatal(err)
	}
	if n, _ := s.Get("codex", "abc"); n != "x" {
		t.Errorf("got %q", n)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Errorf("stray files left beside names.json: %v", entries)
	}
}

func TestState(t *testing.T) {
	dir := t.TempDir()
	s, err := OpenAt(filepath.Join(dir, "names.json"), filepath.Join(dir, "none.json"))
	if err != nil {
		t.Fatal(err)
	}
	if got := s.State(); !reflect.DeepEqual(got, State{}) {
		t.Errorf("fresh state = %+v", got)
	}
	want := State{View: "group:arch", Tabs: []string{"arch", "wm"}, Last: "codex:abc"}
	if err := s.SaveState(want); err != nil {
		t.Fatal(err)
	}
	if got := s.State(); !reflect.DeepEqual(got, want) {
		t.Errorf("state = %+v, want %+v", got, want)
	}
}

func TestGroups(t *testing.T) {
	dir := t.TempDir()
	s, err := OpenAt(filepath.Join(dir, "names.json"), filepath.Join(dir, "none.json"))
	if err != nil {
		t.Fatal(err)
	}
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(s.AddToGroup("claude", "a", "arch"))
	must(s.AddToGroup("claude", "a", "waybar"))
	must(s.AddToGroup("codex", "a", "arch"))  // same id, other agent: distinct session
	must(s.AddToGroup("claude", "a", "arch")) // idempotent

	if got := s.GroupsOf("claude", "a"); len(got) != 2 || got[0] != "arch" || got[1] != "waybar" {
		t.Errorf("GroupsOf = %v", got)
	}
	if got := s.Groups()["codex:a"]; len(got) != 1 || got[0] != "arch" {
		t.Errorf("Groups()[codex:a] = %v", got)
	}
	must(s.RemoveFromGroup("claude", "a", "waybar"))
	if got := s.GroupNames(); len(got) != 1 || got[0] != "arch" {
		t.Errorf("emptied group not deleted: %v", got)
	}
	reopened, _ := OpenAt(filepath.Join(dir, "names.json"), filepath.Join(dir, "none.json"))
	if got := reopened.GroupsOf("codex", "a"); len(got) != 1 {
		t.Errorf("not persisted: %v", got)
	}
}
