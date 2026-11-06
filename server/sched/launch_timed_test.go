package sched_test

import (
	"context"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/events"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/testutil/runnerfake"
)

func TestLaunchTimed_ADeliveredLaunchLogsOneEventWhoseStepsSumToTheTotal(t *testing.T) {
	t.Parallel()
	spy := &launchSpy{Runner: runnerfake.NewRunner()}
	h := launchOne(t, spy, wire.EventIterationStarted)
	t.Cleanup(func() { spy.SetState("epic-a-iter-01", agentrunner.StateDone, "") })
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

func TestLaunchTimed_ASlowInstallCountsInTheDepsStep(t *testing.T) {
	t.Parallel()
	const wait = 200 * time.Millisecond
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	fake := runnerfake.NewRunner()
	h := servertest.StartWithStore(t, store, servertest.WithRunner(fake), func(c *server.Config) {
		c.InstallDeps = func(ctx context.Context, _, _ string) ([]string, error) {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(wait):
				return nil, nil
			}
		}
	})
	t.Cleanup(func() { fake.SetState("epic-a-iter-01", agentrunner.StateDone, "") })
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

func TestLaunchTimed_ALaunchCutOffByAStopLogsNoTiming(t *testing.T) {
	t.Parallel()
	r := &promptHoldRunner{Runner: runnerfake.NewRunner(), inPrompt: make(chan struct{})}
	h, _, _, _ := stopMidLaunch(t, r.inPrompt, servertest.WithRunner(r))
	if timed := ofType(runLog(t, h), events.LaunchTimed); len(timed) != 0 {
		t.Errorf("launch-timed = %+v, want none", timed)
	}
}
