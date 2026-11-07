package sched_test

import (
	"context"
	"testing"

	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
)

// A queued ticket that reaches done or cancelled, however it got there, leaves
// the queue at the next claim pass; the other entries stay.
func TestClaimNext_DropsQueuedTicketsThatAreTerminal(t *testing.T) {
	t.Parallel()
	store := t.TempDir()
	// An error-status blocker keeps the claim pass from launching anything.
	servertest.WriteTicketWith(t, store, "proj", "epic-a", "01", "first", servertest.TicketOpts{Status: "bogus"})
	servertest.WriteTicket(t, store, "proj", "epic-a", "02", "second", "01")
	servertest.WriteTicket(t, store, "proj", "epic-a", "03", "third", "01")
	servertest.WriteTicket(t, store, "proj", "epic-a", "04", "fourth", "01")
	servertest.SetProjectRepo(t, store, "proj", testutil.TempRepo(t))
	h := servertest.StartWithStore(t, store)
	ctx := context.Background()
	for _, a := range []string{"proj:epic-a/02", "proj:epic-a/03", "proj:epic-a/04"} {
		if _, err := h.Client.Queue.Add(ctx, a, ""); err != nil {
			t.Fatal(err)
		}
	}
	servertest.WriteTicketWith(t, store, "proj", "epic-a", "02", "second", servertest.TicketOpts{Status: "done"})
	servertest.WriteTicketWith(t, store, "proj", "epic-a", "03", "third", servertest.TicketOpts{Status: "cancelled"})

	waitFor(t, func() bool {
		h.Server.Scheduler().ClaimNext()
		q, err := h.Client.Queue.Items(ctx)
		return err == nil && len(q) == 1 && q[0].Address == "proj:epic-a/04"
	})
}

// Terminal entries across two projects are each dropped while another entry
// of their root stays; the epic entry is never dropped.
func TestClaimNext_DropsTerminalEntriesAcrossProjects(t *testing.T) {
	t.Parallel()
	store := t.TempDir()
	for _, p := range []string{"proj", "other"} {
		servertest.WriteTicketWith(t, store, p, "epic-a", "01", "first", servertest.TicketOpts{Status: "bogus"})
		servertest.WriteTicket(t, store, p, "epic-a", "02", "second", "01")
		servertest.WriteTicket(t, store, p, "epic-a", "03", "third", "01")
		servertest.SetProjectRepo(t, store, p, testutil.TempRepo(t))
	}
	h := servertest.StartWithStore(t, store)
	ctx := context.Background()
	for _, a := range []string{"proj:epic-a/02", "proj:epic-a/03", "other:epic-a/02", "other:epic-a/03"} {
		if _, err := h.Client.Queue.Add(ctx, a, ""); err != nil {
			t.Fatal(err)
		}
	}
	for _, p := range []string{"proj", "other"} {
		servertest.WriteTicketWith(t, store, p, "epic-a", "02", "second", servertest.TicketOpts{Status: "done"})
	}

	waitFor(t, func() bool {
		h.Server.Scheduler().ClaimNext()
		q, err := h.Client.Queue.Items(ctx)
		return err == nil && len(q) == 2 && q[0].Address == "proj:epic-a/03" && q[1].Address == "other:epic-a/03"
	})
}
