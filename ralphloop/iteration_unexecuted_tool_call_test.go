package ralphloop

import (
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/elentok/gx/herdr"
)

// TestRun_UnexecutedToolCallDetected_RetryLandsCommits pins ticket 02's
// happy path: a zero-commit finish whose transcript matches the
// unexecuted-tool-call detector sends exactly one corrective prompt and
// re-waits for finish before any needs-answer handling runs; when that retry
// lands commits, the ticket proceeds to normal landing instead of
// needs-answer.
func TestRun_UnexecutedToolCallDetected_RetryLandsCommits(t *testing.T) {
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
		t.Errorf("ticket not marked done after the retry landed commits:\n%s", raw)
	}
	if strings.Contains(string(raw), "status: needs-answer") {
		t.Errorf("ticket must not be parked needs-answer once the retry landed commits:\n%s", raw)
	}
}

// TestRun_UnexecutedToolCallNotDetected_GoesStraightToNeedsAnswer pins the
// non-match case: a zero-commit finish the detector doesn't match goes
// straight to the existing needs-answer path exactly as before ticket 02,
// with no corrective retry prompt sent.
func TestRun_UnexecutedToolCallNotDetected_GoesStraightToNeedsAnswer(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: open\ntype: task\n---\n# A\n",
	})
	path := filepath.Join(scratchDir, "epic", "issues", "01-a.md")
	d, prompts, _ := fakeDeps()

	d.CommitsAhead = func(dir, fromExclusive, toRef string) (int, error) {
		return 0, nil
	}
	// d.ReadUnexecutedToolCall keeps fakeDeps' default: return false, nil.

	runUntilParked(t, RunOptions{EpicName: "epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, &recordingSink{})

	wantPrompts := 1
	if len(*prompts) != wantPrompts {
		t.Fatalf("prompts = %v, want %d (initial launch only, no corrective retry)", *prompts, wantPrompts)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(raw), "status: needs-answer") {
		t.Errorf("ticket not marked needs-answer:\n%s", raw)
	}
	if !strings.Contains(string(raw), "park_kind: zero-commit") {
		t.Errorf("ticket not stamped park_kind: zero-commit:\n%s", raw)
	}
}

// TestRun_UnexecutedToolCallDetected_RetryStillZeroCommits_FallsToNeedsAnswer
// pins ticket 02's one-shot cap: a zero-commit finish that still matches (and
// still produces zero commits) after the one retry falls through to
// needs-answer rather than retrying again — exactly one corrective prompt is
// sent, not two.
func TestRun_UnexecutedToolCallDetected_RetryStillZeroCommits_FallsToNeedsAnswer(t *testing.T) {
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

	runUntilParked(t, RunOptions{EpicName: "epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, &recordingSink{})

	wantPrompts := 2
	if len(*prompts) != wantPrompts {
		t.Fatalf("prompts = %v, want %d (initial launch + exactly one corrective retry, no second)", *prompts, wantPrompts)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(raw), "status: needs-answer") {
		t.Errorf("ticket not marked needs-answer after the retry still produced zero commits:\n%s", raw)
	}
}

// TestRun_UnexecutedToolCallDetected_BlockedPane_SkipsCorrectiveRetry pins
// the blocked-pane rule: a matching zero-commit finish whose pane reads
// blocked at the moment of the check sends no corrective prompt at all and
// goes straight to needs-answer.
func TestRun_UnexecutedToolCallDetected_BlockedPane_SkipsCorrectiveRetry(t *testing.T) {
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
		return herdr.Agent{PaneID: "pane-" + target, WorkspaceID: "ws1", TabID: "tab-" + target, AgentStatus: "blocked", AgentSession: "session-" + target}, nil
	}
	d.CommitsAhead = func(dir, fromExclusive, toRef string) (int, error) {
		return 0, nil
	}

	runUntilParked(t, RunOptions{EpicName: "epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, &recordingSink{})

	wantPrompts := 1
	if len(*prompts) != wantPrompts {
		t.Fatalf("prompts = %v, want %d (initial launch only, blocked pane gets no corrective prompt)", *prompts, wantPrompts)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(raw), "status: needs-answer") {
		t.Errorf("ticket not marked needs-answer:\n%s", raw)
	}
}
