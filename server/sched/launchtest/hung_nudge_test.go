package launchtest_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner/claudeui"
	"github.com/elentok/gx/events"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/tickets/schema"
)

// nudgeHang queues proj:epic-a/01 with a short stall delay and makes the first
// hangs stall nudges fail as a hung pane.
func nudgeHang(t *testing.T, hangs int) (*servertest.Harness, string) {
	t.Helper()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	h := servertest.StartWithStore(t, store, servertest.WithLaunchGrace(3*time.Second),
		func(c *server.Config) { c.StalledNudgeDelay = 2 * time.Second })
	h.FailNextPrompts(hangs, &claudeui.HungPaneError{Screen: "> $gx-implement proj:epic-a/01"})
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)
	if res, err := h.Client.Queue.Add(ctx, "proj:epic-a/01", "claude"); err != nil || res.Refused {
		t.Fatalf("add: %+v, %v", res, err)
	}
	return h, store + "/proj/epic-a/issues/01-first.md"
}

func TestStallNudge_HungPaneOnceIsRelaunchedOnce(t *testing.T) {
	t.Parallel()
	h, _ := nudgeHang(t, 1)
	if !servertest.Poll(30*time.Second, func() bool { return len(ofType(runLog(t, h), events.IterationStarted)) == 2 }) {
		t.Fatalf("run log = %+v, want two starts (the relaunch)", runLog(t, h))
	}
	if n := len(ofType(runLog(t, h), events.Paused)); n != 0 {
		t.Errorf("paused events = %d, want none", n)
	}
}

func TestStallNudge_HungPaneTwicePausesPromptStalled(t *testing.T) {
	t.Parallel()
	h, ticketPath := nudgeHang(t, 2)
	servertest.WaitStatus(t, ticketPath, schema.StatusPaused)
	tk, err := schema.ParseTicket(ticketPath)
	if err != nil || string(tk.PauseReason) != string(events.AgentPromptStalled) {
		t.Fatalf("pause reason = %q (%v), want agent_prompt_stalled", tk.PauseReason, err)
	}
	if !servertest.Poll(10*time.Second, func() bool { return len(ofType(runLog(t, h), events.Paused)) > 0 }) {
		t.Fatalf("run log = %+v, want a paused event", runLog(t, h))
	}
	paused := ofType(runLog(t, h), events.Paused)
	if len(paused) != 1 || !strings.Contains(paused[0].Reason, "pane stopped taking keys, twice") {
		t.Errorf("paused events = %+v, want the twice wording", paused)
	}
	if n := len(ofType(runLog(t, h), events.IterationStarted)); n != 2 {
		t.Errorf("starts = %d, want 2 (one relaunch)", n)
	}
	// The settle path drops the run after it writes the pause.
	if !servertest.Poll(10*time.Second, func() bool { return len(h.Server.Runs()) == 0 }) {
		t.Errorf("runs = %+v, want no live agent", h.Server.Runs())
	}
}
