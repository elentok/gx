package ralphloop

import (
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/elentok/gx/herdr"
	"github.com/elentok/gx/tickets/schema"
)

// retryTurnSelfReport wraps a Deps.AgentPrompt so the ticket picks up a
// self-report exactly when the corrective retry's own prompt (see
// retryUnexecutedToolCallOnce, unexecutedToolCallCorrection) is sent — never
// on the original turn's launch prompt, which carries different text. That
// pins these tests to ticket 04's ordering claim: the self-report belongs to
// the retry's own turn, not the original unexecuted-tool-call glitch that
// triggered the retry. It delegates to the given base (fakeDeps' own
// AgentPrompt) so prompt recording keeps working unchanged.
func retryTurnSelfReport(t *testing.T, path string, base func(herdr.AgentPromptOptions) (herdr.Agent, error), mutate func(*schema.Ticket)) func(herdr.AgentPromptOptions) (herdr.Agent, error) {
	t.Helper()
	return func(opts herdr.AgentPromptOptions) (herdr.Agent, error) {
		if strings.Contains(opts.Text, unexecutedToolCallCorrection) {
			if err := updateTicket(path, mutate); err != nil {
				t.Errorf("seeding retry-turn self-report on %s: %v", path, err)
			}
		}
		return base(opts)
	}
}

