// Package prflow holds the server logic of the PR flow. It is a new package
// because the packages that would host it are at their sizeguard limits.
package prflow

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// ActivityFile is the name of the PR activity log, beside the epic's ticket.md.
const ActivityFile = "pr-activity.jsonl"

// Kind is the kind of one PR activity entry.
type Kind string

// Entry kinds.
const (
	KindPROpened        Kind = "pr-opened"
	KindPRAdopted       Kind = "pr-adopted"
	KindMarkedReady     Kind = "marked-ready"
	KindReranCheck      Kind = "reran-check"
	KindCIFailed        Kind = "ci-failed"
	KindFlaky           Kind = "flaky"
	KindPausedCIFailing Kind = "paused-ci-failing"
	KindTryAgainReset   Kind = "try-again-reset"
	KindPRMerged        Kind = "pr-merged"
	KindTrunkUpdated    Kind = "trunk-updated"
	KindPRClosed        Kind = "pr-closed"
	KindGitHubDown      Kind = "github-unreachable"
	KindGitHubUp        Kind = "github-reachable"
	KindMarkedMerged    Kind = "marked-merged"
)

// Entry is one line of pr-activity.jsonl. Fields are versioned by name:
// a reader ignores names it does not know.
type Entry struct {
	Time   time.Time `json:"time"`
	Kind   Kind      `json:"kind"`
	Detail string    `json:"detail,omitempty"`
	// Title replaces the kind's words on the page when set.
	Title string `json:"title,omitempty"`
	// Checks names the check keys involved (one for a rerun or a flaky mark,
	// the failed ones for a CI failure).
	Checks   []string `json:"checks,omitempty"`
	HeadSHA  string   `json:"head_sha,omitempty"`
	TicketID string   `json:"ticket_id,omitempty"`
}

// Append writes e as one whole line to the epic's activity log. A zero Time is
// set to now. One writer at a time.
func Append(epicDir string, e Entry) error {
	if e.Time.IsZero() {
		e.Time = time.Now().UTC()
	}
	line, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("encode PR activity entry: %w", err)
	}
	line = append(line, '\n')
	f, err := os.OpenFile(filepath.Join(epicDir, ActivityFile), os.O_APPEND|os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return fmt.Errorf("open PR activity log: %w", err)
	}
	// A torn earlier write leaves no newline; start a fresh line so this
	// entry is not glued to it.
	if st, serr := f.Stat(); serr == nil && st.Size() > 0 {
		if last, rerr := lastByte(f, st.Size()); rerr == nil && last != '\n' {
			line = append([]byte{'\n'}, line...)
		}
	}
	// One write call keeps the line whole.
	if _, err := f.Write(line); err != nil {
		_ = f.Close()
		return fmt.Errorf("append PR activity entry: %w", err)
	}
	return f.Close()
}

func lastByte(f *os.File, size int64) (byte, error) {
	var b [1]byte
	if _, err := f.ReadAt(b[:], size-1); err != nil {
		return 0, err
	}
	return b[0], nil
}

// Read returns every entry, newest first. A missing file is no entries. A line
// that does not parse (a torn write) is skipped.
func Read(epicDir string) ([]Entry, error) {
	data, err := os.ReadFile(filepath.Join(epicDir, ActivityFile))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read PR activity log: %w", err)
	}
	var out []Entry
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(nil, 1<<20)
	for sc.Scan() {
		var e Entry
		if json.Unmarshal(sc.Bytes(), &e) != nil || e.Kind == "" {
			continue
		}
		out = append(out, e)
	}
	// File order is write order; reverse it, then keep it stable by time.
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Time.After(out[j].Time) })
	return out, nil
}

// sinceReset returns the entries after the newest Try again reset (all of them
// without one), newest first.
func sinceReset(epicDir string) ([]Entry, error) {
	all, err := Read(epicDir)
	if err != nil {
		return nil, err
	}
	for i, e := range all {
		if e.Kind == KindTryAgainReset {
			return all[:i], nil
		}
	}
	return all, nil
}

// ResetTime is the time of the newest Try again reset entry, zero without one.
func ResetTime(epicDir string) (time.Time, error) {
	all, err := Read(epicDir)
	if err != nil {
		return time.Time{}, err
	}
	for _, e := range all {
		if e.Kind == KindTryAgainReset {
			return e.Time, nil
		}
	}
	return time.Time{}, nil
}

// CIFailedByCheck counts CI-failed attempts per check key since the newest reset.
func CIFailedByCheck(epicDir string) (map[string]int, error) {
	es, err := sinceReset(epicDir)
	if err != nil {
		return nil, err
	}
	counts := map[string]int{}
	for _, e := range es {
		if e.Kind != KindCIFailed {
			continue
		}
		for _, c := range e.Checks {
			counts[c]++
		}
	}
	return counts, nil
}

// CIFailedAttempts counts CI-failed attempts for the PR since the newest reset.
func CIFailedAttempts(epicDir string) (int, error) {
	es, err := sinceReset(epicDir)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, e := range es {
		if e.Kind == KindCIFailed {
			n++
		}
	}
	return n, nil
}

// LastClosed is the time of the newest PR-closed entry since the newest reset,
// zero without one.
func LastClosed(epicDir string) (time.Time, error) { return lastOf(epicDir, KindPRClosed) }

// LastOpened is the time of the newest PR-opened entry since the newest reset,
// zero without one.
func LastOpened(epicDir string) (time.Time, error) { return lastOf(epicDir, KindPROpened) }

func lastOf(epicDir string, k Kind) (time.Time, error) {
	es, err := sinceReset(epicDir)
	if err != nil {
		return time.Time{}, err
	}
	for _, e := range es {
		if e.Kind == k {
			return e.Time, nil
		}
	}
	return time.Time{}, nil
}
