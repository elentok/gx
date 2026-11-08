package launchtest_test

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/events"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/testutil/agentfake"
	"github.com/elentok/gx/tickets/schema"
)

// hungLaunch queues proj:epic-a/01 on the real PTY runner over the fake claude
// whose first hangs launches stop taking keys.
func hungLaunch(t *testing.T, hangs int) (*servertest.Harness, string, <-chan wire.Event) {
	t.Helper()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	counter := filepath.Join(t.TempDir(), "launches")
	h := servertest.StartWithStore(t, store,
		servertest.WithEnv(fmt.Sprintf("%s=%d:%s", agentfake.TUIHangLaunchesEnv, hangs, counter)),
		servertest.WithLaunchGrace(3*time.Second))
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)
	snap, err := h.Client.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	evs, err := h.Client.Events(ctx, snap.Seq)
	if err != nil {
		t.Fatal(err)
	}
	if res, err := h.Client.Queue.Add(ctx, "proj:epic-a/01", "claude"); err != nil || res.Refused {
		t.Fatalf("add: %+v, %v", res, err)
	}
	return h, filepath.Join(store, "proj", "epic-a", "issues", "01-first.md"), evs
}

func TestLaunch_HungPaneOnceIsStoppedAndRelaunchedOnce(t *testing.T) {
	t.Parallel()
	h, _, evs := hungLaunch(t, 1)
	// The first attempt's start is announced too; the launch is done once the
	// hung attempt is logged and one agent is live.
	servertest.WaitForEvent(context.Background(), t, evs, wire.EventIterationStarted, "proj:epic-a/01")
	if !servertest.Poll(30*time.Second, func() bool { return len(ofType(runLog(t, h), events.LaunchFailed)) > 0 && len(h.Server.Runs()) == 1 }) {
		t.Fatalf("run log = %+v, want the hung attempt logged and one live agent", runLog(t, h))
	}

	logged := runLog(t, h)
	failed := ofType(logged, events.LaunchFailed)
	if len(failed) != 1 || failed[0].Kind != string(events.AgentPromptStalled) || failed[0].Attempt != 1 {
		t.Fatalf("launch-failed events = %+v, want one agent_prompt_stalled for attempt 1", failed)
	}
	if !strings.Contains(failed[0].Reason, "pane hung") || !strings.Contains(failed[0].Reason, "gx-implement") {
		t.Errorf("launch-failed reason = %q, want the hung-pane error with the screen lines", failed[0].Reason)
	}
	if runs := h.Server.Runs(); len(runs) != 1 {
		t.Errorf("runs = %+v, want one live agent", runs)
	}
}

func TestLaunch_HungPaneTwicePausesPromptStalledWithTheTwiceWording(t *testing.T) {
	t.Parallel()
	h, ticketPath, _ := hungLaunch(t, 2)
	servertest.WaitStatus(t, ticketPath, schema.StatusPaused)

	tk, err := schema.ParseTicket(ticketPath)
	if err != nil || string(tk.PauseReason) != string(events.AgentPromptStalled) {
		t.Fatalf("pause reason = %q (%v), want agent_prompt_stalled", tk.PauseReason, err)
	}
	// The pause is written to the ticket before its run-log event.
	if !servertest.Poll(10*time.Second, func() bool { return len(ofType(runLog(t, h), events.Paused)) > 0 }) {
		t.Fatalf("run log = %+v, want a paused event", runLog(t, h))
	}
	logged := runLog(t, h)
	if n := len(ofType(logged, events.LaunchFailed)); n != 2 {
		t.Errorf("launch-failed events = %d, want 2 (one per hung launch)", n)
	}
	paused := ofType(logged, events.Paused)
	if len(paused) != 1 || !strings.Contains(paused[0].Reason, "pane stopped taking keys, twice") || !strings.Contains(paused[0].Reason, "gx-implement") {
		t.Errorf("paused events = %+v, want the twice wording and the screen lines", paused)
	}
	if runs := h.Server.Runs(); len(runs) != 0 {
		t.Errorf("runs = %+v, want no live agent", runs)
	}
}
