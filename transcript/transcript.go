// Package transcript reads Claude Code's own session transcript files
// (~/.claude/projects/<slugified-cwd>/<session-id>.jsonl) to recover
// information herdr itself doesn't expose, such as current context
// occupancy.
package transcript

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
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
// reads: the assistant-turn usage fields, a system-line's compaction-boundary
// marker, plus the line's own timestamp (used by ReadAll to compute a
// session's wall-clock duration).
type transcriptLine struct {
	Type      string `json:"type"`
	Subtype   string `json:"subtype"`
	Timestamp string `json:"timestamp"`
	Message   struct {
		Model string `json:"model"`
		Usage struct {
			InputTokens              int `json:"input_tokens"`
			OutputTokens             int `json:"output_tokens"`
			CacheReadInputTokens     int `json:"cache_read_input_tokens"`
			CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
		} `json:"usage"`
	} `json:"message"`
	CompactMetadata struct {
		Trigger string `json:"trigger"`
	} `json:"compactMetadata"`
}

// compactBoundarySubtype is the subtype value Claude Code writes on the
// type:"system" line it appends whenever a compaction happens (see
// .scratch/ralph-tickets-visibility/issues/04-compaction-detection-research.md),
// whether triggered by an explicit "/compact" or fired automatically on its
// own context ceiling.
const compactBoundarySubtype = "compact_boundary"

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
	var usage Usage
	found := false
	err := tailScan(path, func(lines []string) bool {
		for _, raw := range slices.Backward(lines) {
			entry, ok := parseLine(raw)
			if !ok || entry.Type != "assistant" {
				continue
			}
			usage, found = usageFromLine(entry), true
			return true
		}
		return false
	})
	if err != nil {
		return Usage{}, false, err
	}
	return usage, found, nil
}

// tailScan hands visit the tail of the transcript at path, split into lines
// in file order, starting from a small window anchored at EOF and doubling
// it (still anchored at EOF) until visit reports it has its answer or the
// window covers the whole file. Each pass re-presents everything the
// previous one saw, so a visit that needs to reason about several lines'
// relative order can simply recompute from scratch. A missing file yields no
// passes at all, leaving the caller's state untouched — every reader here
// treats "not written yet" the same as "nothing found".
func tailScan(path string, visit func(lines []string) bool) error {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return err
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
			return err
		}

		lines := strings.Split(string(buf), "\n")
		// Unless this read reached byte 0 of the file, lines[0] is a
		// truncated continuation of a line that started before offset —
		// skip it rather than risk parsing a partial line as valid JSON.
		if !atStart {
			lines = lines[1:]
		}
		if visit(lines) || atStart {
			return nil
		}
	}
}

// parseLine parses one raw transcript line, reporting ok=false for blank or
// malformed lines so callers can skip them rather than fail the read.
func parseLine(raw string) (transcriptLine, bool) {
	line := strings.TrimSpace(raw)
	if line == "" {
		return transcriptLine{}, false
	}
	var entry transcriptLine
	if err := json.Unmarshal([]byte(line), &entry); err != nil {
		return transcriptLine{}, false
	}
	return entry, true
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

// FirstLineTimestamp returns the timestamp of the first parseable line in
// the transcript at path — a forward scan that stops as soon as one is
// found, unlike ReadAll's whole-file parse, since a caller computing elapsed
// time (time.Now() minus this) only needs the earliest line. ok is false if
// the file doesn't exist yet or has no parseable timestamped line.
func FirstLineTimestamp(path string) (time.Time, bool, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return time.Time{}, false, nil
		}
		return time.Time{}, false, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)
	for scanner.Scan() {
		raw := strings.TrimSpace(scanner.Text())
		if raw == "" {
			continue
		}
		var entry transcriptLine
		if jsonErr := json.Unmarshal([]byte(raw), &entry); jsonErr != nil {
			continue
		}
		ts, tsErr := time.Parse(time.RFC3339Nano, entry.Timestamp)
		if tsErr != nil {
			continue
		}
		return ts, true, nil
	}
	if scanErr := scanner.Err(); scanErr != nil {
		return time.Time{}, false, scanErr
	}
	return time.Time{}, false, nil
}

