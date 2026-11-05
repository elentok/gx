package ralphloop

import (
	"github.com/elentok/gx/ralphloop/deps"
	"github.com/elentok/gx/ralphloop/ticketops"
	"os"
	"strings"
	"testing"

	"github.com/elentok/gx/agentrunner"
	eventsc "github.com/elentok/gx/events"
	"github.com/elentok/gx/ralphloop/runlog"
	"github.com/elentok/gx/tickets/schema"
)

// retryTurnSelfReport makes the ticket pick up a self-report exactly when the
// corrective retry's own prompt (see retryUnexecutedToolCallOnce,
// unexecutedToolCallCorrection) is sent — never on the original turn's launch
// prompt, which carries different text. That pins these tests to ticket 04's
// ordering claim: the self-report belongs to the retry's own turn, not the
// original unexecuted-tool-call glitch that triggered the retry.
func retryTurnSelfReport(t *testing.T, d deps.Deps, path string, mutate func(*schema.Ticket)) {
	t.Helper()
	onRunnerPrompt(d, func(_ agentrunner.Session, text string) error {
		if strings.Contains(text, unexecutedToolCallCorrection) {
			if err := ticketops.UpdateTicket(path, mutate); err != nil {
				t.Errorf("seeding retry-turn self-report on %s: %v", path, err)
			}
		}
		return nil
	})
}

// TestFinishIteration_RetryTurnSelfReportsAsking_ZeroCommits_PausesSelfReported pins
// ticket 04's first case: when the corrective retry's own turn ends
// ahead == 0 and self-reports paused (answer), the ticket must pause with the
// self-reported kind and the agent's actual question, not
// PauseKindZeroCommit/"no commits merged" — even though the pre-retry turn
// matched the unexecuted-tool-call detector and carried no report of its own.
func TestFinishIteration_RetryTurnSelfReportsAsking_ZeroCommits_PausesSelfReported(t *testing.T) {
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
	retryTurnSelfReport(t, d, path, func(tk *schema.Ticket) {
		tk.IterationStatus = schema.IterationStatusAsking
	})

	sink := &recordingSink{}
	driveIteration(t, d, scratchDir, path, sink)

	wantPrompts := 2
	if len(*prompts) != wantPrompts {
		t.Fatalf("prompts = %v, want %d (initial launch + one corrective retry)", *prompts, wantPrompts)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	// Self-reported, not zero-commit: the retry's own self-report takes precedence.
	assertRawPaused(t, string(raw), schema.PauseClassAnswer, schema.PauseSelfReported)

	if len(sink.ticketNeedsHumanCalls) != 1 {
		t.Fatalf("TicketNeedsHuman calls = %v, want exactly 1", sink.ticketNeedsHumanCalls)
	}
	if got := sink.ticketNeedsHumanCalls[0][3]; got != "agent asked via iteration_status" {
		t.Errorf("TicketNeedsHuman reason = %q, want the self-reported message, not a zero-commit message", got)
	}
}

// TestFinishIteration_RetryTurnSelfReportsAsking_MergesCommits_DoesNotSilentlyMerge
// pins ticket 04's second case, ADR 0019's failure mode replayed on the
// retry's own turn: when the corrective retry merges commits (ahead > 0)
// alongside a paused (answer) self-report, the ticket must pause on the
// self-report rather than silently cherry-picking the partial work while
// dropping the unanswered question.
func TestFinishIteration_RetryTurnSelfReportsAsking_MergesCommits_DoesNotSilentlyMerge(t *testing.T) {
	t.Parallel()
	scratchDir, path := finishFixture(t)
	d, prompts, _ := fakeDeps()
	d.ReadUnexecutedToolCall = func(cwd, sessionID string) (bool, error) {
		return true, nil
	}
	fakeRunner(d).PromptState = agentrunner.StateIdle
	retryThenMergeCommits(&d)
	cherryPickCalled := false
	d.CherryPickRange = func(dir, fromExclusive, toInclusive string) error {
		cherryPickCalled = true
		return nil
	}
	retryTurnSelfReport(t, d, path, func(tk *schema.Ticket) {
		tk.IterationStatus = schema.IterationStatusAsking
	})

	out := driveIteration(t, d, scratchDir, path, &recordingSink{})

	wantPrompts := 2
	if len(*prompts) != wantPrompts {
		t.Fatalf("prompts = %v, want %d (initial launch + one corrective retry)", *prompts, wantPrompts)
	}
	if cherryPickCalled || out.Merged {
		t.Errorf("CherryPickRange called = %v, outcome = %+v, want the retry's merged commits left unmerged behind the self-report", cherryPickCalled, out)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	assertRawPaused(t, string(raw), schema.PauseClassAnswer, schema.PauseSelfReported)
	if strings.Contains(string(raw), "status: done") {
		t.Errorf("ticket must not be marked done while the retry's self-reported question is unanswered:\n%s", raw)
	}
}

// TestFinishIteration_RetryTurnSelfReportsCommitlessFinished_MarkedDone pins ticket 04's
// third case: when the corrective retry's own turn self-reports
// commitless-and-finished, the ticket is marked done via the existing
// commitless path rather than paused as a stall.
func TestFinishIteration_RetryTurnSelfReportsCommitlessFinished_MarkedDone(t *testing.T) {
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
	retryTurnSelfReport(t, d, path, func(tk *schema.Ticket) {
		tk.IterationStatus = schema.IterationStatusFinished
		tk.Commitless = true
	})

	out := driveIteration(t, d, scratchDir, path, runlog.NoopEventSink{})

	wantPrompts := 2
	if len(*prompts) != wantPrompts {
		t.Fatalf("prompts = %v, want %d (initial launch + one corrective retry)", *prompts, wantPrompts)
	}
	if !out.Merged {
		t.Errorf("outcome = %+v, want done for the retry's declared-commitless finish", out)
	}

	events, ok, err := runlog.ReadEvents(scratchDir, "epic")
	if err != nil || !ok {
		t.Fatalf("ReadEvents: ok=%v err=%v", ok, err)
	}
	var commitless *runlog.Event
	for i, ev := range events {
		if ev.Type == string(eventsc.Commitless) && ev.Ticket == "01" {
			commitless = &events[i]
		}
		if ev.Type == string(eventsc.Paused) {
			t.Errorf("events = %+v, want no paused event for the retry's declared-commitless self-report", events)
		}
	}
	if commitless == nil {
		t.Fatalf("events = %+v, want a commitless event for ticket 01", events)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(raw), "status: done") {
		t.Errorf("ticket not marked done for the retry's commitless-and-finished self-report:\n%s", raw)
	}
	if rawIsPaused(t, string(raw), schema.PauseClassAnswer) {
		t.Errorf("ticket must not be paused (answer) for a declared-commitless retry finish:\n%s", raw)
	}
}
