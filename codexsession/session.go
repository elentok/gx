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
	"time"
)

// RateLimit describes an exhausted Codex quota and when it becomes available.
type RateLimit struct {
	Quota   string
	ResetAt time.Time
}

// Stats are aggregate figures from one Codex session rollout.
type Stats struct {
	Start       time.Time
	End         time.Time
	PeakContext int
	TotalTokens int
}

// ReadStats returns duration endpoints, peak per-turn context, and the latest
// cumulative token usage for sessionID launched in cwd. Missing, partial, and
// malformed session data return ok=false.
func ReadStats(cwd, sessionID string) (stats Stats, ok bool, err error) {
	if cwd == "" || sessionID == "" {
		return Stats{}, false, nil
	}

	err = walkSessionFiles(sessionID, func(path string) error {
		value, valid, readErr := readStats(path, cwd, sessionID)
		if readErr != nil {
			return readErr
		}
		if valid {
			stats, ok = value, true
		}
		return nil
	})
	if err != nil {
		return Stats{}, false, err
	}
	return stats, ok, nil
}

// LastContextTokens returns the latest Codex context-token count for sessionID
// launched in cwd. It verifies the session metadata before accepting token data
// so a same-named rollout from another worktree cannot pause this iteration.
// Missing, partial, and malformed session files return ok=false.
func LastContextTokens(cwd, sessionID string) (tokens int, ok bool, err error) {
	if cwd == "" || sessionID == "" {
		return 0, false, nil
	}

	var found bool
	err = walkSessionFiles(sessionID, func(path string) error {
		value, valid, readErr := readContextTokens(path, cwd, sessionID)
		if readErr != nil {
			return readErr
		}
		if valid {
			tokens, found = value, true
		}
		return nil
	})
	if err != nil {
		return 0, false, err
	}
	return tokens, found, nil
}

// LastRateLimit returns the latest exhausted primary or secondary quota for
// sessionID launched in cwd. Missing, partial, malformed, and non-exhausted
// session data return ok=false.
func LastRateLimit(cwd, sessionID string) (limit RateLimit, ok bool, err error) {
	if cwd == "" || sessionID == "" {
		return RateLimit{}, false, nil
	}

	err = walkSessionFiles(sessionID, func(path string) error {
		value, valid, readErr := readRateLimit(path, cwd, sessionID)
		if readErr != nil {
			return readErr
		}
		if valid {
			limit, ok = value, true
		}
		return nil
	})
	if err != nil {
		return RateLimit{}, false, err
	}
	return limit, ok, nil
}

func walkSessionFiles(sessionID string, visit func(path string) error) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	sessionsDir := filepath.Join(home, ".codex", "sessions")
	err = filepath.WalkDir(sessionsDir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") || !strings.Contains(entry.Name(), sessionID) {
			return nil
		}
		return visit(path)
	})
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}

type sessionLine struct {
	Timestamp time.Time `json:"timestamp"`
	Type      string    `json:"type"`
	Payload   struct {
		ID   string `json:"id"`
		Cwd  string `json:"cwd"`
		Type string `json:"type"`
		Info struct {
			LastTokenUsage struct {
				InputTokens int `json:"input_tokens"`
			} `json:"last_token_usage"`
			TotalTokenUsage struct {
				TotalTokens int `json:"total_tokens"`
			} `json:"total_token_usage"`
		} `json:"info"`
		RateLimits struct {
			Primary   quotaWindow `json:"primary"`
			Secondary quotaWindow `json:"secondary"`
		} `json:"rate_limits"`
	} `json:"payload"`
}

func readStats(path, cwd, sessionID string) (stats Stats, ok bool, err error) {
	f, err := os.Open(path)
	if err != nil {
		return Stats{}, false, err
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
			if matchingSession && !line.Timestamp.IsZero() {
				stats.Start, stats.End, ok = line.Timestamp, line.Timestamp, true
			}
			continue
		}
		if !matchingSession {
			continue
		}
		if !line.Timestamp.IsZero() && (stats.End.IsZero() || line.Timestamp.After(stats.End)) {
			stats.End = line.Timestamp
		}
		if line.Type != "event_msg" || line.Payload.Type != "token_count" {
			continue
		}
		if line.Payload.Info.LastTokenUsage.InputTokens > stats.PeakContext {
			stats.PeakContext = line.Payload.Info.LastTokenUsage.InputTokens
		}
		if line.Payload.Info.TotalTokenUsage.TotalTokens > 0 {
			stats.TotalTokens = line.Payload.Info.TotalTokenUsage.TotalTokens
		}
	}
	if err := scanner.Err(); err != nil {
		return Stats{}, false, err
	}
	return stats, ok, nil
}

type quotaWindow struct {
	UsedPercent float64 `json:"used_percent"`
	ResetsAt    int64   `json:"resets_at"`
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

func readRateLimit(path, cwd, sessionID string) (limit RateLimit, ok bool, err error) {
	f, err := os.Open(path)
	if err != nil {
		return RateLimit{}, false, err
	}
	defer f.Close()

	matchingSession := false
	seenRateLimits := false
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
		if !matchingSession || line.Type != "event_msg" || line.Payload.Type != "token_count" {
			continue
		}
		primary, secondary := line.Payload.RateLimits.Primary, line.Payload.RateLimits.Secondary
		if !hasQuotaData(primary) && !hasQuotaData(secondary) {
			continue
		}
		seenRateLimits = true
		limit, ok = RateLimit{}, false
		if exhausted, valid := exhaustedQuota(primary, "primary"); valid {
			limit, ok = exhausted, true
			continue
		}
		if exhausted, valid := exhaustedQuota(secondary, "secondary"); valid {
			limit, ok = exhausted, true
		}
	}
	if err := scanner.Err(); err != nil {
		return RateLimit{}, false, err
	}
	return limit, ok && seenRateLimits, nil
}

func hasQuotaData(window quotaWindow) bool {
	return window.UsedPercent != 0 || window.ResetsAt != 0
}

func exhaustedQuota(window quotaWindow, quota string) (RateLimit, bool) {
	if window.UsedPercent < 100 {
		return RateLimit{}, false
	}
	if window.ResetsAt <= 0 {
		return RateLimit{Quota: quota}, true
	}
	return RateLimit{Quota: quota, ResetAt: time.Unix(window.ResetsAt, 0)}, true
}
