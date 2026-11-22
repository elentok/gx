package rest_test

import (
	"context"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/servertest/rigs"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/testutil/runnerfake"
)

// Seam (story 36): a ticket queued while a Break is running still launches
// and finishes, and the Break is untouched by the run.
func TestServer_QueueRunsUnderRest(t *testing.T) {
	t.Parallel()
	store := t.TempDir()
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.SetProjectRepo(t, store, "proj", testutil.TempRepo(t))
	fake := rigs.NewNamedFake(runnerfake.NewRunner(), "pty")
	clock := servertest.NewRestClock(start())
	h := servertest.StartWithStore(t, store,
		servertest.WithRunnerFor(fake, func(string, string) agentrunner.Runner { return fake }),
		servertest.WithRestClock(clock), servertest.WithMentalHealth(breakCfg()))

	for range breakCfg().Break.WorkMinutes {
		if _, err := h.Server.Rest().Active(); err != nil {
			t.Fatal(err)
		}
		h.AdvanceRest(clock, time.Minute)
	}
	if v := h.Server.Rest().View(); v.Period == nil {
		t.Fatal("no Break is running")
	}

	if res, err := h.Client.Queue.Add(context.Background(), rigs.QueueRowTicket, "claude"); err != nil || res.Refused {
		t.Fatalf("add: %+v, %v", res, err)
	}
	servertest.WaitFor(t, 10*time.Second, "the ticket launches during the Break", func() bool {
		return len(fake.Prompts(rigs.ResumeLabel)) > 0 && len(rigs.QueueRows(t, h)) == 1
	})

	fake.SetState(rigs.ResumeLabel, agentrunner.StateDone, "")
	servertest.WaitFor(t, 20*time.Second, "the run to finish during the Break", func() bool { return len(rigs.QueueRows(t, h)) == 0 })
	if v := h.Server.Rest().View(); v.Period == nil {
		t.Error("the Break ended when the run finished, want it still running")
	}
}
