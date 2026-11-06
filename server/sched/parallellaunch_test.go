package sched_test

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil"
)

// runParallelEpic queues n tickets of one epic under a per-root cap and drives
// the claim loop by hand until the root completes. It reports the most runs
// (running plus launching) the registry held right after a pass.
func runParallelEpic(t *testing.T, n, perRoot int) (peak int) {
	t.Helper()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	for i := 1; i <= n; i++ {
		servertest.WriteTicket(t, store, "proj", "epic-a", fmt.Sprintf("%02d", i), "t", "")
	}
	servertest.SetProjectRepo(t, store, "proj", repo)
	h := servertest.StartWithStore(t, store, func(c *server.Config) {
		c.MaxAgentsPerRoot = perRoot
		c.PollInterval = time.Hour
	})
	h.OnTurn(func(turn *servertest.Turn) {
		testutil.WriteFile(t, turn.Cwd, filepath.Base(turn.Cwd)+".txt", "work")
		testutil.CommitAll(t, turn.Cwd, "agent work")
	})
	ctx := t.Context()
	snap, err := h.Client.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	evs, err := h.Client.Events(ctx, snap.Seq)
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= n; i++ {
		if res, err := h.Client.Queue.Add(ctx, fmt.Sprintf("proj:epic-a/%02d", i), "claude"); err != nil || res.Refused {
			t.Fatalf("add: %+v, %v", res, err)
		}
	}
	sc := h.Server.Scheduler()
	tick := time.NewTicker(20 * time.Millisecond)
	defer tick.Stop()
	deadline := time.After(40 * time.Second)
	for {
		select {
		case ev := <-evs:
			if ev.Type == wire.EventRootCompleted {
				sc.Launcher.WaitLaunches()
				return peak
			}
		case <-tick.C:
			sc.ClaimNext()
			peak = max(peak, h.Server.Registry().Count())
		case <-deadline:
			t.Fatalf("root did not complete; peak %d", peak)
		}
	}
}

func TestClaimNext_ReservesEveryFreeSlotInOnePass(t *testing.T) {
	t.Parallel()
	if peak := runParallelEpic(t, 3, 3); peak != 3 {
		t.Fatalf("peak runs = %d, want all 3 tickets reserved together", peak)
	}
}

func TestClaimNext_PerRootCapHoldsWhileLaunchesRun(t *testing.T) {
	t.Parallel()
	if peak := runParallelEpic(t, 3, 2); peak != 2 {
		t.Fatalf("peak runs = %d, want the cap of 2", peak)
	}
}