// TestRun_RetryTurnSelfReportsNeedsAnswer_ZeroCommits_ParksSelfReported pins
// ticket 04's first case: when the corrective retry's own turn ends
// ahead == 0 and self-reports needs-answer, the ticket must park with the
// self-reported kind and the agent's actual question, not
// ParkKindZeroCommit/"no commits landed" — even though the pre-retry turn
// matched the unexecuted-tool-call detector and carried no report of its own.
func TestRun_RetryTurnSelfReportsNeedsAnswer_ZeroCommits_ParksSelfReported(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: open\ntype: task\n---\n# A\n",
	})
	path := filepath.Join(scratchDir, "epic", "issues", "01-a.md")
	d, prompts, _ := fakeDeps()

	d.ReadUnexecutedToolCall = func(cwd, sessionID string) (bool, error) {
		return true, nil
	}
	d.AgentGet = func(target string) (herdr.Agent, error) {
		return herdr.Agent{PaneID: "pane-" + target, WorkspaceID: "ws1", TabID: "tab-" + target, AgentStatus: "idle", AgentSession: "session-" + target}, nil
	}
	d.CommitsAhead = func(dir, fromExclusive, toRef string) (int, error) {
		return 0, nil
	}
	d.AgentPrompt = retryTurnSelfReport(t, path, d.AgentPrompt, func(tk *schema.Ticket) {
		tk.IterationStatus = schema.IterationStatusNeedsAnswer
	})

	sink := &recordingSink{}
	runUntilParked(t, RunOptions{EpicName: "epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, sink)

	wantPrompts := 2
	if len(*prompts) != wantPrompts {
		t.Fatalf("prompts = %v, want %d (initial launch + one corrective retry)", *prompts, wantPrompts)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(raw), "status: needs-answer") {
		t.Errorf("ticket not marked needs-answer:\n%s", raw)
	}
	if !strings.Contains(string(raw), "park_kind: self-reported") {
		t.Errorf("ticket parked with the wrong kind, want park_kind: self-reported:\n%s", raw)
	}
	if strings.Contains(string(raw), "park_kind: zero-commit") {
		t.Errorf("ticket parked as zero-commit, want the retry's own self-report to take precedence:\n%s", raw)
	}

	if len(sink.ticketNeedsHumanCalls) != 1 {
		t.Fatalf("TicketNeedsHuman calls = %v, want exactly 1", sink.ticketNeedsHumanCalls)
	}
	if got := sink.ticketNeedsHumanCalls[0][3]; got != "agent reported needs-answer via iteration_status" {
		t.Errorf("TicketNeedsHuman reason = %q, want the self-reported message, not a zero-commit message", got)
	}
}

// TestRun_RetryTurnSelfReportsNeedsAnswer_LandsCommits_DoesNotSilentlyLand
// pins ticket 04's second case, ADR 0019's failure mode replayed on the
// retry's own turn: when the corrective retry lands commits (ahead > 0)
// alongside a needs-answer self-report, the ticket must park on the
// self-report rather than silently cherry-picking the partial work while
// dropping the unanswered question.
func TestRun_RetryTurnSelfReportsNeedsAnswer_LandsCommits_DoesNotSilentlyLand(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: open\ntype: task\n---\n# A\n",
	})
	path := filepath.Join(scratchDir, "epic", "issues", "01-a.md")
	d, prompts, _ := fakeDeps()

	d.ReadUnexecutedToolCall = func(cwd, sessionID string) (bool, error) {
		return true, nil
	}
	d.AgentGet = func(target string) (herdr.Agent, error) {
		return herdr.Agent{PaneID: "pane-" + target, WorkspaceID: "ws1", TabID: "tab-" + target, AgentStatus: "idle", AgentSession: "session-" + target}, nil
	}
	var calls int32
	d.CommitsAhead = func(dir, fromExclusive, toRef string) (int, error) {
		n := atomic.AddInt32(&calls, 1)
		if n <= 2 {
			return 0, nil
		}
		return 1, nil
	}
	cherryPickCalled := false
	d.CherryPickRange = func(dir, fromExclusive, toInclusive string) error {
		cherryPickCalled = true
		return nil
	}
	d.AgentPrompt = retryTurnSelfReport(t, path, d.AgentPrompt, func(tk *schema.Ticket) {
		tk.IterationStatus = schema.IterationStatusNeedsAnswer
	})

	sink := &recordingSink{}
	runUntilParked(t, RunOptions{EpicName: "epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, sink)

	wantPrompts := 2
	if len(*prompts) != wantPrompts {
		t.Fatalf("prompts = %v, want %d (initial launch + one corrective retry)", *prompts, wantPrompts)
	}
	if cherryPickCalled {
		t.Errorf("CherryPickRange called, want the retry's landed commits left unlanded behind the self-report")
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(raw), "status: needs-answer") {
		t.Errorf("ticket not marked needs-answer despite the retry landing commits alongside a self-report:\n%s", raw)
	}
	if strings.Contains(string(raw), "status: done") {
		t.Errorf("ticket must not be marked done while the retry's self-reported question is unanswered:\n%s", raw)
	}
	if !strings.Contains(string(raw), "park_kind: self-reported") {
		t.Errorf("ticket parked with the wrong kind, want park_kind: self-reported:\n%s", raw)
	}
}

// TestRun_RetryTurnSelfReportsCommitlessFinished_MarkedDone pins ticket 04's
// third case: when the corrective retry's own turn self-reports
// commitless-and-finished, the ticket is marked done via the existing
// commitless path rather than parked as a stall.
func TestRun_RetryTurnSelfReportsCommitlessFinished_MarkedDone(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: open\ntype: task\n---\n# A\n",
	})
	path := filepath.Join(scratchDir, "epic", "issues", "01-a.md")
	d, prompts, _ := fakeDeps()

	d.ReadUnexecutedToolCall = func(cwd, sessionID string) (bool, error) {
		return true, nil
	}
	d.AgentGet = func(target string) (herdr.Agent, error) {
		return herdr.Agent{PaneID: "pane-" + target, WorkspaceID: "ws1", TabID: "tab-" + target, AgentStatus: "idle", AgentSession: "session-" + target}, nil
	}
	d.CommitsAhead = func(dir, fromExclusive, toRef string) (int, error) {
		return 0, nil
	}
	d.AgentPrompt = retryTurnSelfReport(t, path, d.AgentPrompt, func(tk *schema.Ticket) {
		tk.IterationStatus = schema.IterationStatusFinished
		tk.Commitless = true
	})

	if err := Run(RunOptions{EpicName: "epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, noopEventSink{}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	wantPrompts := 2
	if len(*prompts) != wantPrompts {
		t.Fatalf("prompts = %v, want %d (initial launch + one corrective retry)", *prompts, wantPrompts)
	}

	events, ok, err := ReadEvents(scratchDir, "epic")
	if err != nil || !ok {
		t.Fatalf("ReadEvents: ok=%v err=%v", ok, err)
	}
	var commitless *Event
	for i, ev := range events {
		if ev.Type == eventCommitless && ev.Ticket == "01" {
			commitless = &events[i]
		}
		if ev.Type == eventNeedsAnswer {
			t.Errorf("events = %+v, want no needs-answer event for the retry's declared-commitless self-report", events)
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
	if strings.Contains(string(raw), "status: needs-answer") {
		t.Errorf("ticket must not be parked needs-answer for a declared-commitless retry finish:\n%s", raw)
	}
}
