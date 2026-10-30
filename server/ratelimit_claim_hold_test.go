package server_test

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/nativerunner"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/testutil/runnerfake"
)

// While a Claude run holds for its rate limit, no new Claude ticket claims and
// its verdict names the hold; a Codex ticket still claims; when the limit
// lifts the waiting ticket claims.
func TestRateLimitHold_StopsNewClaimsOfThatHarnessOnly(t *testing.T) {
	t.Parallel()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	for _, epic := range []string{"epic-a", "epic-b", "epic-c"} {
		servertest.WriteTicket(t, store, "proj", epic, "01", epic, "")
	}
	servertest.SetProjectRepo(t, store, "proj", repo)
	pty := namedFake{runnerfake.NewRunner(), "pty"}
	h := servertest.StartWithStore(t, store, servertest.WithRunnerFor(pty, func(string, string) agentrunner.Runner { return pty }),
		func(c *server.Config) { c.MaxAgents = 4 })
	t.Cleanup(func() { pty.SetState("epic-a-iter-01", agentrunner.StateDone, "") })
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if res, err := h.Client.QueueAdd(ctx, "proj:epic-a/01", "claude"); err != nil || res.Refused {
		t.Fatalf("add first: %+v, %v", res, err)
	}
	servertest.WaitFor(t, 10*time.Second, "first ticket launches", func() bool { return len(pty.Prompts("epic-a-iter-01")) > 0 })
	pty.SetLimitedUnknownReset("epic-a-iter-01")
	pty.SetState("epic-a-iter-01", agentrunner.StateDone, "")
	servertest.WaitFor(t, 10*time.Second, "the hold shows", func() bool {
		ex, err := h.Client.Explain(ctx, "proj:epic-a/01")
		return err == nil && ex.Verdict == server.VerdictRateLimited
	})

	if res, err := h.Client.QueueAdd(ctx, "proj:epic-b/01", "claude"); err != nil || res.Refused {
		t.Fatalf("add claude: %+v, %v", res, err)
	}
	if res, err := h.Client.QueueAdd(ctx, "proj:epic-c/01", "codex"); err != nil || res.Refused {
		t.Fatalf("add codex: %+v, %v", res, err)
	}
	servertest.WaitFor(t, 10*time.Second, "the codex ticket claims", func() bool { return len(pty.Prompts("epic-c-iter-01")) > 0 })
	servertest.WaitFor(t, 10*time.Second, "the claude ticket explains the hold", func() bool {
		ex, err := h.Client.Explain(ctx, "proj:epic-b/01")
		return err == nil && ex.Verdict == server.VerdictRateLimited
	})
	if n := len(pty.Prompts("epic-b-iter-01")); n != 0 {
		t.Errorf("claude ticket got %d prompts during the hold", n)
	}

	pty.SetRateLimit("epic-a-iter-01", time.Time{})
	// The hold's "continue" prompt puts the agent back to work; its turn then ends.
	servertest.WaitFor(t, 10*time.Second, "the hold re-prompts the agent", func() bool {
		return slices.Contains(pty.Prompts("epic-a-iter-01"), "continue")
	})
	pty.SetState("epic-a-iter-01", agentrunner.StateDone, "")
	servertest.WaitFor(t, 20*time.Second, "the claude ticket claims once the hold ends", func() bool {
		return len(pty.Prompts("epic-b-iter-01")) > 0
	})
}

// A restart during a hold re-establishes the hold before the first claim pass,
// so no new ticket of that harness claims.
func TestRateLimitHold_RestartHoldsBeforeTheFirstClaim(t *testing.T) {
	t.Parallel()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.WriteTicket(t, store, "proj", "epic-b", "01", "second", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	fake := &headlessFake{Runner: runnerfake.NewRunner(), verdict: nativerunner.VerdictLive}
	session, err := fake.Start(agentrunner.StartOptions{Label: "epic-a-iter-01"})
	if err != nil {
		t.Fatal(err)
	}
	fake.SetLimitedUnknownReset("epic-a-iter-01")
	// Working keeps the wait loop from reading the limit itself, so only the
	// check reclaim runs can set the hold.
	fake.SetState("epic-a-iter-01", agentrunner.StateWorking, "")
	h := servertest.StartWithStore(t, store, servertest.WithRunner(fake))
	leaveIteration(t, h, store, repo, "herdr")
	stampRun(t, h, "fake", session)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	// Queued while paused, so the ticket waits across the restart and is
	// claimable the moment the queue resumes on the new server.
	if res, err := h.Client.QueuePause(ctx); err != nil || res.Refused {
		t.Fatalf("pause: %+v, %v", res, err)
	}
	if res, err := h.Client.QueueAdd(ctx, "proj:epic-b/01", "claude"); err != nil || res.Refused {
		t.Fatalf("add second: %+v, %v", res, err)
	}
	h.Restart(t)
	if res, err := h.Client.QueueResume(ctx); err != nil || res.Refused {
		t.Fatalf("resume: %+v, %v", res, err)
	}
	servertest.WaitFor(t, 10*time.Second, "explain shows the hold", func() bool {
		ex, err := h.Client.Explain(ctx, "proj:epic-b/01")
		return err == nil && ex.Verdict == server.VerdictRateLimited
	})
	time.Sleep(200 * time.Millisecond) // negative wait: a claim would show within a few ticks
	if n := len(fake.Prompts("epic-b-iter-01")); n != 0 {
		t.Errorf("second ticket got %d prompts during the hold", n)
	}
	if ex, err := h.Client.Explain(ctx, "proj:epic-a/01"); err != nil || ex.Verdict != server.VerdictRateLimited {
		t.Errorf("held run explain = %+v, %v; want the hold", ex, err)
	}
}
