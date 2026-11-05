package ralphloop

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/ralphloop/agent"
	"github.com/elentok/gx/ralphloop/internal/fixture"
	"github.com/elentok/gx/ralphloop/runlog"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// finishFixture writes a one-ticket epic whose ticket is claimed, as the server
// leaves it before launching, and returns its scratch dir and ticket path.
func finishFixture(t *testing.T) (scratchDir, path string) {
	t.Helper()
	scratchDir = fixture.WriteEpic(t, "epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: claimed\ntype: implement\n---\n# A\n",
	})
	return scratchDir, filepath.Join(scratchDir, "epic", "issues", "01-a.md")
}

// driveIteration runs the fixture ticket through the path the server drives:
// PrepareIteration, launch, WaitIterationFinished, FinishIteration. A finish
// that pauses the ticket is an outcome, not an error.
func driveIteration(t *testing.T, d Deps, scratchDir, path string, sink runlog.EventSink) FinishOutcome {
	t.Helper()
	one := OneIteration{
		RepoDir: "/fake/repo", Epic: "epic", ScratchDir: scratchDir, Agent: agent.AgentClaude, Sink: sink,
		Ticket: tickets.Ticket{Identifier: "01", Path: path},
	}
	w, err := PrepareIteration(d, one)
	if err != nil {
		t.Fatalf("PrepareIteration: %v", err)
	}
	s, err := StartAndPrompt(context.Background(), d.Runner, agentrunner.StartOptions{Label: w.Label, Epic: "epic", Cwd: w.Path},
		"/gx-implement gx:epic/01", func(int, string) {})
	if err != nil {
		t.Fatalf("StartAndPrompt: %v", err)
	}
	if err := WaitIterationFinished(d, one, w, s); err != nil {
		t.Fatalf("WaitIterationFinished: %v", err)
	}
	out, err := FinishIteration(d, one, w, s.ID)
	if err != nil {
		t.Fatalf("FinishIteration: %v", err)
	}
	return out
}

// retryThenMergeCommits makes CommitsAhead report zero for the finish checks
// before the corrective retry and one commit after it.
func retryThenMergeCommits(d *Deps) {
	var calls int32
	d.CommitsAhead = func(dir, fromExclusive, toRef string) (int, error) {
		if atomic.AddInt32(&calls, 1) <= 2 {
			return 0, nil
		}
		return 1, nil
	}
}

// TestFinishIteration_UnexecutedToolCallDetected_RetryMergesCommits pins ticket
// 02's happy path: a zero-commit finish whose transcript matches the
// unexecuted-tool-call detector sends exactly one corrective prompt and
// re-waits for finish before any paused (answer) handling runs; when that retry
// merges commits, the ticket proceeds to normal merging instead of
// paused (answer).
func TestFinishIteration_UnexecutedToolCallDetected_RetryMergesCommits(t *testing.T) {
	t.Parallel()
	scratchDir, path := finishFixture(t)
	d, prompts, _ := fakeDeps()
	d.ReadUnexecutedToolCall = func(cwd, sessionID string) (bool, error) {
		return true, nil
	}
	fakeRunner(d).PromptState = agentrunner.StateIdle
	retryThenMergeCommits(&d)

	out := driveIteration(t, d, scratchDir, path, newRecordingEventSink())

	wantPrompts := 2
	if len(*prompts) != wantPrompts {
		t.Fatalf("prompts = %v, want %d (initial launch + one corrective retry)", *prompts, wantPrompts)
	}
	if !strings.Contains((*prompts)[1], unexecutedToolCallCorrection) {
		t.Errorf("second prompt = %q, want the unexecuted-tool-call corrective text", (*prompts)[1])
	}
	if !out.Merged {
		t.Errorf("outcome = %+v, want merged after the retry merged commits", out)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(raw), "status: done") {
		t.Errorf("ticket not marked done after the retry merged commits:\n%s", raw)
	}
	if rawIsPaused(t, string(raw), schema.PauseClassAnswer) {
		t.Errorf("ticket must not be paused for an answer once the retry merged commits:\n%s", raw)
	}
}

