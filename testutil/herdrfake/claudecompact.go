package herdrfake

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/elentok/gx/transcript"
)

// CompactDurationMs is how many virtual milliseconds a modeled Claude Code
// "/compact" stays "working" before returning to "idle" — the same
// three-minute figure the production slow-compact regression
// (fix-compact-loop) reproduces, so a fake session exercises recovery logic
// against a realistically long compaction without a real wall-clock sleep.
const CompactDurationMs = 180_000

// compactAboveSmartZoneMargin and compactLowOccupancy are the deterministic
// occupancy figures ClaudeCompact writes: comfortably above the smart-zone
// threshold before a compaction, and comfortably below it afterward.
const (
	compactAboveSmartZoneMargin = 500
	compactLowOccupancy         = 50
)

// claudeCompactEpoch is the fixed origin every ClaudeCompact timestamp is
// offset from by the session's virtual clock, so transcript timestamps are
// deterministic run to run instead of depending on wall-clock time.
var claudeCompactEpoch = time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

// transcriptUsage, transcriptMessage, compactMetadata, and transcriptLine
// mirror the subset of a real Claude Code transcript JSONL line that
// transcript.ReadAll/LastAssistantUsage parse (see transcript/transcript.go),
// so lines ClaudeCompact writes are indistinguishable from production ones
// to those readers.
type transcriptUsage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
}

type transcriptMessage struct {
	Model string          `json:"model"`
	Usage transcriptUsage `json:"usage"`
}

type compactMetadata struct {
	Trigger string `json:"trigger"`
}

type transcriptLine struct {
	Type            string             `json:"type"`
	Subtype         string             `json:"subtype,omitempty"`
	Timestamp       string             `json:"timestamp"`
	SessionID       string             `json:"sessionId,omitempty"`
	Message         *transcriptMessage `json:"message,omitempty"`
	CompactMetadata *compactMetadata   `json:"compactMetadata,omitempty"`
}

// ClaudeCompact models one Claude Code agent session's transcript file and
// its "/compact" protocol: a valid transcript that starts above the
// smart-zone threshold, a compaction that stays "working" for exactly
// CompactDurationMs of virtual time, and completion recorded as one
// compact_boundary system line — the same shape production transcript readers
// and smart-zone recovery logic observe from a real Claude Code session. Both
// departures from that shape are opt-in, via ClaudeCompactOption.
//
// ClaudeCompact reads its clock through virtualTime rather than owning one,
// so it can be driven by a herdrfake.State's virtual clock (State.VirtualTime)
// shared with every other command a scenario dispatches.
type ClaudeCompact struct {
	mu sync.Mutex

	path        string
	sessionID   string
	virtualTime func() time.Duration

	prematureIdle bool
	pairedTurn    bool

	started         bool
	boundaryWritten bool
	startedAt       time.Duration
}

// ClaudeCompactOption configures a ClaudeCompact at construction. Both options
// exist so a scenario can model a pane real Claude Code produces but the
// honest default does not; neither changes the modeled compaction's duration
// or the virtual deadline its completion lines are timed at.
type ClaudeCompactOption func(*ClaudeCompact)

// WithPrematureIdlePane models the iter-13c pane: Status reports "idle" from
// the moment "/compact" is submitted, even though the compaction is still
// running and its boundary is not yet written. Real Claude Code reported idle
// seconds after "/compact" was submitted, so recovery logic that trusts pane
// status alone declares the compaction finished while it is still going.
func WithPrematureIdlePane() ClaudeCompactOption {
	return func(c *ClaudeCompact) { c.prematureIdle = true }
}

// WithPairedPostCompactionTurn appends a low-occupancy assistant turn after
// the boundary, modeling an agent that speaks immediately once compaction
// finishes. Real Claude Code writes no such turn at the boundary — the context
// is smaller but nothing says so until the agent's next turn — so this is
// opt-in for scenarios that need a fresh occupancy reading to assert on.
func WithPairedPostCompactionTurn() ClaudeCompactOption {
	return func(c *ClaudeCompact) { c.pairedTurn = true }
}

