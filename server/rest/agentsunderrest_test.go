package rest_test

import (
	"context"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/config"
	"github.com/elentok/gx/server/rest"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/servertest/rigs"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/testutil/runnerfake"
)

// Seam (story 36): a running agent keeps working, and one that needs the
// person stays waiting and shows in the snapshot, during a Break and Bedtime.
func TestServer_AgentsKeepWorkingUnderRest(t *testing.T) {
	t.Parallel()
	bedtime := config.DefaultMentalHealthConfig()
	bedtime.Enabled = true
	bedtime.Bedtime = config.MentalHealthBedtime{Enabled: true, Start: "10:05", End: "11:00"}
	cases := []struct {
		name string
		cfg  config.MentalHealthConfig
		kind rest.Kind
	}{
		{"break", breakCfg(), rest.KindBreak},
		{"bedtime", bedtime, rest.KindBedtime},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			store := t.TempDir()
			servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
			servertest.SetProjectRepo(t, store, "proj", testutil.TempRepo(t))
			fake := rigs.NewNamedFake(runnerfake.NewRunner(), "pty")
			clock := servertest.NewRestClock(start())
			h := servertest.StartWithStore(t, store,
				servertest.WithRunnerFor(fake, func(string, string) agentrunner.Runner { return fake }),
				servertest.WithRestClock(clock), servertest.WithMentalHealth(tc.cfg))
			t.Cleanup(func() { fake.Finish(rigs.ResumeLabel) })

			if res, err := h.Client.Queue.Add(context.Background(), rigs.QueueRowTicket, "claude"); err != nil || res.Refused {
				t.Fatalf("add: %+v, %v", res, err)
			}
			servertest.WaitFor(t, 10*time.Second, "the agent launches", func() bool { return len(fake.Prompts(rigs.ResumeLabel)) > 0 })
			fake.SetState(rigs.ResumeLabel, agentrunner.StateWorking, "")

			for i := 0; i < 10 && h.Server.Rest().View().Period == nil; i++ {
				if _, err := h.Server.Rest().Active(); err != nil {
					t.Fatal(err)
				}
				h.AdvanceRest(clock, time.Minute)
			}
			if p := h.Server.Rest().View().Period; p == nil || p.Kind != tc.kind {
				t.Fatalf("period = %+v, want a %v", p, tc.kind)
			}

			rowState := func() string {
				rows := rigs.QueueRows(t, h)
				if len(rows) != 1 {
					return ""
				}
				return rows[0].Status.State
			}
			servertest.WaitFor(t, 10*time.Second, "the agent keeps working under Rest", func() bool { return rowState() == wire.SessionWorking })

			fake.SetState(rigs.ResumeLabel, agentrunner.StateBlocked, "permission")
			servertest.WaitFor(t, 10*time.Second, "the waiting agent shows as blocked", func() bool { return rowState() == wire.SessionBlocked })
			if h.Server.Rest().View().Period == nil {
				t.Error("the Rest period ended, want it still running")
			}
		})
	}
}