// TestFinishIteration_UnexecutedToolCallNotDetected_GoesStraightToAsking pins the
// non-match case: a zero-commit finish the detector doesn't match goes
// straight to the existing paused (answer) path exactly as before ticket 02,
// with no corrective retry prompt sent.
func TestFinishIteration_UnexecutedToolCallNotDetected_GoesStraightToAsking(t *testing.T) {
	t.Parallel()
	scratchDir, path := finishFixture(t)
	d, prompts, _ := fakeDeps()
	d.CommitsAhead = func(dir, fromExclusive, toRef string) (int, error) {
		return 0, nil
	}
	// d.ReadUnexecutedToolCall keeps fakeDeps' default: return false, nil.

	driveIteration(t, d, scratchDir, path, &recordingSink{})

	wantPrompts := 1
	if len(*prompts) != wantPrompts {
		t.Fatalf("prompts = %v, want %d (initial launch only, no corrective retry)", *prompts, wantPrompts)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	assertRawPaused(t, string(raw), schema.PauseClassAnswer, schema.PauseZeroCommit)
}

// TestFinishIteration_UnexecutedToolCallDetected_RetryStillZeroCommits_FallsToAsking
// pins ticket 02's one-shot cap: a zero-commit finish that still matches (and
// still produces zero commits) after the one retry falls through to
// paused (answer) rather than retrying again — exactly one corrective prompt is
// sent, not two.
func TestFinishIteration_UnexecutedToolCallDetected_RetryStillZeroCommits_FallsToAsking(t *testing.T) {
	t.Parallel()
	scratchDir, path := finishFixture(t)
	d, prompts, _ := fakeDeps()
	d.ReadUnexecutedToolCall = func(cwd, sessionID string) (bool, error) {
		return true, nil
	}
	fakeRunner(d).PromptState = agentrunner.StateIdle
	d.CommitsAhead = func(dir, fromExclusive, toRef string) (int, error) {
		return 0, nil
	}

	driveIteration(t, d, scratchDir, path, &recordingSink{})

	wantPrompts := 2
	if len(*prompts) != wantPrompts {
		t.Fatalf("prompts = %v, want %d (initial launch + exactly one corrective retry, no second)", *prompts, wantPrompts)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	assertRawPaused(t, string(raw), schema.PauseClassAnswer, "")
}

// TestFinishIteration_UnexecutedToolCallDetected_BlockedPane_SkipsCorrectiveRetry pins
// the blocked-pane rule: a matching zero-commit finish whose pane reads
// blocked at the moment of the check sends no corrective prompt at all and
// goes straight to paused (answer).
func TestFinishIteration_UnexecutedToolCallDetected_BlockedPane_SkipsCorrectiveRetry(t *testing.T) {
	t.Parallel()
	scratchDir, path := finishFixture(t)
	d, prompts, _ := fakeDeps()
	// The agent blocks after finishing its turn, by the time the check runs.
	d.ReadUnexecutedToolCall = func(cwd, sessionID string) (bool, error) {
		fakeRunner(d).SetState("epic-iter-01", agentrunner.StateBlocked, "approval")
		return true, nil
	}
	d.CommitsAhead = func(dir, fromExclusive, toRef string) (int, error) {
		return 0, nil
	}

	driveIteration(t, d, scratchDir, path, &recordingSink{})

	wantPrompts := 1
	if len(*prompts) != wantPrompts {
		t.Fatalf("prompts = %v, want %d (initial launch only, blocked pane gets no corrective prompt)", *prompts, wantPrompts)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	assertRawPaused(t, string(raw), schema.PauseClassAnswer, "")
}

// TestFinishIteration_UnexecutedToolCallRetry_PromptRefusedWhileBusy_KeepsWaiting pins
// the race where the agent resumed between the finish check and the
// corrective prompt: the refused prompt is skipped and the iteration waits
// for the turn that is already running, with no pause.
func TestFinishIteration_UnexecutedToolCallRetry_PromptRefusedWhileBusy_KeepsWaiting(t *testing.T) {
	t.Parallel()
	scratchDir, path := finishFixture(t)
	d, _, _ := fakeDeps()
	d.ReadUnexecutedToolCall = func(cwd, sessionID string) (bool, error) { return true, nil }
	fakeRunner(d).PromptState = agentrunner.StateIdle
	retryThenMergeCommits(&d)
	onRunnerPrompt(d, func(s agentrunner.Session, text string) error {
		if text != unexecutedToolCallCorrection {
			return nil
		}
		r := fakeRunner(d)
		r.SetState(s.Label, agentrunner.StateWorking, "")
		time.AfterFunc(200*time.Millisecond, func() { r.SetState(s.Label, agentrunner.StateDone, "") })
		return agentrunner.ErrNotReady
	})

	out := driveIteration(t, d, scratchDir, path, newRecordingEventSink())

	if !out.Merged {
		t.Errorf("outcome = %+v, want merged after waiting out the busy agent", out)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(raw), "status: done") {
		t.Errorf("ticket not done after waiting out the busy agent:\n%s", raw)
	}
}