// Elapsed returns time.Now() minus the session's transcript's first-line
// timestamp — the same first-line-to-now approach ralphloop's
// readSessionStats uses for a landed session's elapsed_time, so a
// still-running session's elapsed figure is computed consistently, and
// survives a UI restart mid-iteration (a plain time.Now()-at-start capture
// has no way to recover the iteration's true age after a reattach). ok is
// false if cwd/sessionID is empty or the transcript can't be read yet.
func Elapsed(cwd, sessionID string) (time.Duration, bool, error) {
	if cwd == "" || sessionID == "" {
		return 0, false, nil
	}
	path, err := Path(cwd, sessionID)
	if err != nil {
		return 0, false, err
	}
	start, ok, err := FirstLineTimestamp(path)
	if err != nil || !ok {
		return 0, ok, err
	}
	return time.Since(start), true, nil
}

// Line is a single parsed transcript line, timestamped, carrying its
// assistant-turn usage if it has one (the zero Usage otherwise), and its
// compaction-boundary marker if it's a system line reporting one (Subtype
// == "compact_boundary", CompactTrigger "manual" or "auto"). Unlike
// LastAssistantUsage's tail-only read (built for cheap, frequent polling
// against a long-running session), ReadAll parses the whole file, so it's
// meant for one-off aggregate reporting (peak occupancy, total cost, session
// span, compaction count), not the live smart-zone guardrail.
type Line struct {
	Type           string
	Subtype        string
	CompactTrigger string
	Timestamp      time.Time
	Usage          Usage
}

// IsCompactBoundary reports whether l is Claude Code's own marker line for a
// compaction that just happened.
func (l Line) IsCompactBoundary() bool {
	return l.Type == "system" && l.Subtype == compactBoundarySubtype
}

// ReadAll parses every line of the transcript at path in order, skipping
// lines that are malformed JSON or lack a parseable timestamp (both
// possible on a torn final line from a killed process) rather than failing
// the whole read. ok is false if the file doesn't exist yet.
func ReadAll(path string) (lines []Line, ok bool, err error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)
	for scanner.Scan() {
		raw := strings.TrimSpace(scanner.Text())
		if raw == "" {
			continue
		}
		var entry transcriptLine
		if jsonErr := json.Unmarshal([]byte(raw), &entry); jsonErr != nil {
			continue
		}
		ts, tsErr := time.Parse(time.RFC3339Nano, entry.Timestamp)
		if tsErr != nil {
			continue
		}
		lines = append(lines, Line{
			Type:           entry.Type,
			Subtype:        entry.Subtype,
			CompactTrigger: entry.CompactMetadata.Trigger,
			Timestamp:      ts,
			Usage:          usageFromLine(entry),
		})
	}
	if scanErr := scanner.Err(); scanErr != nil {
		return nil, false, scanErr
	}
	return lines, true, nil
}

// CountCompactions counts how many of lines are a compaction-boundary marker
// (see Line.IsCompactBoundary) — gx-triggered and Claude Code's own
// auto-compaction both count, since both write the same marker line and
// aren't reliably distinguishable from each other via CompactTrigger alone
// (a human typing "/compact" also lands "manual").
func CountCompactions(lines []Line) int {
	n := 0
	for _, l := range lines {
		if l.IsCompactBoundary() {
			n++
		}
	}
	return n
}

// Compactions is a convenience wrapper combining Path, ReadAll, and
// CountCompactions: it returns how many compaction boundaries the session
// launched in cwd hit, or ok=false if its transcript can't be found yet.
func Compactions(cwd, sessionID string) (count int, ok bool, err error) {
	path, err := Path(cwd, sessionID)
	if err != nil {
		return 0, false, err
	}
	lines, ok, err := ReadAll(path)
	if err != nil || !ok {
		return 0, ok, err
	}
	return CountCompactions(lines), true, nil
}
