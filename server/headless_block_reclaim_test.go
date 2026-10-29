package server_test

import (
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/nativerunner"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/testutil/runnerfake"
	"github.com/elentok/gx/tickets/schema"
)

var openRequest = &agentrunner.Pending{ID: "r1", Tool: "Bash", Input: []byte(`{"command":"ls"}`)}

// restartHeadlessClaimed restarts a server over a claimed ticket whose headless
// claude agent has the given verdict. prepare runs on the staged handle and the
// fake, as the server's last save and the agent's event log would have left them.
func restartHeadlessClaimed(t *testing.T, verdict nativerunner.Verdict, prepare func(*servertest.Harness, *headlessFake)) (*servertest.Harness, string, *headlessFake) {
	t.Helper()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	fake := &headlessFake{Runner: runnerfake.NewRunner(), verdict: verdict}
	h := servertest.StartWithStore(t, store, servertest.WithRunner(fake))
	wtPath, ticketPath := leaveIteration(t, h, store, repo)
	if _, err := fake.Start(agentrunner.StartOptions{Label: "epic-a-iter-01", Cwd: wtPath}); err != nil {
		t.Fatal(err)
	}
	stampRun(t, h, string(agentrunner.ChoiceHeadless), agentrunner.Session{Label: "epic-a-iter-01", ID: "p1"})
	prepare(h, fake)
	h.Restart(t)
	return h, ticketPath, fake
}

func TestHeadlessBlockReclaim_OpenRequestRepausesStuckClaimed(t *testing.T) {
	t.Parallel()
	h, ticketPath, _ := restartHeadlessClaimed(t, nativerunner.VerdictLive, func(_ *servertest.Harness, f *headlessFake) {
		f.SetPending("epic-a-iter-01", openRequest)
	})
	waitStatus(t, ticketPath, schema.StatusLegacyAnswer)
	tk, err := schema.ParseTicket(ticketPath)
	if err != nil {
		t.Fatal(err)
	}
	assertPaused(t, tk, schema.PauseClassAnswer, schema.PauseBlockedPane)
	// The slot grace clock starts at the pause, not when the block began.
	waitFor(t, func() bool { return runsJSONHas(h, `"waiting_since"`) })
}

func TestHeadlessBlockReclaim_AnsweredRequestStaysClaimed(t *testing.T) {
	t.Parallel()
	// recordAnswered leaves no open request for the replay to find.
	_, ticketPath, _ := restartHeadlessClaimed(t, nativerunner.VerdictLive, func(_ *servertest.Harness, f *headlessFake) {
		f.SetState("epic-a-iter-01", agentrunner.StateWorking, "")
	})
	time.Sleep(300 * time.Millisecond) // negative wait: a pause would come within a few polls
	tk, err := schema.ParseTicket(ticketPath)
	if err != nil {
		t.Fatal(err)
	}
	if tk.Status != schema.StatusClaimed {
		t.Errorf("status = %q, want claimed", tk.Status)
	}
}

func TestHeadlessBlockReclaim_DeadAgentUsesDeadRunHandling(t *testing.T) {
	t.Parallel()
	_, ticketPath, _ := restartHeadlessClaimed(t, nativerunner.VerdictDied, func(_ *servertest.Harness, f *headlessFake) {
		f.SetPending("epic-a-iter-01", openRequest)
	})
	waitStatus(t, ticketPath, schema.StatusPaused)
	tk, err := schema.ParseTicket(ticketPath)
	if err != nil {
		t.Fatal(err)
	}
	if tk.PauseReason == schema.PauseBlockedPane {
		t.Errorf("dead agent paused as blocked-pane, want the dead-run pause")
	}
	if tk.PausedClass() == schema.PauseClassAnswer {
		t.Errorf("dead agent got an answer-class pause: %q", tk.PauseReason)
	}
}

// A run already waiting for an answer is not paused a second time, open request or not.
func TestHeadlessBlockReclaim_WaitingRunWithOpenRequestIsUntouched(t *testing.T) {
	t.Parallel()
	_, ticketPath, _ := restartHeadlessClaimed(t, nativerunner.VerdictLive, func(h *servertest.Harness, f *headlessFake) {
		f.SetPending("epic-a-iter-01", openRequest)
		markWaiting(t, h, time.Now().Add(-time.Hour))
	})
	time.Sleep(300 * time.Millisecond) // negative wait: a pause would come within a few polls
	tk, err := schema.ParseTicket(ticketPath)
	if err != nil {
		t.Fatal(err)
	}
	if tk.Status != schema.StatusClaimed {
		t.Errorf("status = %q, want claimed", tk.Status)
	}
}
