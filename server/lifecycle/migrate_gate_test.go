package lifecycle_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/elentok/gx/events"
	"github.com/elentok/gx/recovery"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
)

// Seam 1 and 2: while migrating, a queued ready ticket is not claimed and a
// pending recovery does not start, yet reads and a queue add still work; after
// the release both run without another nudge.
func TestMigrate_SchedulerHoldsUntilTheRunEnds(t *testing.T) {
	t.Parallel()
	store := t.TempDir()
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.WriteTicket(t, store, "proj", "epic-a", "02", "second", "01")
	servertest.SetProjectRepo(t, store, "proj", testutil.TempRepo(t))
	opt, entered, release := servertest.HeldMigration(t)
	h := servertest.StartWithStore(t, store, opt, func(c *server.Config) {
		c.Recovery = recovery.Catalog{Enabled: true}
	})
	<-entered
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if _, err := h.Client.Snapshot(ctx); err != nil {
		t.Fatalf("read while held: %v", err)
	}
	// The add's ticket stamp waits on the project lock, so the call returns only
	// after the release; the queue entry itself is written before that.
	added := make(chan error, 1)
	go func() {
		res, err := h.Client.Queue.Add(ctx, "proj:epic-a/01", "")
		if err == nil && res.Refused {
			err = errors.New("refused: " + res.Message)
		}
		added <- err
	}()
	waitFor(t, func() bool { return slices.Contains(servertest.QueueAddresses(t, h), "proj:epic-a/01") })
	h.Server.Scheduler().ClaimNext()
	h.Server.Recovery().RecoverAsync(recovery.Failure{Address: "proj:epic-a", Type: events.Deadlocked, Kind: events.AllPaused, Reason: "every remaining ticket is paused"})
	time.Sleep(300 * time.Millisecond) // negative wait: a gate that leaks would have acted by now
	if n := len(h.Starts()); n != 0 {
		t.Errorf("agent starts while held = %d, want 0", n)
	}
	if got := runLogOf(t, h, events.RecoveryApplied); len(got) != 0 {
		t.Errorf("recovery applied while held: %+v", got)
	}

	release()
	if err := <-added; err != nil {
		t.Errorf("manual queue add: %v", err)
	}
	waitFor(t, func() bool { return len(runLogOf(t, h, events.RecoveryApplied)) > 0 })
	waitFor(t, func() bool { return len(h.Starts()) > 0 })
}
