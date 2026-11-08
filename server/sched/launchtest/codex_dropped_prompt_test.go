package launchtest_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/events"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/testutil/agentfake/codex"
	"github.com/elentok/gx/tickets/schema"
)

// A Codex launch prompt the agent drops is resent in the session, relaunched
// once, then pauses agent_prompt_stalled, whatever the ticket's type.
func TestLaunch_CodexDroppedPromptResendsRelaunchesOnceThenPauses(t *testing.T) {
	for _, typ := range []string{"implement", "merge"} {
		t.Run(typ, func(t *testing.T) {
			t.Parallel()
			store, repo := t.TempDir(), testutil.TempRepo(t)
			servertest.WriteTicketWith(t, store, "proj", "epic-a", "01", "first", servertest.TicketOpts{Type: typ})
			servertest.SetProjectRepo(t, store, "proj", repo)
			h := servertest.StartWithStore(t, store,
				servertest.WithEnv(
					codex.CodexDropPromptEnv+"=1",
					"PATH="+servertest.FakeCodexDir(t)+string(os.PathListSeparator)+os.Getenv("PATH")),
				servertest.WithLaunchGrace(2*time.Second))
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()
			if res, err := h.Client.Queue.Add(ctx, "proj:epic-a/01", "codex"); err != nil || res.Refused {
				t.Fatalf("add: %+v, %v", res, err)
			}
			ticketPath := filepath.Join(store, "proj", "epic-a", "issues", "01-first.md")
			servertest.WaitStatus(t, ticketPath, schema.StatusPaused)

			tk, err := schema.ParseTicket(ticketPath)
			if err != nil || string(tk.PauseReason) != string(events.AgentPromptStalled) {
				t.Fatalf("pause reason = %q (%v), want agent_prompt_stalled", tk.PauseReason, err)
			}
			if !servertest.Poll(10*time.Second, func() bool { return len(ofType(runLog(t, h), events.Paused)) > 0 }) {
				t.Fatalf("run log = %+v, want a paused event", runLog(t, h))
			}
			logged := runLog(t, h)
			failed := ofType(logged, events.LaunchFailed)
			if len(failed) != 2 {
				t.Fatalf("launch-failed events = %+v, want one per launch (the original and one relaunch)", failed)
			}
			for _, f := range failed {
				if !strings.Contains(f.Reason, "no user record after 2s") {
					t.Errorf("launch-failed reason = %q, want the no-user-record wording", f.Reason)
				}
			}
			if len(ofType(logged, events.PromptNudged)) == 0 {
				t.Errorf("run log = %+v, want prompt-nudged for the resends", logged)
			}
		})
	}
}
