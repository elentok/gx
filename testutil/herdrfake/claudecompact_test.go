package herdrfake

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/elentok/gx/transcript"
)

const compactTestSmartZone = 100

// newTestClaudeCompact wires a ClaudeCompact against a fresh $HOME and a
// virtual clock the test advances explicitly (never a real sleep). Options are
// threaded through rather than set inside, so a test that wants non-default
// pane behavior asks for it and every other test keeps the defaults.
func newTestClaudeCompact(t *testing.T, opts ...ClaudeCompactOption) (*ClaudeCompact, *time.Duration) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())

	var virtualTime time.Duration
	c := NewClaudeCompact(t, "/repo/iter-c", "sess-c", func() time.Duration { return virtualTime }, compactTestSmartZone, opts...)
	return c, &virtualTime
}

func TestClaudeCompact_InitialTranscriptStartsAboveSmartZone(t *testing.T) {
	c, _ := newTestClaudeCompact(t)

	occupancy, ok, err := transcript.LastAssistantOccupancy("/repo/iter-c", "sess-c")
	if err != nil || !ok {
		t.Fatalf("LastAssistantOccupancy: ok=%v err=%v", ok, err)
	}
	if occupancy <= compactTestSmartZone {
		t.Errorf("initial occupancy = %d, want > smartZone (%d)", occupancy, compactTestSmartZone)
	}
	_ = c
}

func TestClaudeCompact_StatusStaysWorkingUntilExactlyCompactDuration(t *testing.T) {
	c, virtualTime := newTestClaudeCompact(t)

	if err := c.StartCompact(); err != nil {
		t.Fatalf("StartCompact: %v", err)
	}

	*virtualTime = (CompactDurationMs - 1) * time.Millisecond
	if status, err := c.Status(); err != nil || status != "working" {
		t.Fatalf("Status() at duration-1ms = %q, err=%v, want working", status, err)
	}

	*virtualTime = CompactDurationMs * time.Millisecond
	status, err := c.Status()
	if err != nil {
		t.Fatalf("Status(): %v", err)
	}
	if status != "idle" {
		t.Fatalf("Status() at exactly CompactDurationMs = %q, want idle", status)
	}
}

func TestClaudeCompact_CompletionAppendsOneBoundaryAndLowOccupancyTurn(t *testing.T) {
	c, virtualTime := newTestClaudeCompact(t, WithPairedPostCompactionTurn())

	if err := c.StartCompact(); err != nil {
		t.Fatalf("StartCompact: %v", err)
	}
	*virtualTime = CompactDurationMs * time.Millisecond
	if _, err := c.Status(); err != nil {
		t.Fatalf("Status(): %v", err)
	}
	// A second poll after completion must not append another boundary.
	if _, err := c.Status(); err != nil {
		t.Fatalf("Status() (second poll): %v", err)
	}

	lines, ok, err := transcript.ReadAll(c.Path())
	if err != nil || !ok {
		t.Fatalf("ReadAll: ok=%v err=%v", ok, err)
	}
	if got := transcript.CountCompactions(lines); got != 1 {
		t.Errorf("CountCompactions = %d, want exactly 1", got)
	}

	occupancy, ok, err := transcript.LastAssistantOccupancy("/repo/iter-c", "sess-c")
	if err != nil || !ok {
		t.Fatalf("LastAssistantOccupancy: ok=%v err=%v", ok, err)
	}
	if occupancy >= compactTestSmartZone {
		t.Errorf("post-compact occupancy = %d, want < smartZone (%d)", occupancy, compactTestSmartZone)
	}
}

func TestClaudeCompact_CompletionAppendsOnlyABoundaryByDefault(t *testing.T) {
	c, virtualTime := newTestClaudeCompact(t)

	if err := c.StartCompact(); err != nil {
		t.Fatalf("StartCompact: %v", err)
	}
	*virtualTime = CompactDurationMs * time.Millisecond
	if _, err := c.Status(); err != nil {
		t.Fatalf("Status(): %v", err)
	}

	lines, ok, err := transcript.ReadAll(c.Path())
	if err != nil || !ok {
		t.Fatalf("ReadAll: ok=%v err=%v", ok, err)
	}
	if got := transcript.CountCompactions(lines); got != 1 {
		t.Errorf("CountCompactions = %d, want exactly 1", got)
	}
	if len(lines) != 2 {
		t.Fatalf("line count = %d, want 2 (seed turn + boundary)", len(lines))
	}
	if !lines[1].IsCompactBoundary() {
		t.Errorf("last line = %+v, want the compact boundary", lines[1])
	}

	// No assistant turn follows the boundary, so the newest occupancy in the
	// transcript is still the pre-compaction one — the real Claude Code shape.
	occupancy, ok, err := transcript.LastAssistantOccupancy("/repo/iter-c", "sess-c")
	if err != nil || !ok {
		t.Fatalf("LastAssistantOccupancy: ok=%v err=%v", ok, err)
	}
	if occupancy <= compactTestSmartZone {
		t.Errorf("post-compact occupancy = %d, want the stale above-smartZone (%d) value", occupancy, compactTestSmartZone)
	}
}

