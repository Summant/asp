package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/summant/asp/internal/source"
	"github.com/summant/asp/internal/store"
	"github.com/summant/asp/internal/ui"
)

// fakeAgent stands in for claude/codex. It records its arguments and signal
// dispositions, bumps the mtime of an existing session in the same folder (as
// resuming would), and creates a new Claude-format session there.
const fakeAgent = `#!/bin/sh
echo "$@" > "$FAKE_OUT/args"
grep SigIgn /proc/self/status > "$FAKE_OUT/sigign"
sleep "${FAKE_SLEEP:-0}"
touch "$FAKE_ROOT/p/old.jsonl"
printf '{"type":"user","cwd":"%s","message":{"content":"hi"}}\n' "$(pwd)" > "$FAKE_ROOT/p/$FAKE_NEW_ID.jsonl"
exit "${FAKE_EXIT:-0}"
`

type env struct {
	root, out, dir string
	src            *source.ClaudeSource
	st             *store.Store
}

func setup(t *testing.T) env {
	t.Helper()
	tmp := t.TempDir()
	e := env{
		root: filepath.Join(tmp, "projects"),
		out:  filepath.Join(tmp, "out"),
		dir:  filepath.Join(tmp, "work"),
	}
	bin := filepath.Join(tmp, "bin")
	for _, d := range []string{filepath.Join(e.root, "p"), e.out, e.dir, bin} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"claude", "codex"} {
		if err := os.WriteFile(filepath.Join(bin, name), []byte(fakeAgent), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin+":/usr/bin:/bin")
	t.Setenv("FAKE_ROOT", e.root)
	t.Setenv("FAKE_OUT", e.out)
	t.Setenv("FAKE_NEW_ID", "new-id")

	// An older session in the same folder, plus one elsewhere.
	old := filepath.Join(e.root, "p", "old.jsonl")
	writeSession(t, old, e.dir)
	past := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(old, past, past); err != nil {
		t.Fatal(err)
	}
	writeSession(t, filepath.Join(e.root, "p", "other.jsonl"), "/elsewhere")

	e.src = &source.ClaudeSource{Root: e.root}
	st, err := store.OpenAt(filepath.Join(tmp, "names.json"), filepath.Join(tmp, "no-legacy.json"))
	if err != nil {
		t.Fatal(err)
	}
	e.st = st
	return e
}

func writeSession(t *testing.T, path, cwd string) {
	t.Helper()
	line := `{"type":"user","cwd":"` + cwd + `","message":{"content":"old"}}` + "\n"
	if err := os.WriteFile(path, []byte(line), 0o644); err != nil {
		t.Fatal(err)
	}
}

func (e env) read(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(e.out, name))
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(b))
}

func TestCommand(t *testing.T) {
	cases := []struct {
		agent source.Agent
		id    string
		want  string
	}{
		{source.Claude, "", "claude"},
		{source.Claude, "abc", "claude --resume abc"},
		{source.Codex, "", "codex"},
		{source.Codex, "abc", "codex resume abc"},
	}
	for _, c := range cases {
		bin, args, err := command(c.agent, c.id)
		if err != nil {
			t.Fatal(err)
		}
		if got := strings.Join(append([]string{bin}, args...), " "); got != c.want {
			t.Errorf("command(%s, %q) = %q, want %q", c.agent, c.id, got, c.want)
		}
	}
	if _, _, err := command("gemini", ""); err == nil {
		t.Error("unknown agent should be an error")
	}
}

func TestLaunchNamesNewSessionByIDDiff(t *testing.T) {
	e := setup(t)
	var stderr bytes.Buffer
	code, err := launch(&ui.Launch{Agent: source.Claude, Dir: e.dir, Name: "Waybar redesign"}, e.src, e.st, &stderr)
	if err != nil || code != 0 {
		t.Fatalf("launch = %d, %v; stderr %q", code, err, stderr.String())
	}
	if got := e.read(t, "args"); got != "" {
		t.Errorf("new session args = %q, want none", got)
	}
	if n, _ := e.st.Get("claude", "new-id"); n != "Waybar redesign" {
		t.Errorf("new session named %q", n)
	}
	// old.jsonl is now the most recently modified file in the folder, but it
	// existed before the launch, so it must not take the name.
	if n, ok := e.st.Get("claude", "old"); ok {
		t.Errorf("pre-existing session was named %q", n)
	}
}

