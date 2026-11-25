package sched_test

import (
	"context"
	"github.com/elentok/gx/server/servertest/rigs"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/elentok/gx/events"
	"github.com/elentok/gx/recovery"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/tickets"
)

// An epic whose only unblocked ticket is paused logs deadlocked once, however
// many claim passes see it.
func TestDeadlock_AllPausedEpicLogsDeadlockedOncePerEntry(t *testing.T) {
	t.Parallel()
	h := startDeadlock(t, recovery.Catalog{})
	if err := h.Server.PauseAs("proj:epic-a/01", events.IterationError, "broken"); err != nil {
		t.Fatalf("pause: %v", err)
	}
	if _, err := h.Client.Queue.Add(context.Background(), "proj:epic-a/02", ""); err != nil {
		t.Fatal(err)
	}
	for range 3 {
		h.Server.Scheduler().ClaimNext()
	}
	waitFor(t, func() bool { return len(runLogOf(t, h, events.Deadlocked)) > 0 })
	h.Server.Scheduler().ClaimNext()
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
	if _, err := h.Client.Queue.Add(context.Background(), "proj:epic-a/02", ""); err != nil {
		t.Fatal(err)
	}
	h.Server.Scheduler().ClaimNext()
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
		"error": {Status: "error"},
		"draft": {Status: "draft"},
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
					if _, err := h.Client.Queue.Add(context.Background(), "proj:epic-a/"+tk.id, ""); err != nil {
						t.Fatal(err)
					}
					break
				}
			}
			h.Server.Scheduler().ClaimNext()
			waitFor(t, func() bool { return len(runLogOf(t, h, events.Deadlocked)) > 0 })
			got := runLogOf(t, h, events.Deadlocked)
			if len(got) != 1 || got[0].Kind != string(events.AllPaused) {
				t.Errorf("deadlocked events = %+v, want one all-paused", got)
			}
		})
	}
}

