package server_test

import (
	"context"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/events"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/recovery"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/tickets"
)

func startDeadlock(t *testing.T, cat recovery.Catalog) *servertest.Harness {
	t.Helper()
	store := t.TempDir()
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.WriteTicket(t, store, "proj", "epic-a", "02", "second", "01")
	servertest.SetProjectRepo(t, store, "proj", testutil.TempRepo(t))
	return servertest.StartWithStore(t, store, func(c *server.Config) {
		c.Orchestrator = config.OrchestratorServer
		c.Recovery = cat
	})
}

// runLogOf is the run log's epic-level events of typ. The claim loop may
// launch and park 01 meanwhile; its own events are not the epic's.
func runLogOf(t *testing.T, h *servertest.Harness, typ events.Type) []ralphloop.Event {
	t.Helper()
	log, _, err := ralphloop.ReadEvents(filepath.Join(h.TicketStore, "proj"), "epic-a")
	if err != nil {
		t.Fatal(err)
	}
	var got []ralphloop.Event
	for _, ev := range log {
		if events.Type(ev.Type) == typ && ev.Ticket == "" {
			got = append(got, ev)
		}
	}
	return got
}

// An epic whose only unblocked ticket is parked logs deadlocked once, however
// many claim passes see it.
func TestDeadlock_AllParkedEpicLogsDeadlockedOncePerEntry(t *testing.T) {
	h := startDeadlock(t, recovery.Catalog{})
	if err := h.Server.ParkAs("proj:epic-a/01", events.IterationError, "broken"); err != nil {
		t.Fatalf("park: %v", err)
	}
	if _, err := h.Client.QueueAdd(context.Background(), "proj:epic-a/02", ""); err != nil {
		t.Fatal(err)
	}
	for range 3 {
		h.Server.ClaimNext()
	}
	waitFor(t, func() bool { return len(runLogOf(t, h, events.Deadlocked)) > 0 })
	h.Server.ClaimNext()
	got := runLogOf(t, h, events.Deadlocked)
	if len(got) != 1 || got[0].Kind != string(events.AllParked) {
		t.Errorf("deadlocked events = %+v, want one all-parked", got)
	}
}

// A deadlock has no ticket to act on, so recovery forks a top-level
// investigation of the epic and queues it at the front.
func TestDeadlock_RecoveryInvestigatesTheEpic(t *testing.T) {
	h := startDeadlock(t, recovery.Catalog{Enabled: true})
	h.Server.RecoverAsync(recovery.Failure{Address: "proj:epic-a", Type: events.Deadlocked, Kind: events.AllParked, Reason: "every remaining ticket is parked"})
	waitFor(t, func() bool { return len(runLogOf(t, h, events.RecoveryApplied)) > 0 })

	applied := runLogOf(t, h, events.RecoveryApplied)
	if len(applied) != 1 || applied[0].Reason != "investigate" || applied[0].Outcome != "proj:epic-a/03" {
		t.Errorf("recovery-applied = %+v, want one investigate forking 03", applied)
	}
	var child tickets.Ticket
	for _, tk := range epicTickets(t, h).Tickets {
		if tk.Identifier == "03" {
			child = tk
		}
	}
	if child.Type != "investigate" || child.Parent != nil {
		t.Errorf("03 = %+v, want a top-level investigate ticket", child)
	}
	if q := queueAddresses(t, h); !slices.Contains(q, "proj:epic-a/03") {
		t.Errorf("queue = %v, want 03 queued", q)
	}

	// A second deadlock of the same kind hits the per-kind cap.
	h.Server.RecoverAsync(recovery.Failure{Address: "proj:epic-a", Type: events.Deadlocked, Kind: events.AllParked, Reason: "again"})
	waitFor(t, func() bool { return len(runLogOf(t, h, events.RecoveryEscalated)) > 0 })
	time.Sleep(100 * time.Millisecond)
	if got := runLogOf(t, h, events.RecoveryApplied); len(got) != 1 {
		t.Errorf("recovery-applied = %+v, want only the first", got)
	}
}
