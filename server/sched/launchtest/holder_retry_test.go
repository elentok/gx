package launchtest_test

import (
	"context"
	"testing"
	"time"

	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets/schema"
)

// Start over stops a live holder and confirms it gone before resetting the ticket.
func TestHolderReadopt_StartOverStopsTheLiveHolder(t *testing.T) {
	t.Parallel()
	h, _, pty, path := startUnreachable(t)
	servertest.WaitFor(t, 30*time.Second, "the ticket to pause", func() bool {
		tk, err := schema.ParseTicket(path)
		return err == nil && tk.Status == schema.StatusPaused
	})
	servertest.WaitFor(t, 10*time.Second, "the run to leave the registry", func() bool { return len(h.Server.Runs()) == 0 })
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	res, err := h.Client.Admin.Repair(ctx, "start-over", wire.RepairRequest{Address: "proj:epic-a/01", Reason: "stuck"})
	if err != nil || res.Refused {
		t.Fatalf("start-over = %+v, %v; want accepted", res, err)
	}
	if _, ok, err := pty.Find(readoptLabel); err != nil || ok {
		t.Errorf("holder alive = %v, %v after start over; want stopped", ok, err)
	}
}

// Retry of a ticket paused at the re-adopt cap stops the silent holder, confirms
// it gone and relaunches: one live agent is left, never two.
func TestHolderReadopt_RetryStopsTheSilentHolderAndRelaunches(t *testing.T) {
	t.Parallel()
	h, _, pty, path := startUnreachable(t)
	servertest.WaitFor(t, 30*time.Second, "the ticket to pause", func() bool {
		tk, err := schema.ParseTicket(path)
		return err == nil && tk.Status == schema.StatusPaused
	})
	first, ok, err := pty.Find(readoptLabel)
	if err != nil || !ok {
		t.Fatalf("holder alive = %v, %v; want true", ok, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	// The pause is written just before the finished run leaves the registry.
	servertest.WaitFor(t, 10*time.Second, "the run to leave the registry", func() bool { return len(h.Server.Runs()) == 0 })
	if res, err := h.Client.Tickets.Retry(ctx, "proj:epic-a/01", false); err != nil || res.Refused {
		t.Fatalf("retry = %+v, %v; want accepted", res, err)
	}
	servertest.WaitFor(t, 10*time.Second, "a new agent to replace the silent holder", func() bool {
		s, ok, err := pty.Find(readoptLabel)
		return err == nil && ok && s.ID != first.ID
	})
}
