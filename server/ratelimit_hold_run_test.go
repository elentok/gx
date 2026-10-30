package server_test

import (
	"context"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/testutil/runnerfake"
)

// A rate-limit-held run keeps its slot and explains as "rate-limited: resets
// <time>" (just "rate-limited" without a reset), in explain and in the pending list.
func TestRateLimitHold_RunKeepsSlotAndExplainsTheHold(t *testing.T) {
	t.Parallel()
	reset := time.Now().Add(time.Hour).UTC().Truncate(time.Second)
	cases := []struct {
		name  string
		limit func(r *runnerfake.Runner)
		want  string
	}{
		{"known reset", func(r *runnerfake.Runner) { r.SetRateLimit("epic-a-iter-01", reset) }, "rate-limited: resets " + reset.Format(time.RFC3339)},
		{"unknown reset", func(r *runnerfake.Runner) { r.SetLimitedUnknownReset("epic-a-iter-01") }, server.VerdictRateLimited},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			store, repo := t.TempDir(), testutil.TempRepo(t)
			servertest.WriteTicket(t, store, "proj", "epic-a", "01", "held", "")
			servertest.WriteTicket(t, store, "proj", "epic-b", "01", "second", "")
			servertest.SetProjectRepo(t, store, "proj", repo)
			pty := namedFake{runnerfake.NewRunner(), "pty"}
			h := servertest.StartWithStore(t, store, servertest.WithRunnerFor(pty, func(string, string) agentrunner.Runner { return pty }),
				func(c *server.Config) { c.MaxAgents = 1 })
			t.Cleanup(func() { pty.SetState("epic-a-iter-01", agentrunner.StateDone, "") })
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			if res, err := h.Client.QueueAdd(ctx, "proj:epic-a/01", "claude"); err != nil || res.Refused {
				t.Fatalf("add first: %+v, %v", res, err)
			}
			servertest.WaitFor(t, 10*time.Second, "first ticket launches", func() bool {
				return len(pty.Prompts("epic-a-iter-01")) > 0
			})
			tc.limit(pty.Runner)
			pty.SetState("epic-a-iter-01", agentrunner.StateDone, "")

			servertest.WaitFor(t, 10*time.Second, "explain shows the hold", func() bool {
				ex, err := h.Client.Explain(ctx, "proj:epic-a/01")
				return err == nil && ex.Verdict == tc.want && ex.Reason == ""
			})
			snap, err := h.Client.Snapshot(ctx)
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, row := range snap.Pending {
				if row.Address == "proj:epic-a/01" && row.Verdict == tc.want {
					found = true
				}
			}
			if !found {
				t.Errorf("pending = %+v, want a row for the held run with verdict %q", snap.Pending, tc.want)
			}

			// The held run still counts against the cap: a second ticket stays queued.
			if res, err := h.Client.QueueAdd(ctx, "proj:epic-b/01", "claude"); err != nil || res.Refused {
				t.Fatalf("add second: %+v, %v", res, err)
			}
			time.Sleep(200 * time.Millisecond) // negative wait: a launch would show within a few ticks
			if n := len(pty.Prompts("epic-b-iter-01")); n != 0 {
				t.Errorf("second ticket got %d prompts while the held run kept the only slot", n)
			}
		})
	}
}
