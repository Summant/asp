package source

import (
	"bufio"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// CodexSource reads OpenAI Codex CLI transcripts ("rollouts").
//
// Layout: ~/.codex/sessions/YYYY/MM/DD/rollout-<timestamp>-<uuid>.jsonl, with
// archived conversations under ~/.codex/archived_sessions. Newer builds may
// write .jsonl.zst; those are listed but skipped rather than guessed at, since
// decompressing them would need a zstd dependency for no real gain.
//
// The resumable id is the UUID in the filename.
type CodexSource struct{ Roots []string }

func NewCodex() *CodexSource {
	home, _ := os.UserHomeDir()
	base := filepath.Join(home, ".codex")
	return &CodexSource{Roots: []string{
		filepath.Join(base, "sessions"),
		filepath.Join(base, "archived_sessions"),
	}}
}

func (c *CodexSource) Agent() Agent { return Codex }

func (c *CodexSource) Sessions() ([]Session, error) {
	var out []Session
	for _, root := range c.Roots {
		// The date-partitioned tree means a plain glob would need a fixed
		// depth; walking tolerates layout changes between Codex versions.
		_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil //nolint:nilerr // a missing or unreadable dir is not fatal
			}
			if !strings.HasSuffix(path, ".jsonl") {
				return nil
			}
			if s, ok := c.parse(path); ok {
				out = append(out, s)
			}
			return nil
		})
	}
	return out, nil
}

// codexLine covers both the session-metadata line and ordinary items. Codex
// nests most data under "payload", so fields are read from either level.
type codexLine struct {
	Type    string `json:"type"`
	CWD     string `json:"cwd"`
	ID      string `json:"id"`
	Payload struct {
		Type      string          `json:"type"`
		Role      string          `json:"role"`
		CWD       string          `json:"cwd"`
		ID        string          `json:"id"`
		SessionID string          `json:"session_id"`
		Content   json.RawMessage `json:"content"`
		Text      string          `json:"text"`
	} `json:"payload"`
}

func (c *CodexSource) parse(path string) (Session, bool) {
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
		ID:       codexID(path),
		Agent:    Codex,
		Modified: info.ModTime(),
		Path:     path,
	}

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	for sc.Scan() {
		s.Messages++
		if s.CWD != "" && s.Opening != "" {
			continue
		}
		var l codexLine
		if json.Unmarshal(sc.Bytes(), &l) != nil {
			continue
		}
		if s.CWD == "" {
			if l.CWD != "" {
				s.CWD = l.CWD
			} else if l.Payload.CWD != "" {
				s.CWD = l.Payload.CWD
			}
		}
		if s.Opening == "" && l.Payload.Role == "user" {
			t := l.Payload.Text
			if t == "" {
				t = firstText(l.Payload.Content)
			}
			if t != "" && !strings.HasPrefix(strings.TrimSpace(t), "<") {
				s.Opening = collapse(t)
			}
		}
		// The session_meta line carries the authoritative id; prefer it over
		// the one parsed out of the filename.
		if l.Payload.SessionID != "" {
			s.ID = l.Payload.SessionID
		}
	}
	if s.CWD == "" || s.ID == "" {
		return Session{}, false
	}
	return s, true
}

// codexID extracts the UUID from "rollout-2026-09-30T12-00-00-<uuid>.jsonl".
// The timestamp itself contains dashes, so take the last five dash-separated
// groups, which is exactly a UUID.
func codexID(path string) string {
	name := strings.TrimSuffix(filepath.Base(path), ".jsonl")
	name = strings.TrimPrefix(name, "rollout-")
	parts := strings.Split(name, "-")
	if len(parts) < 5 {
		return ""
	}
	return strings.Join(parts[len(parts)-5:], "-")
}
