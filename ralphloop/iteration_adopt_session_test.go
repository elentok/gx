package ralphloop

import (
	"testing"

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
