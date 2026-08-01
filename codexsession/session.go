// Package codexsession reads Codex CLI's locally persisted session data.
package codexsession

import (
	"bufio"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// LastContextTokens returns the latest Codex context-token count for sessionID
// launched in cwd. It verifies the session metadata before accepting token data
// so a same-named rollout from another worktree cannot pause this iteration.
// Missing, partial, and malformed session files return ok=false.
func LastContextTokens(cwd, sessionID string) (tokens int, ok bool, err error) {
	if cwd == "" || sessionID == "" {
		return 0, false, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return 0, false, err
	}
	sessionsDir := filepath.Join(home, ".codex", "sessions")
	var found bool
	err = filepath.WalkDir(sessionsDir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") || !strings.Contains(entry.Name(), sessionID) {
			return nil
		}

		value, valid, readErr := readContextTokens(path, cwd, sessionID)
		if readErr != nil {
			return readErr
		}
		if valid {
			tokens, found = value, true
		}
		return nil
	})
	if errors.Is(err, fs.ErrNotExist) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return tokens, found, nil
}

type sessionLine struct {
	Type    string `json:"type"`
	Payload struct {
		ID   string `json:"id"`
		Cwd  string `json:"cwd"`
		Type string `json:"type"`
		Info struct {
			LastTokenUsage struct {
				InputTokens int `json:"input_tokens"`
			} `json:"last_token_usage"`
		} `json:"info"`
	} `json:"payload"`
}

func readContextTokens(path, cwd, sessionID string) (tokens int, ok bool, err error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, false, err
	}
	defer f.Close()

	matchingSession := false
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)
	for scanner.Scan() {
		var line sessionLine
		if json.Unmarshal(scanner.Bytes(), &line) != nil {
			continue
		}
		if line.Type == "session_meta" {
			matchingSession = line.Payload.ID == sessionID && line.Payload.Cwd == cwd
			continue
		}
		if matchingSession && line.Type == "event_msg" && line.Payload.Type == "token_count" && line.Payload.Info.LastTokenUsage.InputTokens > 0 {
			tokens, ok = line.Payload.Info.LastTokenUsage.InputTokens, true
		}
	}
	if err := scanner.Err(); err != nil {
		return 0, false, err
	}
	return tokens, ok, nil
}
