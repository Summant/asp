package source

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// ClaudeSource reads Claude Code transcripts.
//
// Layout: ~/.claude/projects/<encoded-cwd>/<session-id>.jsonl, one JSON event
// per line. The directory name encodes the working directory lossily (both '/'
// and '.' become '-'), so it cannot be decoded back — the real path is read
// from the "cwd" field inside the transcript instead.
type ClaudeSource struct{ Root string }

func NewClaude() *ClaudeSource {
	home, _ := os.UserHomeDir()
	return &ClaudeSource{Root: filepath.Join(home, ".claude", "projects")}
}

func (c *ClaudeSource) Agent() Agent { return Claude }

func (c *ClaudeSource) Sessions() ([]Session, error) {
	files, err := filepath.Glob(filepath.Join(c.Root, "*", "*.jsonl"))
	if err != nil {
		return nil, err
	}
	out := make([]Session, 0, len(files))
	for _, f := range files {
		s, ok := c.parse(f)
		if ok {
			out = append(out, s)
		}
	}
	return out, nil
}

type claudeLine struct {
	CWD     string `json:"cwd"`
	Type    string `json:"type"`
	Message struct {
		Content json.RawMessage `json:"content"`
	} `json:"message"`
}

func (c *ClaudeSource) parse(path string) (Session, bool) {
	f, err := os.Open(path)
	if err != nil {
		return Session{}, false
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return Session{}, false
	}

	s := Session{
		ID:       strings.TrimSuffix(filepath.Base(path), ".jsonl"),
		Agent:    Claude,
		Modified: info.ModTime(),
		Path:     path,
	}

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024) // transcripts contain long lines
	for sc.Scan() {
		s.Messages++
		// Once both fields are known, keep counting lines but stop parsing:
		// a large transcript is mostly tool output we do not need.
		if s.CWD != "" && s.Opening != "" {
			continue
		}
		var l claudeLine
		if json.Unmarshal(sc.Bytes(), &l) != nil {
			continue
		}
		if s.CWD == "" && l.CWD != "" {
			s.CWD = l.CWD
		}
		if s.Opening == "" && l.Type == "user" {
			if t := firstText(l.Message.Content); t != "" && !strings.HasPrefix(strings.TrimSpace(t), "<") {
				s.Opening = collapse(t)
			}
		}
	}
	if s.CWD == "" {
		return Session{}, false // cannot resume a session whose directory is unknown
	}
	return s, true
}

// firstText pulls the user's text out of a message body, which is either a
// plain string or an array of typed content blocks.
func firstText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var str string
	if json.Unmarshal(raw, &str) == nil {
		return str
	}
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &blocks) == nil {
		for _, b := range blocks {
			// Claude Code emits "text"; Codex emits "input_text".
			if (b.Type == "text" || b.Type == "input_text") && b.Text != "" {
				return b.Text
			}
		}
	}
	return ""
}

// collapse squeezes all whitespace to single spaces so titles stay on one line.
func collapse(s string) string { return strings.Join(strings.Fields(s), " ") }