func TestClaudeCompact_PrematureIdlePaneReportsIdleWhileCompactionRuns(t *testing.T) {
	c, virtualTime := newTestClaudeCompact(t, WithPrematureIdlePane())

	if err := c.StartCompact(); err != nil {
		t.Fatalf("StartCompact: %v", err)
	}

	*virtualTime = (CompactDurationMs / 2) * time.Millisecond
	if status, err := c.Status(); err != nil || status != "idle" {
		t.Fatalf("Status() mid-compaction = %q, err=%v, want idle", status, err)
	}

	lines, ok, err := transcript.ReadAll(c.Path())
	if err != nil || !ok {
		t.Fatalf("ReadAll: ok=%v err=%v", ok, err)
	}
	if got := transcript.CountCompactions(lines); got != 0 {
		t.Errorf("CountCompactions mid-compaction = %d, want 0", got)
	}
	if err := c.AcceptFinishUp(); err == nil {
		t.Error("AcceptFinishUp() mid-compaction = nil, want error")
	}

	*virtualTime = CompactDurationMs * time.Millisecond
	if status, err := c.Status(); err != nil || status != "idle" {
		t.Fatalf("Status() at CompactDurationMs = %q, err=%v, want idle", status, err)
	}

	lines, ok, err = transcript.ReadAll(c.Path())
	if err != nil || !ok {
		t.Fatalf("ReadAll: ok=%v err=%v", ok, err)
	}
	if got := transcript.CountCompactions(lines); got != 1 {
		t.Fatalf("CountCompactions after the deadline = %d, want exactly 1", got)
	}
	wantTS := claudeCompactEpoch.Add(CompactDurationMs * time.Millisecond)
	if got := lines[len(lines)-1].Timestamp; !got.Equal(wantTS) {
		t.Errorf("boundary timestamp = %v, want the virtual deadline %v", got, wantTS)
	}
}

func TestClaudeCompact_PrematureIdlePaneStillWritesPairedTurnAtTheDeadline(t *testing.T) {
	c, virtualTime := newTestClaudeCompact(t, WithPrematureIdlePane(), WithPairedPostCompactionTurn())

	if err := c.StartCompact(); err != nil {
		t.Fatalf("StartCompact: %v", err)
	}
	*virtualTime = CompactDurationMs * time.Millisecond
	if _, err := c.Status(); err != nil {
		t.Fatalf("Status(): %v", err)
	}

	lines, ok, err := transcript.ReadAll(c.Path())
	if err != nil || !ok {
		t.Fatalf("ReadAll: ok=%v err=%v", ok, err)
	}
	if len(lines) != 3 {
		t.Fatalf("line count = %d, want 3 (seed turn + boundary + paired turn)", len(lines))
	}
	if !lines[1].IsCompactBoundary() {
		t.Errorf("line 1 = %+v, want the boundary written before the paired turn", lines[1])
	}
	wantTS := claudeCompactEpoch.Add(CompactDurationMs * time.Millisecond)
	for i, line := range lines[1:] {
		if !line.Timestamp.Equal(wantTS) {
			t.Errorf("completion line %d timestamp = %v, want the virtual deadline %v", i+1, line.Timestamp, wantTS)
		}
	}
}

func TestClaudeCompact_FinishUpRejectedBeforeBoundaryExists(t *testing.T) {
	c, virtualTime := newTestClaudeCompact(t)

	if err := c.AcceptFinishUp(); err == nil {
		t.Error("AcceptFinishUp() before any compaction = nil, want error")
	}

	if err := c.StartCompact(); err != nil {
		t.Fatalf("StartCompact: %v", err)
	}
	*virtualTime = (CompactDurationMs - 1) * time.Millisecond
	if _, err := c.Status(); err != nil {
		t.Fatalf("Status(): %v", err)
	}
	if err := c.AcceptFinishUp(); err == nil {
		t.Error("AcceptFinishUp() mid-compaction = nil, want error")
	}

	*virtualTime = CompactDurationMs * time.Millisecond
	if _, err := c.Status(); err != nil {
		t.Fatalf("Status(): %v", err)
	}
	if err := c.AcceptFinishUp(); err != nil {
		t.Errorf("AcceptFinishUp() after boundary = %v, want nil", err)
	}
}

