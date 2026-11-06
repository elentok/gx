package sched_test

import (
	"context"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/ralphloop/ticketops"
	"github.com/elentok/gx/server/runs"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire"
)

func TestSnapshot_LaunchingTicketCarriesItsStepUntilTheLaunchEnds(t *testing.T) {
	t.Parallel()
	h, path := startTicketWrites(t)
	if err := ticketops.Claim(path); err != nil {
		t.Fatal(err)
	}
	reg := h.Server.Registry()
	reg.PutRunAt("proj:epic-a", runs.Run{Address: "proj:epic-a/01", Runner: "pty", Session: agentrunner.Session{ID: "p1"}}, time.Now())
	reg.SetLaunchStep("proj:epic-a/01", runs.StepStartingAgent)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	stepOf := func() wire.LaunchStep {
		snap, err := h.Client.Snapshot(ctx)
		if err != nil {
			t.Fatal(err)
		}
		for _, tk := range snap.Tickets {
			if tk.Address == "proj:epic-a/01" && tk.Status == "claimed" {
				return tk.LaunchStep
			}
		}
		return wire.LaunchStep{Label: "not claimed yet"}
	}
	want := wire.LaunchStep{Label: "starting agent", Position: 3}
	servertest.WaitFor(t, 5*time.Second, "the snapshot to carry the launch step", func() bool { return stepOf() == want })
	reg.SetLaunchStep("proj:epic-a/01", "")
	if got := stepOf(); got != (wire.LaunchStep{}) {
		t.Fatalf("step after the launch = %+v, want none", got)
	}
}

func TestSnapshot_ClaimedTicketCarriesLaunchTime(t *testing.T) {
	t.Parallel()
	h, path := startTicketWrites(t)
	if err := ticketops.Claim(path); err != nil {
		t.Fatal(err)
	}
	launched := time.Now().Add(-90 * time.Second).Truncate(time.Second)
	h.Server.Registry().PutRunAt("proj:epic-a", runs.Run{Address: "proj:epic-a/01", Runner: "pty", Session: agentrunner.Session{ID: "p1"}}, launched)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var claimed wire.TicketInfo
	servertest.WaitFor(t, 5*time.Second, "the ticket to show as claimed", func() bool {
		snap, err := h.Client.Snapshot(ctx)
		if err != nil {
			t.Fatal(err)
		}
		for _, tk := range snap.Tickets {
			if tk.Address == "proj:epic-a/01" && tk.Status == "claimed" {
				claimed = tk
				return true
			}
		}
		return false
	})
	if !claimed.ClaimedAt.Equal(launched) {
		t.Fatalf("ClaimedAt = %v, want %v", claimed.ClaimedAt, launched)
	}
}
