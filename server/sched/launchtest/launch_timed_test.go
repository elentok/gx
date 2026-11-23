package launchtest_test

import (
	"context"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/events"
	"github.com/elentok/gx/nativerunner"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/testutil/runnerfake"
	"github.com/elentok/gx/tickets/schema"
)

func TestLaunchTimed_ADeliveredLaunchLogsOneEventWhoseStepsSumToTheTotal(t *testing.T) {
	t.Parallel()
	spy := &launchSpy{Runner: runnerfake.NewRunner()}
	h := launchOne(t, spy, wire.EventIterationStarted)
	t.Cleanup(func() { spy.Finish("epic-a-iter-01") })
	timed := ofType(runLog(t, h), events.LaunchTimed)
	if len(timed) != 1 {
		t.Fatalf("launch-timed = %+v, want one", timed)
	}
	ev := timed[0]
	if sum := ev.WorktreeStep + ev.DepsStep + ev.AgentStep + ev.PromptStep; ev.Total <= 0 || sum != ev.Total {
		t.Errorf("total = %v, steps sum to %v", ev.Total, sum)
	}
	if ev.Path != "cold-start" || ev.ClaimedAt.IsZero() {
		t.Errorf("event = %+v, want path cold-start and a claim time", ev)
	}
}

func TestLaunchTimed_TheAgentAndPromptStepsSplitIntoPartsThatSumToThem(t *testing.T) {
	t.Parallel()
	spy := &launchSpy{Runner: runnerfake.NewRunner()}
	h := launchOne(t, spy, wire.EventIterationStarted)
	t.Cleanup(func() { spy.Finish("epic-a-iter-01") })
	timed := ofType(runLog(t, h), events.LaunchTimed)
	if len(timed) != 1 {
		t.Fatalf("launch-timed = %+v, want one", timed)
	}
	ev := timed[0]
	for _, c := range []struct {
		step  string
		total time.Duration
		parts map[string]time.Duration
		names []string
	}{
		{"agent", ev.AgentStep, ev.AgentParts, []string{"spawn", "dial", "ready"}},
		{"prompt", ev.PromptStep, ev.PromptParts, []string{"idle", "echo", "turn"}},
	} {
		var sum time.Duration
		for _, d := range c.parts {
			sum += d
		}
		if len(c.parts) != len(c.names) || sum != c.total {
			t.Errorf("%s parts = %v, want %v summing to %v", c.step, c.parts, c.names, c.total)
		}
		for _, n := range c.names {
			if _, ok := c.parts[n]; !ok {
				t.Errorf("%s parts = %v, missing %q", c.step, c.parts, n)
			}
		}
	}
}

func TestLaunchTimed_ASlowInstallCountsInTheDepsStep(t *testing.T) {
	t.Parallel()
	const wait = 200 * time.Millisecond
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	fake := runnerfake.NewRunner()
	h := servertest.StartWithStore(t, store, servertest.WithRunner(fake), func(c *server.Config) {
		c.InstallDeps = func(ctx context.Context, _, _ string, _ []string, _ func()) ([]string, time.Duration, time.Duration, error) {
			select {
			case <-ctx.Done():
				return nil, 0, 0, ctx.Err()
			case <-time.After(wait):
				return nil, 0, 0, nil
			}
		}
	})
	t.Cleanup(func() { fake.Finish("epic-a-iter-01") })
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
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
	servertest.WaitForEvent(ctx, t, evs, wire.EventIterationStarted, "proj:epic-a/01")
	timed := ofType(runLog(t, h), events.LaunchTimed)
	if len(timed) != 1 || timed[0].DepsStep < wait {
		t.Fatalf("launch-timed = %+v, want one with a deps step of at least %v", timed, wait)
	}
}

func TestLaunchTimed_AFailedLaunchLogsNoTiming(t *testing.T) {
	t.Parallel()
	spy, _ := spyWithHolder(t, agentrunner.StateWorking)
	h := launchOne(t, spy, wire.EventIterationLaunchFailed)
	logged := runLog(t, h)
	if len(ofType(logged, events.LaunchFailed)) == 0 || len(ofType(logged, events.LaunchTimed)) != 0 {
		t.Errorf("run log = %+v, want launch-failed and no launch-timed", logged)
	}
}

// A launch the server died in is reclaimed untimed; the launch that follows is a
// new one with its own timing.
func TestLaunchTimed_TheReclaimOfACutOffLaunchGetsItsOwnTiming(t *testing.T) {
	t.Parallel()
	fake := servertest.NewHeadlessFake(nativerunner.VerdictDied)
	h, ticketPath := restartLaunching(t, fake, noWork)
	waitStatus(t, ticketPath, schema.StatusOpen)
	if timed := ofType(runLog(t, h), events.LaunchTimed); len(timed) != 0 {
		t.Fatalf("launch-timed = %+v after the reclaim, want none", timed)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if res, err := h.Client.Queue.Add(ctx, "proj:epic-a/01", "claude"); err != nil || res.Refused {
		t.Fatalf("add: %+v, %v", res, err)
	}
	if !servertest.Poll(30*time.Second, func() bool { return len(ofType(runLog(t, h), events.LaunchTimed)) == 1 }) {
		t.Fatalf("launch-timed = %+v, want one for the new launch", ofType(runLog(t, h), events.LaunchTimed))
	}
}

func TestLaunchTimed_ALaunchCutOffByAStopLogsNoTiming(t *testing.T) {
	t.Parallel()
	r := &promptHoldRunner{Runner: runnerfake.NewRunner(), inPrompt: make(chan struct{})}
	h, _, _, _ := stopMidLaunch(t, r.inPrompt, servertest.WithRunner(r))
	if timed := ofType(runLog(t, h), events.LaunchTimed); len(timed) != 0 {
		t.Errorf("launch-timed = %+v, want none", timed)
	}
}
