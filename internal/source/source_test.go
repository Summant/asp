package source

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func claudeFixtures(t *testing.T) map[string]Session {
	t.Helper()
	got, err := (&ClaudeSource{Root: filepath.Join("testdata", "claude")}).Sessions()
	if err != nil {
		t.Fatal(err)
	}
	return byID(got)
}

func codexFixtures(t *testing.T) map[string]Session {
	t.Helper()
	base := filepath.Join("testdata", "codex")
	got, err := (&CodexSource{Roots: []string{
		filepath.Join(base, "sessions"),
		filepath.Join(base, "archived_sessions"),
	}}).Sessions()
	if err != nil {
		t.Fatal(err)
	}
	return byID(got)
}

func byID(ss []Session) map[string]Session {
	m := make(map[string]Session, len(ss))
	for _, s := range ss {
		m[s.ID] = s
	}
	return m
}

func need(t *testing.T, m map[string]Session, id string) Session {
	t.Helper()
	s, ok := m[id]
	if !ok {
		ids := make([]string, 0, len(m))
		for k := range m {
			ids = append(ids, k)
		}
		t.Fatalf("session %s not found; have %v", id, ids)
	}
	return s
}

func TestClaudeStringContent(t *testing.T) {
	s := need(t, claudeFixtures(t), "11111111-1111-4111-8111-111111111111")
	if s.Opening != "Hi, set up my waybar" {
		t.Errorf("Opening = %q; whitespace should collapse to single spaces", s.Opening)
	}
	if s.Agent != Claude || s.Messages != 4 {
		t.Errorf("Agent = %q, Messages = %d; want claude, 4", s.Agent, s.Messages)
	}
}

func TestClaudeCWDNotOnFirstLine(t *testing.T) {
	s := need(t, claudeFixtures(t), "11111111-1111-4111-8111-111111111111")
	if s.CWD != "/home/u/str" {
		t.Errorf("CWD = %q; want it read from line 3", s.CWD)
	}
}

func TestClaudeBlockContent(t *testing.T) {
	s := need(t, claudeFixtures(t), "22222222-2222-4222-8222-222222222222")
	if s.Opening != "What is in this screenshot?" {
		t.Errorf("Opening = %q", s.Opening)
	}
	if s.CWD != "/home/u/blocks" {
		t.Errorf("CWD = %q", s.CWD)
	}
}

func TestClaudeSkipsInjectedMessages(t *testing.T) {
	s := need(t, claudeFixtures(t), "33333333-3333-4333-8333-333333333333")
	if s.Opening != "Refactor the parser" {
		t.Errorf("Opening = %q; <-prefixed messages must be skipped", s.Opening)
	}
}

func TestClaudeWithoutCWDIsDropped(t *testing.T) {
	if _, ok := claudeFixtures(t)["44444444-4444-4444-8444-444444444444"]; ok {
		t.Error("a session with no cwd cannot be resumed and must not be listed")
	}
}

