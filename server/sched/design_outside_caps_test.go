package sched_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/nativerunner"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/queue"
	"github.com/elentok/gx/server/servertest"
)

// Seam: with every cap at 1 a live design session holds no slot, so a queued
// ticket still claims; the design is a session row, never a Queue row.
func TestDesignSession_TakesNoSlotAndIsOutsideTheQueueViews(t *testing.T) {
	t.Parallel()
	fake := servertest.NewHeadlessFake(nativerunner.VerdictLive)
	f := servertest.StartDesigns(t, servertest.WithRunner(fake), func(c *server.Config) {
		c.BudgetPollInterval = time.Hour
		c.MaxAgents = 1
		c.MaxAgentsPerRoot = 1
	})
	res := f.Open(t, "proj:map", false)
	if res.Refused || res.Session == nil {
		t.Fatalf("open = %+v", res)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if r, err := f.H.Client.Queue.Add(ctx, "proj:plain/01", "claude"); err != nil || r.Refused {
		t.Fatalf("add = %+v, %v", r, err)
	}
	waitFor(t, func() bool { return len(f.H.Server.Runs()) == 1 })

	snap, err := f.H.Client.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	listed := false
	for _, s := range snap.Sessions {
		if s.ID == res.Session.ID {
			listed = true
		}
		if strings.HasPrefix(s.ID, queue.AgentIDPrefix) && strings.Contains(s.ID, "map") {
			t.Errorf("design appears as a Queue agent row: %s", s.ID)
		}
	}
	if !listed {
		t.Errorf("Agents tab lost the design session %s", res.Session.ID)
	}
	for _, p := range snap.Pending {
		if strings.Contains(p.Address, "map") {
			t.Errorf("design appears in pending rows: %+v", p)
		}
	}
	for _, r := range f.H.Server.Runs() {
		if strings.Contains(r.Address, "map") {
			t.Errorf("design tracked as a run: %+v", r)
		}
	}
}
