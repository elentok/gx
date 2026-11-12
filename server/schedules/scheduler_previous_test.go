package schedules_test

import (
	"context"
	"testing"
	"time"

	"github.com/elentok/gx/server/schedules"
	"github.com/elentok/gx/server/servertest"
)

const prevAddr = "proj:one-offs/90"

// writePrevious writes this machine's previous run of the rig's Schedule.
func (r *rig) writePrevious(t *testing.T, opts servertest.TicketOpts) {
	t.Helper()
	opts.Unique = "schedule:" + r.key.String() + "@" + r.h.Server.Schedules().MachineID()
	opts.Type = "prompt"
	servertest.WriteTicketWith(t, r.h.TicketStore, "proj", "one-offs", "90", "previous", opts)
}

func (r *rig) waitSkips(t *testing.T, n int) []schedules.Skip {
	t.Helper()
	var got []schedules.Skip
	servertest.WaitFor(t, 10*time.Second, "skip log", func() bool {
		got = r.h.Server.Schedules().Skips(r.key)
		return len(got) == n
	})
	return got
}

func (r *rig) liveWithKey(t *testing.T) int {
	t.Helper()
	n := 0
	for _, run := range r.runs(t) {
		if run.Unique != "" && run.Status != "cancelled" && run.Status != "done" {
			n++
		}
	}
	return n
}

func TestSchedulerPrevious_QueuedNeverStartedRunIsReplaced(t *testing.T) {
	t.Parallel()
	r := start(t, 2, "09:00")
	r.enable(t)
	r.writePrevious(t, servertest.TicketOpts{})
	r.clock.Set(at(9, 0).Add(30 * time.Second))

	// The one-offs sweep archives the cancelled run, so only the new one stays.
	servertest.WaitFor(t, 10*time.Second, "old run cancelled and new one queued", func() bool {
		runs := r.runs(t)
		return len(runs) == 1 && runs[0].Status == "open" && runs[0].DisplayNumber() != "90"
	})
	if n := r.liveWithKey(t); n != 1 {
		t.Errorf("live tickets with the key = %d, want 1", n)
	}
	if n := len(r.h.Server.Schedules().Skips(r.key)); n != 0 {
		t.Errorf("skips = %d, want 0", n)
	}
}

func TestSchedulerPrevious_RunningPreviousRunSkips(t *testing.T) {
	t.Parallel()
	r := start(t, 2, "09:00")
	r.enable(t)
	r.writePrevious(t, servertest.TicketOpts{Status: "claimed", ClaimedAt: at(8, 30).Format(time.RFC3339)})
	r.clock.Set(at(9, 0).Add(30 * time.Second))

	sk := r.waitSkips(t, 1)[0]
	settle()
	if sk.Reason != schedules.ReasonPreviousRunStarted {
		t.Errorf("reason = %q", sk.Reason)
	}
	if !sk.SetTime.Equal(at(9, 0)) {
		t.Errorf("set time = %v", sk.SetTime)
	}
	if got := r.waitRuns(t, 1); got[0].Status != "claimed" {
		t.Errorf("a new run was queued: %+v", got)
	}
	r.waitSkips(t, 1)
}

func TestSchedulerPrevious_BlockingRunSkipsUntilCancelled(t *testing.T) {
	cases := map[string]servertest.TicketOpts{
		"paused":       {PauseReason: "question"},
		"needs-repair": {Status: "needs-repair"},
	}
	for name, opts := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			r := start(t, 2, "09:00", "10:00", "11:00")
			r.enable(t)
			r.writePrevious(t, opts)

			r.clock.Set(at(9, 0).Add(30 * time.Second))
			sk := r.waitSkips(t, 1)[0]
			if sk.Reason != schedules.ReasonPreviousRunNeedsAttention || sk.Blocking != prevAddr {
				t.Errorf("skip = %+v", sk)
			}
			r.clock.Set(at(10, 0).Add(30 * time.Second))
			r.waitSkips(t, 2)
			r.waitRuns(t, 1)

			if res, err := r.h.Client.Tickets.Cancel(context.Background(), prevAddr, true); err != nil || res.Refused {
				t.Fatalf("cancel = %+v, %v", res, err)
			}
			r.clock.Set(at(11, 0).Add(30 * time.Second))
			servertest.WaitFor(t, 10*time.Second, "a run is queued", func() bool {
				for _, run := range r.runs(t) {
					if run.Status == "open" {
						return true
					}
				}
				return false
			})
			if n := len(r.h.Server.Schedules().Skips(r.key)); n != 2 {
				t.Errorf("skips = %d, want 2", n)
			}
		})
	}
}