func TestClaudeLongLines(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "-p")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	huge := `{"type":"file-history-snapshot","snapshot":"` + strings.Repeat("x", 1<<20) + `"}`
	body := huge + "\n" + `{"type":"user","cwd":"/p","message":{"content":"after a 1 MB line"}}` + "\n"
	if err := os.WriteFile(filepath.Join(dir, "abc.jsonl"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := (&ClaudeSource{Root: root}).Sessions()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Opening != "after a 1 MB line" || got[0].CWD != "/p" {
		t.Errorf("got %+v", got)
	}
}

func TestCodexInputText(t *testing.T) {
	s := need(t, codexFixtures(t), "01a0efa1-9c6d-7141-9237-f46909ef3743")
	if s.Opening != "test" {
		t.Errorf("Opening = %q; want the first non-<environment_context> input_text", s.Opening)
	}
	if s.CWD != "/home/u/AgentSessionPicker" || s.Agent != Codex {
		t.Errorf("CWD = %q, Agent = %q", s.CWD, s.Agent)
	}
}

func TestCodexSessionMetaOnly(t *testing.T) {
	s := need(t, codexFixtures(t), "01a0efa0-1111-7222-8333-444455556666")
	if s.Opening != "" || s.Messages != 1 {
		t.Errorf("Opening = %q, Messages = %d; want empty, 1", s.Opening, s.Messages)
	}
	if s.CWD != "/home/u/AgentSessionPicker" {
		t.Errorf("CWD = %q", s.CWD)
	}
}

func TestCodexMessageIDDoesNotLeak(t *testing.T) {
	m := codexFixtures(t)
	for id := range m {
		if strings.HasPrefix(id, "msg_") {
			t.Errorf("message id %q used as a session id", id)
		}
	}
	// The msg_ ids in the fixture share the session's 8-char prefix, so only
	// a full comparison proves the right id was kept.
	need(t, m, "01a0efa1-9c6d-7141-9237-f46909ef3743")
}

func TestCodexPrefersSessionMetaID(t *testing.T) {
	root := t.TempDir()
	name := "rollout-2026-09-30T00-00-00-aaaaaaaa-aaaa-7aaa-8aaa-aaaaaaaaaaaa.jsonl"
	body := `{"type":"session_meta","payload":{"session_id":"bbbbbbbb-bbbb-7bbb-8bbb-bbbbbbbbbbbb","cwd":"/x"}}` + "\n"
	if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	got, _ := (&CodexSource{Roots: []string{root}}).Sessions()
	if len(got) != 1 || got[0].ID != "bbbbbbbb-bbbb-7bbb-8bbb-bbbbbbbbbbbb" {
		t.Errorf("got %+v; session_meta id should win over the filename", got)
	}
}

func TestCodexArchivedAndCompressed(t *testing.T) {
	m := codexFixtures(t)
	if s := need(t, m, "0198aaaa-bbbb-7ccc-8ddd-eeeeffff0000"); s.Opening != "archived prompt" {
		t.Errorf("archived Opening = %q", s.Opening)
	}
	if _, ok := m["0198bbbb-bbbb-7ccc-8ddd-eeeeffff0000"]; ok {
		t.Error(".jsonl.zst must be skipped")
	}
	if len(m) != 3 {
		t.Errorf("got %d codex sessions, want 3", len(m))
	}
}

func TestCodexID(t *testing.T) {
	cases := map[string]string{
		"/x/rollout-2026-09-30T00-05-41-01a0efa1-9c6d-7141-9237-f46909ef3743.jsonl": "01a0efa1-9c6d-7141-9237-f46909ef3743",
		"rollout-2025-01-02T03-04-05-0198aaaa-bbbb-7ccc-8ddd-eeeeffff0000.jsonl":    "0198aaaa-bbbb-7ccc-8ddd-eeeeffff0000",
		"rollout-abc.jsonl": "",
	}
	for in, want := range cases {
		if got := codexID(in); got != want {
			t.Errorf("codexID(%q) = %q, want %q", in, got, want)
		}
	}
}

type fakeSource struct {
	agent Agent
	ss    []Session
	err   error
}

func (f fakeSource) Agent() Agent                 { return f.agent }
func (f fakeSource) Sessions() ([]Session, error) { return f.ss, f.err }

func TestAllSkipsBrokenSourceAndSortsNewestFirst(t *testing.T) {
	now := time.Now()
	got := All(
		fakeSource{Claude, []Session{{ID: "old", Modified: now.Add(-time.Hour)}, {ID: "new", Modified: now}}, nil},
		fakeSource{Codex, nil, os.ErrPermission},
		fakeSource{Codex, []Session{{ID: "mid", Modified: now.Add(-time.Minute)}}, nil},
	)
	var ids []string
	for _, s := range got {
		ids = append(ids, s.ID)
	}
	if strings.Join(ids, ",") != "new,mid,old" {
		t.Errorf("order = %v", ids)
	}
}
