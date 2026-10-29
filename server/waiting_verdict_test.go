package server_test

import (
	"context"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/nativerunner"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/testutil/runnerfake"
)

// Seam 1: a run waiting for its answer explains as waiting inside the grace
// period, as released once the grace passed, and as held when the release cap
// keeps its slot.
func TestExplain_WaitingRunVerdicts(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name      string
		waitedFor time.Duration
		maxFree   int
		want      string
	}{
		{"within grace", time.Second, 5, server.VerdictWaitingOnAnswer},
		{"past grace", 10 * time.Minute, 5, server.VerdictWaitingReleased},
		{"past grace at the release cap", 10 * time.Minute, 0, server.VerdictWaitingReleaseCapd},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store, repo := t.TempDir(), testutil.TempRepo(t)
			servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
			servertest.SetProjectRepo(t, store, "proj", repo)
			fake := &headlessFake{Runner: runnerfake.NewRunner(), verdict: nativerunner.VerdictLive}
			h := servertest.StartWithStore(t, store, servertest.WithRunner(fake), func(c *server.Config) {
				c.BudgetPollInterval = time.Hour
				c.MaxAgents = 1
				c.WaitingSlotGrace = time.Minute
				c.MaxReleasedAgents = tc.maxFree
			})
			wtPath, _ := leaveIteration(t, h, store, repo)
			if _, err := fake.Start(agentrunner.StartOptions{Label: "epic-a-iter-01", Cwd: wtPath}); err != nil {
				t.Fatal(err)
			}
			asHeadlessRun(t, h)
			markWaiting(t, h, time.Now().Add(-tc.waitedFor))
			h.Restart(t)
			waitFor(t, func() bool { return len(h.Server.Runs()) == 1 })

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			waitFor(t, func() bool {
				ex, err := h.Client.Explain(ctx, "proj:epic-a/01")
				return err == nil && ex.Verdict == tc.want
			})
		})
	}
}
