package tickets

import (
	"path/filepath"
	"testing"

	"github.com/elentok/gx/tickets"
)

// TestModel_ImplementKeyWithNoActiveLoopReplacesPendingSelection covers
// ticket 11: with no ralph-loop running, "i" replaces the queue's
// not-yet-started (pending) entries with the current checked selection,
// directly and without a confirmation — while running/done entries are left
// exactly as they are, whether or not they're still part of the selection.
func TestModel_ImplementKeyWithNoActiveLoopReplacesPendingSelection(t *testing.T) {
	worktreeRoot := t.TempDir()
	scratch := func(name string) string {
		return filepath.Join(worktreeRoot, ".scratch", "alpha", "issues", name)
	}
	running := scratch("01-running.md")
	done := scratch("02-done.md")
	stalePending := scratch("03-stale.md")
	newSelection := scratch("04-new.md")

	store := loadQueueStoreAt(filepath.Join(t.TempDir(), "queue.json"))
	for _, p := range []string{running, done, stalePending} {
		if err := store.Check(p); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.SetStatus(running, queueStatusRunning); err != nil {
		t.Fatal(err)
	}
	if err := store.SetStatus(done, queueStatusDone); err != nil {
		t.Fatal(err)
	}

	m := Model{
		worktreeRoot: worktreeRoot,
		queueStore:   store,
		// The new checked selection: still includes the running/done tickets
		// (they render checked regardless of status), drops the stale pending
		// one, and adds a fresh pending one.
		checked:    map[string]bool{running: true, done: true, newSelection: true},
		checkOrder: map[string]uint64{running: 1, done: 2, newSelection: 3},
	}

	updated, cmd := m.handleImplementKey()
	m = updated.(Model)
	if cmd == nil {
		t.Fatal("expected a tab-switch command, got nil")
	}
	if m.confirm.IsOpen {
		t.Fatal("expected no confirmation with no active loop")
	}

	status := store.Snapshot().Status
	if got := status[running]; got != queueStatusRunning {
		t.Fatalf("running entry status = %v, want untouched running", got)
	}
	if got := status[done]; got != queueStatusDone {
		t.Fatalf("done entry status = %v, want untouched done", got)
	}
	if got, ok := status[stalePending]; ok {
		t.Fatalf("stale pending entry should have been replaced, still present as %v", got)
	}
	if got := status[newSelection]; got != queueStatusPending {
		t.Fatalf("new selection entry status = %v, want pending", got)
	}

	if len(m.checked) != 0 {
		t.Fatalf("checked set = %v, want empty after queueing (ticket 15)", m.checked)
	}
	snapshot := store.Snapshot()
	if len(snapshot.TicketChecked) != 0 {
		t.Fatalf("store TicketChecked = %v, want empty after queueing", snapshot.TicketChecked)
	}
	for _, p := range []string{running, done, newSelection} {
		if _, ok := snapshot.Status[p]; !ok {
			t.Fatalf("queue status missing %q after checked-set clear", p)
		}
	}
}

// TestModel_ImplementKeyLeavesOtherWorktreeEntriesUntouched covers
// replaceQueuedSelection's scope boundary: a pending entry belonging to
// another worktree isn't visible in this tab's checked selection, so it must
// survive the replace rather than being dropped as "not part of the new
// selection".
func TestModel_ImplementKeyLeavesOtherWorktreeEntriesUntouched(t *testing.T) {
	worktreeRoot := t.TempDir()
	otherWorktreeRoot := t.TempDir()
	otherPending := filepath.Join(otherWorktreeRoot, ".scratch", "alpha", "issues", "01-other.md")
	newSelection := filepath.Join(worktreeRoot, ".scratch", "alpha", "issues", "02-new.md")

	store := loadQueueStoreAt(filepath.Join(t.TempDir(), "queue.json"))
	if err := store.Check(otherPending); err != nil {
		t.Fatal(err)
	}

	m := Model{
		worktreeRoot: worktreeRoot,
		queueStore:   store,
		checked:      map[string]bool{newSelection: true},
		checkOrder:   map[string]uint64{newSelection: 1},
	}

	updated, _ := m.handleImplementKey()
	m = updated.(Model)

	status := store.Snapshot().Status
	if got := status[otherPending]; got != queueStatusPending {
		t.Fatalf("other worktree's pending entry status = %v, want untouched pending", got)
	}
	if got := status[newSelection]; got != queueStatusPending {
		t.Fatalf("new selection entry status = %v, want pending", got)
	}
}

// TestModel_ImplementKeyExcludesAlreadyDoneTickets covers ticket 05(b): a
// checked selection that includes a ticket whose Epic.RenderedStatus is
// already tickets.StatusDone must not be enqueued — it has nothing left to
// implement — while the rest of the checked selection still queues normally.
func TestModel_ImplementKeyExcludesAlreadyDoneTickets(t *testing.T) {
	worktreeRoot := t.TempDir()
	scratch := func(name string) string {
		return filepath.Join(worktreeRoot, ".scratch", "alpha", "issues", name)
	}
	donePath := scratch("01-done.md")
	openPath := scratch("02-open.md")

	epic := tickets.Epic{
		Name: "alpha",
		Tickets: []tickets.Ticket{
			{Number: 1, Identifier: "01", Path: donePath, Status: "done"},
			{Number: 2, Identifier: "02", Path: openPath, Status: "open"},
		},
	}

	store := loadQueueStoreAt(filepath.Join(t.TempDir(), "queue.json"))
	m := Model{
		worktreeRoot: worktreeRoot,
		queueStore:   store,
		epics:        []tickets.Epic{epic},
		checked:      map[string]bool{donePath: true, openPath: true},
		checkOrder:   map[string]uint64{donePath: 1, openPath: 2},
	}

	updated, _ := m.handleImplementKey()
	m = updated.(Model)

	status := store.Snapshot().Status
	if _, ok := status[donePath]; ok {
		t.Fatalf("done ticket should not have been enqueued, got status %v", status[donePath])
	}
	if got := status[openPath]; got != queueStatusPending {
		t.Fatalf("open ticket status = %v, want pending", got)
	}
	if len(m.checked) != 0 {
		t.Fatalf("checked set = %v, want empty after queueing", m.checked)
	}
}