func TestClaudeCompact_SecondCtrlCOrEnterDuringActiveCompactionFailsImmediately(t *testing.T) {
	c, virtualTime := newTestClaudeCompact(t)

	// The first Ctrl-C (interrupting the agent before "/compact" is
	// submitted) must still be accepted.
	if err := c.SendKey("ctrl+c"); err != nil {
		t.Errorf("SendKey(ctrl+c) before StartCompact = %v, want nil", err)
	}

	if err := c.StartCompact(); err != nil {
		t.Fatalf("StartCompact: %v", err)
	}
	*virtualTime = (CompactDurationMs / 2) * time.Millisecond

	if err := c.SendKey("ctrl+c"); err == nil {
		t.Error("SendKey(ctrl+c) during active compaction = nil, want error")
	}
	if err := c.SendKey("enter"); err == nil {
		t.Error("SendKey(enter) during active compaction = nil, want error")
	}

	*virtualTime = CompactDurationMs * time.Millisecond
	if _, err := c.Status(); err != nil {
		t.Fatalf("Status(): %v", err)
	}
	if err := c.SendKey("enter"); err != nil {
		t.Errorf("SendKey(enter) after compaction completed = %v, want nil", err)
	}
}

func TestClaudeCompact_AcceptPrompt_SwitchOffAcceptsEvenWhenBlocked(t *testing.T) {
	c, _ := newTestClaudeCompact(t)
	c.SetBlocked(true)
	if err := c.AcceptPrompt("iter-c"); err != nil {
		t.Errorf("AcceptPrompt() = %v, want nil (switch is off)", err)
	}
}

func TestClaudeCompact_AcceptPrompt_SwitchOnAcceptsWhenNotBlocked(t *testing.T) {
	c, _ := newTestClaudeCompact(t, WithAgentBlockedRejection())
	if err := c.AcceptPrompt("iter-c"); err != nil {
		t.Errorf("AcceptPrompt() = %v, want nil (pane not blocked)", err)
	}
}

func TestClaudeCompact_AcceptPrompt_SwitchOnRejectsWhenBlocked(t *testing.T) {
	c, _ := newTestClaudeCompact(t, WithAgentBlockedRejection())
	c.SetBlocked(true)

	err := c.AcceptPrompt("iter-c")
	if err == nil {
		t.Fatal("AcceptPrompt() = nil, want the agent_blocked envelope")
	}

	var envelope struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if jsonErr := json.Unmarshal([]byte(err.Error()), &envelope); jsonErr != nil {
		t.Fatalf("AcceptPrompt() error is not the JSON envelope herdr.run classifies: %v", jsonErr)
	}
	if envelope.Error.Code != "agent_blocked" {
		t.Errorf("envelope code = %q, want %q (the code herdr.run's classifier switches on)", envelope.Error.Code, "agent_blocked")
	}
}

func TestClaudeCompact_TranscriptTimestampsAndUsageAreDeterministic(t *testing.T) {
	run := func() []transcript.Line {
		c, virtualTime := newTestClaudeCompact(t)
		if err := c.StartCompact(); err != nil {
			t.Fatalf("StartCompact: %v", err)
		}
		*virtualTime = CompactDurationMs * time.Millisecond
		if _, err := c.Status(); err != nil {
			t.Fatalf("Status(): %v", err)
		}
		lines, ok, err := transcript.ReadAll(c.Path())
		if err != nil || !ok {
			t.Fatalf("ReadAll: ok=%v err=%v", ok, err)
		}
		return lines
	}

	first := run()
	second := run()

	if len(first) != len(second) {
		t.Fatalf("line count differs across runs: %d vs %d", len(first), len(second))
	}
	for i := range first {
		if !first[i].Timestamp.Equal(second[i].Timestamp) {
			t.Errorf("line %d timestamp differs across runs: %v vs %v", i, first[i].Timestamp, second[i].Timestamp)
		}
		if first[i].Usage != second[i].Usage {
			t.Errorf("line %d usage differs across runs: %+v vs %+v", i, first[i].Usage, second[i].Usage)
		}
	}
}
