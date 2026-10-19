package ralphloop

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/events"
)

// adoptLiveSession leaves a live session of ticket 01's iteration, as a
// racing gx process would, whose logged launch baseline is Turn 1 and which
// has since started turns more turns, now idle.
func adoptLiveSession(t *testing.T, d Deps, scratchDir string, turns int) string {
	t.Helper()
	r := fakeRunner(d)
	label := iterLabel("epic", "01")
	s, err := r.Start(agentrunner.StartOptions{Label: label, Epic: "epic", Cwd: iterationWorktreePath("/fake/worktrees", "epic", "01")})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	for range 1 + turns {
		if err := r.Prompt(s, "/implement"); err != nil {
			t.Fatalf("Prompt: %v", err)
		}
	}
	r.SetState(label, agentrunner.StateIdle, "")
	if err := logEvent(scratchDir, "epic", Event{Type: string(events.IterationStarted), Ticket: "01", AgentSession: s.SessionID, StateChangeSeq: 1}); err != nil {
		t.Fatalf("logEvent: %v", err)
	}
	return label
}

func TestRun_AdoptedSessionStalledSinceLaunch_IsPrompted(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: open\ntype: implement\n---\n# A\n",
	})
	d, _, _ := fakeDeps()
	label := adoptLiveSession(t, d, scratchDir, 0)

	if err := Run(RunOptions{EpicName: "epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, newRecordingEventSink()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if got := fakeRunner(d).Prompts(label); len(got) != 2 {
		t.Errorf("prompts = %v, want 2 (the stalled launch's, then a re-prompt on adoption)", got)
	}
}

func TestRun_AdoptedFinishedSession_IsNotPrompted(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: open\ntype: implement\n---\n# A\n",
	})
	d, _, _ := fakeDeps()
	label := adoptLiveSession(t, d, scratchDir, 1)

	if err := Run(RunOptions{EpicName: "epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, newRecordingEventSink()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if got := fakeRunner(d).Prompts(label); len(got) != 2 {
		t.Errorf("prompts = %v, want only the 2 sent before adoption", got)
	}
}

// TestRun_AdoptedStalledSession_PromptRefusedWhileBusy_KeepsWaiting: the
// stalled session picked up a turn before the re-prompt landed, so the
// refused prompt is skipped and the iteration waits for that turn.
func TestRun_AdoptedStalledSession_PromptRefusedWhileBusy_KeepsWaiting(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: open\ntype: implement\n---\n# A\n",
	})
	d, _, _ := fakeDeps()
	label := adoptLiveSession(t, d, scratchDir, 0)
	onRunnerPrompt(d, func(s agentrunner.Session, text string) error {
		r := fakeRunner(d)
		r.SetState(s.Label, agentrunner.StateWorking, "")
		time.AfterFunc(200*time.Millisecond, func() { r.SetState(s.Label, agentrunner.StateDone, "") })
		return agentrunner.ErrNotReady
	})

	if err := Run(RunOptions{EpicName: "epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, newRecordingEventSink()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if got := fakeRunner(d).Prompts(label); len(got) != 1 {
		t.Errorf("prompts = %v, want only the 1 sent before adoption (re-prompt skipped)", got)
	}
}

// TestRun_AdoptedStalledSession_PromptRefusedWhileBlocked_Parks: the stalled
// session sits on a dialog, so the re-prompt is refused and the iteration
// parks blocked-pane.
func TestRun_AdoptedStalledSession_PromptRefusedWhileBlocked_Parks(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: open\ntype: implement\n---\n# A\n",
	})
	path := filepath.Join(scratchDir, "epic", "issues", "01-a.md")
	d, _, _ := fakeDeps()
	adoptLiveSession(t, d, scratchDir, 0)
	onRunnerPrompt(d, func(s agentrunner.Session, text string) error {
		fakeRunner(d).SetState(s.Label, agentrunner.StateBlocked, "approval")
		return agentrunner.ErrNotReady
	})

	runUntilParked(t, RunOptions{EpicName: "epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, &recordingSink{})

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(raw), "status: needs-answer") {
		t.Errorf("ticket not marked needs-answer:\n%s", raw)
	}
}
