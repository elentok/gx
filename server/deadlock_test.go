package server_test

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

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
		c.Recovery = cat
	})
}

// runLogOf is the run log's epic-level events of typ. The claim loop may
// launch and pause 01 meanwhile; its own events are not the epic's.
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

// An epic whose only unblocked ticket is paused logs deadlocked once, however
// many claim passes see it.
func TestDeadlock_AllPausedEpicLogsDeadlockedOncePerEntry(t *testing.T) {
	t.Parallel()
	h := startDeadlock(t, recovery.Catalog{})
	if err := h.Server.PauseAs("proj:epic-a/01", events.IterationError, "broken"); err != nil {
		t.Fatalf("pause: %v", err)
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
	if len(got) != 1 || got[0].Kind != string(events.AllPaused) {
		t.Errorf("deadlocked events = %+v, want one all-paused", got)
	}
}

// An epic holding an error ticket is Paused: a person can fix it, so it is not
// a dependency deadlock.
func TestDeadlock_ErrorTicketPausesTheEpic(t *testing.T) {
	t.Parallel()
	store := t.TempDir()
	servertest.WriteTicketWith(t, store, "proj", "epic-a", "01", "first", servertest.TicketOpts{Status: "bogus"})
	servertest.WriteTicket(t, store, "proj", "epic-a", "02", "second", "01")
	servertest.SetProjectRepo(t, store, "proj", testutil.TempRepo(t))
	h := servertest.StartWithStore(t, store)
	if _, err := h.Client.QueueAdd(context.Background(), "proj:epic-a/02", ""); err != nil {
		t.Fatal(err)
	}
	h.Server.ClaimNext()
	waitFor(t, func() bool { return len(runLogOf(t, h, events.Deadlocked)) > 0 })
	got := runLogOf(t, h, events.Deadlocked)
	if len(got) != 1 || got[0].Kind != string(events.AllPaused) {
		t.Errorf("deadlocked events = %+v, want one all-paused", got)
	}
}

// A blocked (or waiting) ticket next to a paused one makes the epic Paused, not
// a blocked cycle, whichever of the two is listed first.
func TestDeadlock_BlockedOrWaitingNextToPausedIsAllPaused(t *testing.T) {
	t.Parallel()
	type tk struct {
		id   string
		opts servertest.TicketOpts
	}
	pauses := map[string]servertest.TicketOpts{
		"paused-answer": {PauseReason: "question"},
		"error":         {Status: "error"},
		"draft":         {Status: "draft"},
	}
	done := servertest.TicketOpts{Status: "done"}
	cases := map[string][]tk{}
	for name, p := range pauses {
		cases["blocked first, "+name] = []tk{{"01", servertest.TicketOpts{BlockedBy: []string{"02"}}}, {"02", p}}
		cases["paused first, "+name] = []tk{{"01", p}, {"02", servertest.TicketOpts{BlockedBy: []string{"01"}}}}
		// A done ticket whose fork child is paused renders as waiting for children.
		child1, child2 := p, p
		child1.Parent, child2.Parent = "01", "02"
		cases["waiting first, "+name] = []tk{{"01", done}, {"01a", child1}}
		cases["paused first, waiting, "+name] = []tk{{"01", p}, {"02", done}, {"02a", child2}}
	}
	for name, tickets := range cases {
		t.Run(name, func(t *testing.T) {
			store := t.TempDir()
			for _, tk := range tickets {
				servertest.WriteTicketWith(t, store, "proj", "epic-a", tk.id, "ticket-"+tk.id, tk.opts)
			}
			servertest.SetProjectRepo(t, store, "proj", testutil.TempRepo(t))
			h := servertest.StartWithStore(t, store)
			for _, tk := range tickets {
				if tk.opts.Status != "done" {
					if _, err := h.Client.QueueAdd(context.Background(), "proj:epic-a/"+tk.id, ""); err != nil {
						t.Fatal(err)
					}
					break
				}
			}
			h.Server.ClaimNext()
			waitFor(t, func() bool { return len(runLogOf(t, h, events.Deadlocked)) > 0 })
			got := runLogOf(t, h, events.Deadlocked)
			if len(got) != 1 || got[0].Kind != string(events.AllPaused) {
				t.Errorf("deadlocked events = %+v, want one all-paused", got)
			}
		})
	}
}

// A map epic holds notes, not work, so it is never Deadlocked.
func TestDeadlock_MapEpicIsNeverDeadlocked(t *testing.T) {
	t.Parallel()
	store := t.TempDir()
	servertest.WriteTicketWith(t, store, "proj", "epic-a", "01", "first", servertest.TicketOpts{Status: "bogus"})
	servertest.WriteTicket(t, store, "proj", "epic-a", "02", "second", "01")
	servertest.SetProjectRepo(t, store, "proj", testutil.TempRepo(t))
	if err := os.WriteFile(filepath.Join(store, "proj", "epic-a", "ticket.md"), []byte("---\nkind: map\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	h := servertest.StartWithStore(t, store)
	_, _ = h.Client.QueueAdd(context.Background(), "proj:epic-a/02", "")
	for range 3 {
		h.Server.ClaimNext()
	}
	time.Sleep(200 * time.Millisecond)
	if got := runLogOf(t, h, events.Deadlocked); len(got) != 0 {
		t.Errorf("deadlocked events = %+v, want none for a map epic", got)
	}
}

// A blocked deadlock's record names the tickets that wait and what they wait on.
func TestDeadlock_BlockedRecordNamesTheBlockers(t *testing.T) {
	t.Parallel()
	store := t.TempDir()
	servertest.WriteTicketWith(t, store, "proj", "epic-a", "01", "first", servertest.TicketOpts{Status: "cancelled"})
	servertest.WriteTicketWith(t, store, "proj", "epic-a", "02", "second", servertest.TicketOpts{Parent: "01"})
	servertest.SetProjectRepo(t, store, "proj", testutil.TempRepo(t))
	h := servertest.StartWithStore(t, store)
	_, _ = h.Client.QueueAdd(context.Background(), "proj:epic-a/02", "")
	h.Server.ClaimNext()
	waitFor(t, func() bool { return len(runLogOf(t, h, events.Deadlocked)) > 0 })
	got := runLogOf(t, h, events.Deadlocked)
	want := "proj:epic-a/02 waits on proj:epic-a/01"
	if got[0].Kind != string(events.BlockedCycle) || got[0].Reason != want {
		t.Errorf("deadlocked = %+v, want blocked-cycle %q", got[0], want)
	}
}

// A deadlock has no ticket to act on, so recovery forks a top-level
// investigation of the epic and queues it at the front.
func TestDeadlock_RecoveryInvestigatesTheEpic(t *testing.T) {
	t.Parallel()
	h := startDeadlock(t, recovery.Catalog{Enabled: true})
	h.Server.RecoverAsync(recovery.Failure{Address: "proj:epic-a", Type: events.Deadlocked, Kind: events.AllPaused, Reason: "every remaining ticket is paused"})
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
	h.Server.RecoverAsync(recovery.Failure{Address: "proj:epic-a", Type: events.Deadlocked, Kind: events.AllPaused, Reason: "again"})
	waitFor(t, func() bool { return len(runLogOf(t, h, events.RecoveryEscalated)) > 0 })
	time.Sleep(100 * time.Millisecond)
	if got := runLogOf(t, h, events.RecoveryApplied); len(got) != 1 {
		t.Errorf("recovery-applied = %+v, want only the first", got)
	}
}