func TestLaunchChildDoesNotInheritIgnoredSIGINT(t *testing.T) {
	e := setup(t)
	if _, err := launch(&ui.Launch{Agent: source.Claude, Dir: e.dir}, e.src, e.st, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	line := e.read(t, "sigign") // "SigIgn:\t0000000000000000"
	mask, err := strconv.ParseUint(strings.TrimSpace(strings.TrimPrefix(line, "SigIgn:")), 16, 64)
	if err != nil {
		t.Fatalf("parse %q: %v", line, err)
	}
	if mask&(1<<(int(syscall.SIGINT)-1)) != 0 {
		t.Errorf("agent started with SIGINT ignored (SigIgn %016x); Ctrl+C would not reach it", mask)
	}
}

func TestLaunchSurvivesCtrlC(t *testing.T) {
	e := setup(t)
	t.Setenv("FAKE_SLEEP", "0.5")
	go func() {
		time.Sleep(150 * time.Millisecond)
		_ = syscall.Kill(os.Getpid(), syscall.SIGINT) // as the terminal would
	}()
	code, err := launch(&ui.Launch{Agent: source.Claude, Dir: e.dir, Name: "after ctrl-c"}, e.src, e.st, &bytes.Buffer{})
	if err != nil || code != 0 {
		t.Fatalf("launch = %d, %v", code, err)
	}
	if n, _ := e.st.Get("claude", "new-id"); n != "after ctrl-c" {
		t.Errorf("asp did not survive to name the session; got %q", n)
	}
}

func TestLaunchResume(t *testing.T) {
	e := setup(t)
	id := "01a0efa1-9c6d-7141-9237-f46909ef3743"
	if _, err := launch(&ui.Launch{Agent: source.Codex, Dir: e.dir, ResumeID: id}, e.src, e.st, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if got := e.read(t, "args"); got != "resume "+id {
		t.Errorf("codex args = %q", got)
	}
	if _, ok := e.st.Get("claude", "new-id"); ok {
		t.Error("resume must not name anything")
	}
}

func TestLaunchExitCodeAndErrors(t *testing.T) {
	e := setup(t)
	t.Setenv("FAKE_EXIT", "3")
	code, err := launch(&ui.Launch{Agent: source.Claude, Dir: e.dir}, e.src, e.st, &bytes.Buffer{})
	if err != nil || code != 3 {
		t.Errorf("launch = %d, %v; want the agent's exit status 3", code, err)
	}

	if _, err := launch(&ui.Launch{Agent: source.Claude, Dir: filepath.Join(e.dir, "nope")}, e.src, e.st, &bytes.Buffer{}); err == nil {
		t.Error("missing folder should be an error")
	}

	t.Setenv("PATH", t.TempDir())
	code, err = launch(&ui.Launch{Agent: source.Claude, Dir: e.dir}, e.src, e.st, &bytes.Buffer{})
	if err == nil || code == 0 || !strings.Contains(err.Error(), "claude not found") {
		t.Errorf("missing binary: code %d, err %v", code, err)
	}
}

func TestLaunchReportsWhenNoNewSession(t *testing.T) {
	e := setup(t)
	t.Setenv("FAKE_NEW_ID", "other") // agent "creates" a session that already existed
	var stderr bytes.Buffer
	if _, err := launch(&ui.Launch{Agent: source.Claude, Dir: e.dir, Name: "lost"}, e.src, e.st, &stderr); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stderr.String(), `name "lost" not saved`) {
		t.Errorf("stderr = %q", stderr.String())
	}
}

func TestNewSessionPicksNewestUnseenInFolder(t *testing.T) {
	now := time.Now()
	after := []source.Session{
		{ID: "old", CWD: "/w", Modified: now.Add(time.Hour)}, // bumped by a resume
		{ID: "a", CWD: "/w", Modified: now},
		{ID: "b", CWD: "/w/", Modified: now.Add(time.Minute)}, // e.g. after /clear
		{ID: "c", CWD: "/other", Modified: now.Add(2 * time.Minute)},
	}
	got, ok := newSession(map[string]bool{"old": true}, after, "/w")
	if !ok || got.ID != "b" {
		t.Errorf("got %q, %v; want b", got.ID, ok)
	}
	if _, ok := newSession(map[string]bool{"old": true, "a": true, "b": true}, after, "/w"); ok {
		t.Error("no unseen session in /w, want none")
	}
}

func TestWriteList(t *testing.T) {
	var b bytes.Buffer
	writeList(&b, []ui.Item{
		{Session: source.Session{Agent: source.Claude, ID: "id1", CWD: "/a  b", Messages: 2800, Opening: "hi"}, Name: "tab\there"},
		{Session: source.Session{Agent: source.Codex, ID: "id2", CWD: "/c"}},
	})
	lines := strings.Split(strings.TrimSuffix(b.String(), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("got %d lines", len(lines))
	}
	for _, l := range lines {
		if n := strings.Count(l, "\t"); n != 6 {
			t.Errorf("%q has %d tabs, want 6", l, n)
		}
	}
	f := strings.Split(lines[0], "\t")
	if f[0] != "claude" || f[1] != "id1" || f[3] != "2800" || f[4] != "/a  b" || f[5] != "tab here" || f[6] != "hi" {
		t.Errorf("fields = %q", f)
	}
}
