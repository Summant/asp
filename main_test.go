package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/summant/asp/internal/source"
	"github.com/summant/asp/internal/ui"
)

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

func TestWriteList(t *testing.T) {
	var b bytes.Buffer
	writeList(&b, []ui.Item{
		{Session: source.Session{Agent: source.Claude, ID: "id1", CWD: "/a  b", Messages: 2800, Opening: "hi"}, Name: "tab\there", Groups: []string{"arch", "wm"}},
		{Session: source.Session{Agent: source.Codex, ID: "id2", CWD: "/c"}},
	})
	lines := strings.Split(strings.TrimSuffix(b.String(), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("got %d lines", len(lines))
	}
	for _, l := range lines {
		if n := strings.Count(l, "\t"); n != 7 {
			t.Errorf("%q has %d tabs, want 7", l, n)
		}
	}
	f := strings.Split(lines[0], "\t")
	if f[0] != "claude" || f[1] != "id1" || f[3] != "2800" || f[4] != "/a  b" || f[5] != "tab here" || f[6] != "arch,wm" || f[7] != "hi" {
		t.Errorf("fields = %q", f)
	}
}