// An epic whose every pause waits on an answer is a normal wait: it logs
// waiting-for-answer, never deadlocked, and starts no recovery. An answer pause
// next to a repair pause is still a deadlock.
func TestDeadlock_AnswerOnlyPausesLogWaitingForAnswer(t *testing.T) {
	t.Parallel()
	answer := servertest.TicketOpts{PauseReason: "self-reported"}
	cases := map[string]struct {
		second     servertest.TicketOpts
		wantWait   bool
		wantLocked bool
	}{
		"answer only":        {second: answer, wantWait: true},
		"answer plus repair": {second: servertest.TicketOpts{Status: "error"}, wantLocked: true},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			store := t.TempDir()
			servertest.WriteTicketWith(t, store, "proj", "epic-a", "01", "first", answer)
			servertest.WriteTicketWith(t, store, "proj", "epic-a", "02", "second", c.second)
			servertest.SetProjectRepo(t, store, "proj", testutil.TempRepo(t))
			h := servertest.StartWithStore(t, store)
			if _, err := h.Client.Queue.Add(context.Background(), "proj:epic-a/01", ""); err != nil {
				t.Fatal(err)
			}
			h.Server.Scheduler().ClaimNext()
			waitFor(t, func() bool {
				return len(runLogOf(t, h, events.WaitingForAnswer))+len(runLogOf(t, h, events.Deadlocked)) > 0
			})
			if got := len(runLogOf(t, h, events.WaitingForAnswer)) > 0; got != c.wantWait {
				t.Errorf("waiting-for-answer logged = %v, want %v", got, c.wantWait)
			}
			if got := len(runLogOf(t, h, events.Deadlocked)) > 0; got != c.wantLocked {
				t.Errorf("deadlocked logged = %v, want %v", got, c.wantLocked)
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
	_, _ = h.Client.Queue.Add(context.Background(), "proj:epic-a/02", "")
	for range 3 {
		h.Server.Scheduler().ClaimNext()
	}
	time.Sleep(200 * time.Millisecond) // negative wait: a deadlock event would show by now
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
	_, _ = h.Client.Queue.Add(context.Background(), "proj:epic-a/02", "")
	h.Server.Scheduler().ClaimNext()
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
	h.Server.Recovery().RecoverAsync(recovery.Failure{Address: "proj:epic-a", Type: events.Deadlocked, Kind: events.AllPaused, Reason: "every remaining ticket is paused"})
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
	if q := rigs.QueueAddresses(t, h); !slices.Contains(q, "proj:epic-a/03") {
		t.Errorf("queue = %v, want 03 queued", q)
	}

	// A second deadlock of the same kind hits the per-kind cap.
	h.Server.Recovery().RecoverAsync(recovery.Failure{Address: "proj:epic-a", Type: events.Deadlocked, Kind: events.AllPaused, Reason: "again"})
	waitFor(t, func() bool { return len(runLogOf(t, h, events.RecoveryEscalated)) > 0 })
	time.Sleep(100 * time.Millisecond) // negative wait: a second recovery would show by now
	if got := runLogOf(t, h, events.RecoveryApplied); len(got) != 1 {
		t.Errorf("recovery-applied = %+v, want only the first", got)
	}
}

// waitingEpic is epic-a waiting on the done, unmerged epic "blocker": the
// Waiting state. Ticket 01 is queued; extra tickets replace or add to it.
func waitingEpic(t *testing.T, landing string, extra map[string]servertest.TicketOpts) (*servertest.Harness, string) {
	t.Helper()
	store := t.TempDir()
	repo := testutil.TempRepo(t)
	if landing != "" { // an on-done epic stacks on the blocker's branch
		servertest.CommitOnBranch(t, repo, servertest.StackBlocker)
	}
	servertest.WriteCompleteEpic(t, store, "blocker")
	servertest.WriteTicketWith(t, store, "proj", "epic-a", "01", "first", servertest.TicketOpts{})
	for id, opts := range extra {
		servertest.WriteTicketWith(t, store, "proj", "epic-a", id, "first", opts)
	}
	servertest.WriteEpicBlockedBy(t, store, "proj", "epic-a", "blocker")
	if err := servertest.WriteProjectRepo(store, "proj", repo, landing); err != nil {
		t.Fatal(err)
	}
	h := servertest.StartWithStore(t, store)
	if _, err := h.Client.Queue.Add(context.Background(), "proj:epic-a/01", ""); err != nil {
		t.Fatal(err)
	}
	return h, store
}

// An epic that only waits on an unreleased blocker epic is not a deadlock: it
// logs nothing, starts no recovery, keeps its queue place and runs no agent.
func TestDeadlock_WaitingEpicLogsNothingAndStartsNoRecovery(t *testing.T) {
	t.Parallel()
	h, _ := waitingEpic(t, "", nil)
	for range 3 {
		h.Server.Scheduler().ClaimNext()
	}
	time.Sleep(200 * time.Millisecond) // negative wait: a deadlock event would show by now
	if got := runLogOf(t, h, events.Deadlocked); len(got) != 0 {
		t.Errorf("deadlocked events = %+v, want none", got)
	}
	if got := runLogOf(t, h, events.RecoveryApplied); len(got) != 0 {
		t.Errorf("recovery-applied = %+v, want none", got)
	}
	if q := rigs.QueueAddresses(t, h); !slices.Contains(q, "proj:epic-a/01") {
		t.Errorf("queue = %v, want proj:epic-a/01 kept", q)
	}
	if n := h.Server.Registry().CountRoot("proj:epic-a"); n != 0 {
		t.Errorf("running sessions = %d, want 0", n)
	}
}

// The blocker merging releases the waiting epic, which then starts by itself.
func TestDeadlock_WaitingEpicStartsWhenBlockerReleases(t *testing.T) {
	t.Parallel()
	h, store := waitingEpic(t, "", nil)
	h.Server.Scheduler().ClaimNext()
	if err := tickets.StampEpicFields(filepath.Join(store, "proj", "blocker"), map[string]any{"merged_at": time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	h.Server.Rescan()
	h.Server.Scheduler().ClaimNext()
	servertest.WaitStatus(t, filepath.Join(store, "proj", "epic-a", "issues", "01-first.md"), "claimed")
	if got := runLogOf(t, h, events.Deadlocked); len(got) != 0 {
		t.Errorf("deadlocked events = %+v, want none", got)
	}
}

// Under the on-done policy a done blocker releases the epic without a merge.
func TestDeadlock_WaitingEpicWithOnDoneBlockerReleases(t *testing.T) {
	t.Parallel()
	h, store := waitingEpic(t, `,"landing":"on-done"`, nil)
	h.Server.Rescan()
	h.Server.Scheduler().ClaimNext()
	servertest.WaitStatus(t, filepath.Join(store, "proj", "epic-a", "issues", "01-first.md"), "claimed")
	if got := runLogOf(t, h, events.Deadlocked); len(got) != 0 {
		t.Errorf("deadlocked events = %+v, want none", got)
	}
}

// A paused ticket in a waiting epic is still all-paused: a person can clear it.
func TestDeadlock_PausedTicketBeatsWaiting(t *testing.T) {
	t.Parallel()
	h, _ := waitingEpic(t, "", nil)
	if err := h.Server.PauseAs("proj:epic-a/01", events.IterationError, "broken"); err != nil {
		t.Fatalf("pause: %v", err)
	}
	h.Server.Scheduler().ClaimNext()
	waitFor(t, func() bool { return len(runLogOf(t, h, events.Deadlocked)) > 0 })
	h.Server.Scheduler().ClaimNext()
	got := runLogOf(t, h, events.Deadlocked)
	if len(got) != 1 || got[0].Kind != string(events.AllPaused) {
		t.Errorf("deadlocked events = %+v, want one all-paused", got)
	}
}

// A blocked cycle inside an epic that also waits on a blocker still deadlocks.
func TestDeadlock_CycleStillDeadlocksWhileBlockerUnreleased(t *testing.T) {
	t.Parallel()
	h, _ := waitingEpic(t, "", map[string]servertest.TicketOpts{
		"01": {BlockedBy: []string{"02"}},
		"02": {BlockedBy: []string{"01"}},
	})
	h.Server.Scheduler().ClaimNext()
	waitFor(t, func() bool { return len(runLogOf(t, h, events.Deadlocked)) > 0 })
	h.Server.Scheduler().ClaimNext()
	got := runLogOf(t, h, events.Deadlocked)
	if len(got) != 1 || got[0].Kind != string(events.BlockedCycle) {
		t.Errorf("deadlocked events = %+v, want one blocked-cycle", got)
	}
}

// Waiting forgets an earlier deadlock entry, so a later deadlock logs again.
func TestDeadlock_LeavingCycleForgetsTheEntry(t *testing.T) {
	t.Parallel()
	h, store := waitingEpic(t, "", map[string]servertest.TicketOpts{
		"01": {BlockedBy: []string{"02"}},
		"02": {BlockedBy: []string{"01"}},
	})
	h.Server.Scheduler().ClaimNext()
	waitFor(t, func() bool { return len(runLogOf(t, h, events.Deadlocked)) == 1 })

	servertest.WriteTicketWith(t, store, "proj", "epic-a", "01", "first", servertest.TicketOpts{})
	h.Server.Rescan()
	h.Server.Scheduler().ClaimNext()
	time.Sleep(200 * time.Millisecond) // negative wait: a second event would show by now
	if got := runLogOf(t, h, events.Deadlocked); len(got) != 1 {
		t.Fatalf("deadlocked events = %+v, want still one while Waiting", got)
	}

	servertest.WriteTicketWith(t, store, "proj", "epic-a", "03", "third", servertest.TicketOpts{Status: "error"})
	h.Server.Rescan()
	h.Server.Scheduler().ClaimNext()
	waitFor(t, func() bool { return len(runLogOf(t, h, events.Deadlocked)) == 2 })
}