// NewClaudeCompact creates the transcript file for a session launched in cwd
// with sessionID, seeded with one assistant turn whose occupancy is above
// smartZone — the "over-budget transcript" precondition smart-zone recovery
// reacts to.
func NewClaudeCompact(t testing.TB, cwd, sessionID string, virtualTime func() time.Duration, smartZone int, opts ...ClaudeCompactOption) *ClaudeCompact {
	t.Helper()

	path, err := transcript.Path(cwd, sessionID)
	if err != nil {
		t.Fatalf("herdrfake: resolve transcript path: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("herdrfake: create transcript dir: %v", err)
	}

	c := &ClaudeCompact{path: path, sessionID: sessionID, virtualTime: virtualTime}
	for _, opt := range opts {
		opt(c)
	}

	if err := c.writeLine(assistantLine(sessionID, c.timestamp(), smartZone+compactAboveSmartZoneMargin)); err != nil {
		t.Fatalf("herdrfake: write initial transcript: %v", err)
	}

	return c
}

// Path returns the transcript file's real filesystem path.
func (c *ClaudeCompact) Path() string {
	return c.path
}

// StartCompact records that a "/compact" prompt was just submitted,
// beginning the CompactDurationMs virtual-time countdown Status polls
// against. Calling it again while a compaction is already active (started
// but its boundary not yet written) is a fake protocol error: production
// must submit "/compact" exactly once per breach.
func (c *ClaudeCompact) StartCompact() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.started && !c.boundaryWritten {
		return fmt.Errorf("herdrfake: /compact submitted while a compaction is already active")
	}
	c.started = true
	c.boundaryWritten = false
	c.startedAt = c.virtualTime()
	return nil
}

// activeLocked reports whether a compaction is currently running: started,
// but its completion boundary hasn't been written yet. Callers must hold
// c.mu.
func (c *ClaudeCompact) activeLocked() bool {
	return c.started && !c.boundaryWritten
}

// Status reports the modeled agent's current status: "working" for the
// first CompactDurationMs virtual milliseconds after StartCompact, then
// "idle" from the moment that elapses onward — or, under
// WithPrematureIdlePane, "idle" throughout. The first observation at or past
// that deadline appends the completion transcript lines — one
// compact_boundary system line, plus a low-occupancy assistant turn under
// WithPairedPostCompactionTurn — timed at exactly
// startedAt+CompactDurationMs regardless of how much later this call happens
// to run, so completion timestamps stay tied to the virtual deadline rather
// than poll timing. Before StartCompact has ever been called, Status reports
// "idle".
func (c *ClaudeCompact) Status() (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.activeLocked() {
		return "idle", nil
	}

	elapsed := c.virtualTime() - c.startedAt
	if elapsed < CompactDurationMs*time.Millisecond {
		if c.prematureIdle {
			return "idle", nil
		}
		return "working", nil
	}

	completedAt := c.startedAt + CompactDurationMs*time.Millisecond
	ts := claudeCompactEpoch.Add(completedAt).Format(time.RFC3339Nano)
	// Boundary first, paired turn second: a compaction that finished and an
	// agent that then spoke. Both carry the deadline's timestamp, which is why
	// readers distinguishing them must go by file order, not by timestamp.
	if err := c.writeLine(boundaryLine(ts)); err != nil {
		return "", err
	}
	if c.pairedTurn {
		if err := c.writeLine(assistantLine(c.sessionID, ts, compactLowOccupancy)); err != nil {
			return "", err
		}
	}
	c.boundaryWritten = true

	return "idle", nil
}

// SendKey models sending a key (e.g. "ctrl+c" or "enter") to this agent's
// pane. A second smart-zone Ctrl-C, or an Enter nudge, sent while a
// compaction is actively running is a fake protocol violation: either
// keypress can cancel a real in-progress compaction, so production must
// never send one once "/compact" is known to be working. SendKey fails fast
// instead of silently accepting it. The Ctrl-C that originally interrupts
// the agent to submit "/compact" happens before StartCompact is called, so
// it's unaffected by this check.
func (c *ClaudeCompact) SendKey(key string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.activeLocked() && (key == "ctrl+c" || key == "enter") {
		return fmt.Errorf("herdrfake: %s sent while compaction is active", key)
	}
	return nil
}

// AcceptFinishUp models gx's finish-up prompt being submitted to this
// agent. It fails unless the compact_boundary transcript line already
// exists, so a caller can't get ahead of a still-running compaction the way
// the production nudge bug used to.
func (c *ClaudeCompact) AcceptFinishUp() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.boundaryWritten {
		return fmt.Errorf("herdrfake: finish-up prompt submitted before compact boundary exists")
	}
	return nil
}

func (c *ClaudeCompact) timestamp() string {
	return claudeCompactEpoch.Add(c.virtualTime()).Format(time.RFC3339Nano)
}

func (c *ClaudeCompact) writeLine(line transcriptLine) error {
	b, err := json.Marshal(line)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(c.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(append(b, '\n'))
	return err
}

func assistantLine(sessionID, timestamp string, occupancy int) transcriptLine {
	return transcriptLine{
		Type:      "assistant",
		Timestamp: timestamp,
		SessionID: sessionID,
		Message: &transcriptMessage{
			Model: "claude",
			Usage: transcriptUsage{
				InputTokens:  occupancy,
				OutputTokens: 1,
			},
		},
	}
}

func boundaryLine(timestamp string) transcriptLine {
	return transcriptLine{
		Type:            "system",
		Subtype:         "compact_boundary",
		Timestamp:       timestamp,
		CompactMetadata: &compactMetadata{Trigger: "manual"},
	}
}
