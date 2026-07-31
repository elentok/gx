// Package transcript reads Claude Code's own session transcript files
// (~/.claude/projects/<slugified-cwd>/<session-id>.jsonl) to recover
// information herdr itself doesn't expose, such as current context
// occupancy.
package transcript

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// Usage is a single assistant turn's token accounting, as recorded in a
// transcript line's message.usage.
type Usage struct {
	Model                    string
	InputTokens              int
	OutputTokens             int
	CacheReadInputTokens     int
	CacheCreationInputTokens int
}

// Occupancy returns u's current-context-occupancy figure: the input-side
// token fields only (input + cache-read + cache-creation), not output. A
// turn's cache-read total already reflects nearly all prior context reused
// via caching, so this — read from a single turn, never summed across
// turns — is the right proxy for "how much context is this session
// currently holding."
func (u Usage) Occupancy() int {
	return u.InputTokens + u.CacheReadInputTokens + u.CacheCreationInputTokens
}

// Slugify turns an absolute cwd into the directory name Claude Code stores
// that project's transcripts under: every "/" and "." replaced with "-".
func Slugify(cwd string) string {
	return strings.NewReplacer("/", "-", ".", "-").Replace(cwd)
}

// Path returns the transcript file path for a Claude Code session launched
// in cwd with the given session id (herdr's agent_session.value, which is
// the same UUID as the transcript's sessionId field and filename).
func Path(cwd, sessionID string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".claude", "projects", Slugify(cwd), sessionID+".jsonl"), nil
}

// transcriptLine is the subset of a transcript JSONL line this package
// reads: the assistant-turn usage fields.
type transcriptLine struct {
	Type    string `json:"type"`
	Message struct {
		Model string `json:"model"`
		Usage struct {
			InputTokens              int `json:"input_tokens"`
			OutputTokens             int `json:"output_tokens"`
			CacheReadInputTokens     int `json:"cache_read_input_tokens"`
			CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
		} `json:"usage"`
	} `json:"message"`
}

// initialTailBytes is how much of a transcript's tail LastAssistantUsage
// reads on its first pass. It doubles on each subsequent pass (still
// anchored to EOF) until it either finds an assistant line or has covered
// the whole file, so a poll tick against a long-running session's
// transcript stays cheap: the common case (the last line already is an
// assistant turn) is satisfied by a single small read from the end, never a
// full-file scan.
const initialTailBytes = 64 * 1024

// LastAssistantUsage returns the last `type: "assistant"` line's usage in
// the transcript at path, or ok=false if the file has no such line yet (or
// doesn't exist at all — e.g. the agent hasn't written its first turn out
// yet). Malformed lines are skipped rather than failing the read.
func LastAssistantUsage(path string) (Usage, bool, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Usage{}, false, nil
		}
		return Usage{}, false, err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return Usage{}, false, err
	}
	size := info.Size()

	for readSize := int64(initialTailBytes); ; readSize *= 2 {
		atStart := readSize >= size
		if atStart {
			readSize = size
		}
		offset := size - readSize

		buf := make([]byte, readSize)
		if _, err := f.ReadAt(buf, offset); err != nil {
			return Usage{}, false, err
		}

		lines := strings.Split(string(buf), "\n")
		// Unless this read reached byte 0 of the file, lines[0] is a
		// truncated continuation of a line that started before offset —
		// skip it rather than risk parsing a partial line as valid JSON.
		start := 0
		if !atStart {
			start = 1
		}
		for i := len(lines) - 1; i >= start; i-- {
			line := strings.TrimSpace(lines[i])
			if line == "" {
				continue
			}
			var entry transcriptLine
			if jsonErr := json.Unmarshal([]byte(line), &entry); jsonErr != nil {
				continue
			}
			if entry.Type != "assistant" {
				continue
			}
			return usageFromLine(entry), true, nil
		}

		if atStart {
			return Usage{}, false, nil
		}
	}
}

func usageFromLine(line transcriptLine) Usage {
	return Usage{
		Model:                    line.Message.Model,
		InputTokens:              line.Message.Usage.InputTokens,
		OutputTokens:             line.Message.Usage.OutputTokens,
		CacheReadInputTokens:     line.Message.Usage.CacheReadInputTokens,
		CacheCreationInputTokens: line.Message.Usage.CacheCreationInputTokens,
	}
}

// LastAssistantOccupancy is a convenience wrapper combining Path and
// LastAssistantUsage: it returns the current context occupancy for the
// session launched in cwd, or ok=false if no assistant turn has landed in
// its transcript yet.
func LastAssistantOccupancy(cwd, sessionID string) (occupancy int, ok bool, err error) {
	path, err := Path(cwd, sessionID)
	if err != nil {
		return 0, false, err
	}
	usage, ok, err := LastAssistantUsage(path)
	if err != nil || !ok {
		return 0, ok, err
	}
	return usage.Occupancy(), true, nil
}
