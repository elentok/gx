package ralphloop

import (
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/tickets/schema"
)

// TestRun_UnexecutedToolCallDetected_RetryMergesCommits pins ticket 02's
// happy path: a zero-commit finish whose transcript matches the
// unexecuted-tool-call detector sends exactly one corrective prompt and
// re-waits for finish before any paused (answer) handling runs; when that retry
// merges commits, the ticket proceeds to normal merging instead of
// paused (answer).
func TestRun_UnexecutedToolCallDetected_RetryMergesCommits(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: open\ntype: implement\n---\n# A\n",
	})
	path := filepath.Join(scratchDir, "epic", "issues", "01-a.md")
	d, prompts, _ := fakeDeps()

	d.ReadUnexecutedToolCall = func(cwd, sessionID string) (bool, error) {
		return true, nil
	}
	fakeRunner(d).PromptState = agentrunner.StateIdle
	var calls int32
	d.CommitsAhead = func(dir, fromExclusive, toRef string) (int, error) {
		n := atomic.AddInt32(&calls, 1)
		if n <= 2 {
			return 0, nil
		}
		return 1, nil
	}

	if err := Run(RunOptions{EpicName: "epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, newRecordingEventSink()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	wantPrompts := 2
	if len(*prompts) != wantPrompts {
		t.Fatalf("prompts = %v, want %d (initial launch + one corrective retry)", *prompts, wantPrompts)
	}
	if !strings.Contains((*prompts)[1], unexecutedToolCallCorrection) {
		t.Errorf("second prompt = %q, want the unexecuted-tool-call corrective text", (*prompts)[1])
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

// TestRun_UnexecutedToolCallNotDetected_GoesStraightToAsking pins the
// non-match case: a zero-commit finish the detector doesn't match goes
// straight to the existing paused (answer) path exactly as before ticket 02,
// with no corrective retry prompt sent.
func TestRun_UnexecutedToolCallNotDetected_GoesStraightToAsking(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: open\ntype: implement\n---\n# A\n",
	})
	path := filepath.Join(scratchDir, "epic", "issues", "01-a.md")
	d, prompts, _ := fakeDeps()

	d.CommitsAhead = func(dir, fromExclusive, toRef string) (int, error) {
		return 0, nil
	}
	// d.ReadUnexecutedToolCall keeps fakeDeps' default: return false, nil.

	runUntilPaused(t, RunOptions{EpicName: "epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, &recordingSink{})

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

// TestRun_UnexecutedToolCallDetected_RetryStillZeroCommits_FallsToAsking
// pins ticket 02's one-shot cap: a zero-commit finish that still matches (and
// still produces zero commits) after the one retry falls through to
// paused (answer) rather than retrying again — exactly one corrective prompt is
// sent, not two.
func TestRun_UnexecutedToolCallDetected_RetryStillZeroCommits_FallsToAsking(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: open\ntype: implement\n---\n# A\n",
	})
	path := filepath.Join(scratchDir, "epic", "issues", "01-a.md")
	d, prompts, _ := fakeDeps()

	d.ReadUnexecutedToolCall = func(cwd, sessionID string) (bool, error) {
		return true, nil
	}
	fakeRunner(d).PromptState = agentrunner.StateIdle
	d.CommitsAhead = func(dir, fromExclusive, toRef string) (int, error) {
		return 0, nil
	}

	runUntilPaused(t, RunOptions{EpicName: "epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, &recordingSink{})

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

// TestRun_UnexecutedToolCallDetected_BlockedPane_SkipsCorrectiveRetry pins
// the blocked-pane rule: a matching zero-commit finish whose pane reads
// blocked at the moment of the check sends no corrective prompt at all and
// goes straight to paused (answer).
func TestRun_UnexecutedToolCallDetected_BlockedPane_SkipsCorrectiveRetry(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: open\ntype: implement\n---\n# A\n",
	})
	path := filepath.Join(scratchDir, "epic", "issues", "01-a.md")
	d, prompts, _ := fakeDeps()

	// The agent blocks after finishing its turn, by the time the check runs.
	d.ReadUnexecutedToolCall = func(cwd, sessionID string) (bool, error) {
		fakeRunner(d).SetState("epic-iter-01", agentrunner.StateBlocked, "approval")
		return true, nil
	}
	d.CommitsAhead = func(dir, fromExclusive, toRef string) (int, error) {
		return 0, nil
	}

	runUntilPaused(t, RunOptions{EpicName: "epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, &recordingSink{})

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

// TestRun_UnexecutedToolCallRetry_PromptRefusedWhileBusy_KeepsWaiting pins
// the race where the agent resumed between the finish check and the
// corrective prompt: the refused prompt is skipped and the iteration waits
// for the turn that is already running, with no pause.
func TestRun_UnexecutedToolCallRetry_PromptRefusedWhileBusy_KeepsWaiting(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: open\ntype: implement\n---\n# A\n",
	})
	path := filepath.Join(scratchDir, "epic", "issues", "01-a.md")
	d, _, _ := fakeDeps()

	d.ReadUnexecutedToolCall = func(cwd, sessionID string) (bool, error) { return true, nil }
	fakeRunner(d).PromptState = agentrunner.StateIdle
	var calls int32
	d.CommitsAhead = func(dir, fromExclusive, toRef string) (int, error) {
		if atomic.AddInt32(&calls, 1) <= 2 {
			return 0, nil
		}
		return 1, nil
	}
	onRunnerPrompt(d, func(s agentrunner.Session, text string) error {
		if text != unexecutedToolCallCorrection {
			return nil
		}
		r := fakeRunner(d)
		r.SetState(s.Label, agentrunner.StateWorking, "")
		time.AfterFunc(200*time.Millisecond, func() { r.SetState(s.Label, agentrunner.StateDone, "") })
		return agentrunner.ErrNotReady
	})

	if err := Run(RunOptions{EpicName: "epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, newRecordingEventSink()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(raw), "status: done") {
		t.Errorf("ticket not done after waiting out the busy agent:\n%s", raw)
	}
}

// TestRun_UnexecutedToolCallRetry_PromptRefusedWhileBlocked_Pauses is the
// blocked counterpart: the agent blocked on a dialog between the check and
// the prompt, so the iteration pauses blocked-pane.
func TestRun_UnexecutedToolCallRetry_PromptRefusedWhileBlocked_Pauses(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: open\ntype: implement\n---\n# A\n",
	})
	path := filepath.Join(scratchDir, "epic", "issues", "01-a.md")
	d, _, _ := fakeDeps()

	d.ReadUnexecutedToolCall = func(cwd, sessionID string) (bool, error) { return true, nil }
	fakeRunner(d).PromptState = agentrunner.StateIdle
	d.CommitsAhead = func(dir, fromExclusive, toRef string) (int, error) { return 0, nil }
	onRunnerPrompt(d, func(s agentrunner.Session, text string) error {
		if text != unexecutedToolCallCorrection {
			return nil
		}
		fakeRunner(d).SetState(s.Label, agentrunner.StateBlocked, "approval")
		return agentrunner.ErrNotReady
	})

	runUntilPaused(t, RunOptions{EpicName: "epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, &recordingSink{})

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	assertRawPaused(t, string(raw), schema.PauseClassAnswer, "")
}
