package codexsession

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/elentok/gx/transcript"
)

// LaunchPrompt is the first person message of a rollout. Skill is the user
// record that follows a "$skill" prompt and carries the skill body; it is ""
// when the next record is not one.
type LaunchPrompt struct {
	Text  string
	Skill string
}

const skillRecordPrefix = "<skill>"

// FirstPrompt returns the first person message of the rollout at path that
// starts at or after byte offset, skipping the injected AGENTS.md and
// environment lines. A resumed launch appends to the old file, so the caller
// records the file size when it sends and passes it back: an older user line
// never counts. ok is false while the file or such a message does not exist.
func FirstPrompt(path string, offset int64) (prompt LaunchPrompt, ok bool, err error) {
	f, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return LaunchPrompt{}, false, nil
	}
	if err != nil {
		return LaunchPrompt{}, false, err
	}
	defer f.Close()
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return LaunchPrompt{}, false, err
	}
	var texts []string
	err = transcript.ForEachLine(f, func(raw []byte) {
		if len(texts) >= 2 {
			return
		}
		var line chatRolloutLine
		if json.Unmarshal(raw, &line) != nil || line.Type != "response_item" || line.Payload.Type != "message" || line.Payload.Role != "user" {
			return
		}
		var parts []string
		for _, c := range line.Payload.Content {
			if t := strings.TrimSpace(c.Text); t != "" && !IsInjected(t) {
				parts = append(parts, t)
			}
		}
		if len(parts) > 0 {
			texts = append(texts, strings.Join(parts, "\n\n"))
		}
	})
	if err != nil || len(texts) == 0 {
		return LaunchPrompt{}, false, err
	}
	prompt.Text = texts[0]
	if len(texts) == 2 && strings.HasPrefix(texts[1], skillRecordPrefix) {
		prompt.Skill = texts[1]
	}
	return prompt, true, nil
}

type metaLine struct {
	Timestamp time.Time `json:"timestamp"`
	Type      string    `json:"type"`
	Payload   struct {
		Cwd       string          `json:"cwd"`
		Timestamp time.Time       `json:"timestamp"`
		Source    json.RawMessage `json:"source"`
	} `json:"payload"`
}

// LocateRollout finds the rollout of a launch whose session id is not known
// yet: the newest one under the Codex home whose cwd is cwd and whose session
// started at or after since. Only a person's session counts: its source is a
// string, while guardian and subagent rollouts carry an object.
func LocateRollout(home, cwd string, since time.Time) (path string, ok bool, err error) {
	var newest time.Time
	err = filepath.WalkDir(filepath.Join(home, "sessions"), func(p string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
			return nil
		}
		started, match, err := ownRolloutStart(p, cwd)
		if err != nil || !match || started.Before(since) {
			return err
		}
		if !ok || started.After(newest) {
			path, ok, newest = p, true, started
		}
		return nil
	})
	if errors.Is(err, fs.ErrNotExist) {
		return "", false, nil
	}
	return path, ok, err
}

// ownRolloutStart reads a rollout's session_meta line: when the session
// started, and whether it is a person's session in cwd. A file with no
// readable first line (Codex has not written it yet) does not match.
func ownRolloutStart(path, cwd string) (started time.Time, match bool, err error) {
	f, err := os.Open(path)
	if err != nil {
		return time.Time{}, false, err
	}
	defer f.Close()
	reader := bufio.NewReaderSize(f, 64*1024)
	raw, err := reader.ReadBytes('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return time.Time{}, false, err
	}
	var line metaLine
	if json.Unmarshal(raw, &line) != nil || line.Type != "session_meta" {
		return time.Time{}, false, nil
	}
	if !strings.HasPrefix(string(line.Payload.Source), `"`) || !sameDir(line.Payload.Cwd, cwd) {
		return time.Time{}, false, nil
	}
	started = line.Payload.Timestamp
	if started.IsZero() {
		started = line.Timestamp
	}
	return started, true, nil
}
