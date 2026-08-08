package tickets

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/ui/notify"
)

// TestModel_ImplementKeyReplacesPendingSelectionAfterConfirmation covers
// bugs-05/03: "r" ("Replace queue") opens the same confirmation "a" already
// goes through before touching anything; only once that's accepted
// (handleReplaceQueueConfirmed) does it replace the queue's not-yet-started
// (pending) entries with the current checked selection — while running/done
// entries are left exactly as they are, whether or not they're still part of
// the selection.
func TestModel_ImplementKeyReplacesPendingSelectionAfterConfirmation(t *testing.T) {
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

	updated, cmd := m.handleReplaceQueueKey()
	m = updated.(Model)
	if cmd != nil {
		t.Fatal("expected no cmd until the confirmation is accepted")
	}
	if !m.confirm.IsOpen {
		t.Fatal("expected the confirmation modal to open")
	}

	confirmedMsg := cmdConfirmReplaceQueue(m.worktreeRoot)()
	updated, cmd = m.handleReplaceQueueConfirmed(confirmedMsg.(replaceQueueConfirmedMsg))
	m = updated.(Model)
	if cmd == nil {
		t.Fatal("expected a tab-switch command, got nil")
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

	updated, _ := m.handleReplaceQueueKey()
	m = updated.(Model)
	confirmedMsg := cmdConfirmReplaceQueue(m.worktreeRoot)()
	updated, _ = m.handleReplaceQueueConfirmed(confirmedMsg.(replaceQueueConfirmedMsg))
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

// TestModel_ImplementKeyNotBlockedByUnrelatedRunningEpic covers bugs-05/03:
// a live run on an epic other than the one under the cursor no longer blocks
// "r" — only the cursor's own epic having a live run does. Pressing "r" here
// opens the confirmation instead of showing "Can't replace a live queue".
func TestModel_ImplementKeyNotBlockedByUnrelatedRunningEpic(t *testing.T) {
	worktreeRoot := t.TempDir()
	epic := tickets.Epic{Name: "my-epic", Tickets: []tickets.Ticket{
		{Number: 1, Identifier: "01", Status: "open"},
	}}

	store := loadQueueStoreAt(filepath.Join(t.TempDir(), "queue.json"))

	r := newLoopRegistry(2)
	r.tryStart("unrelated-epic", 0, 1)
	previous := ralphLoopRegistry
	ralphLoopRegistry = r
	t.Cleanup(func() {
		r.finish("unrelated-epic", nil)
		ralphLoopRegistry = previous
	})

	m := Model{
		worktreeRoot: worktreeRoot,
		queueStore:   store,
		epics:        []tickets.Epic{epic},
		checked:      map[string]bool{"/my-epic/01.md": true},
		checkOrder:   map[string]uint64{"/my-epic/01.md": 1},
	}

	updated, cmd := m.handleReplaceQueueKey()
	m = updated.(Model)
	if cmd != nil {
		t.Fatal("expected no cmd until the confirmation is accepted")
	}
	if !m.confirm.IsOpen {
		t.Fatal("expected the confirmation modal to open, cursor epic isn't the running one")
	}
}

// TestModel_ImplementKeyBlockedWhenCursorEpicRunning covers bugs-05/03: "r"
// is still blocked when the epic under the cursor itself has a live run,
// showing "Can't replace a live queue" and never opening the confirmation.
func TestModel_ImplementKeyBlockedWhenCursorEpicRunning(t *testing.T) {
	worktreeRoot := t.TempDir()
	scratch := func(name string) string {
		return filepath.Join(worktreeRoot, ".scratch", "my-epic", "issues", name)
	}
	stalePending := scratch("03-stale.md")

	epic := tickets.Epic{Name: "my-epic", Tickets: []tickets.Ticket{
		{Number: 1, Identifier: "01", Status: "open"},
	}}

	store := loadQueueStoreAt(filepath.Join(t.TempDir(), "queue.json"))
	if err := store.Check(stalePending); err != nil {
		t.Fatal(err)
	}
	if err := store.SetStatus(stalePending, queueStatusPending); err != nil {
		t.Fatal(err)
	}

	r := newLoopRegistry(1)
	r.tryStart("my-epic", 0, 1)
	previous := ralphLoopRegistry
	ralphLoopRegistry = r
	t.Cleanup(func() {
		r.finish("my-epic", nil)
		ralphLoopRegistry = previous
	})

	m := Model{
		worktreeRoot: worktreeRoot,
		queueStore:   store,
		epics:        []tickets.Epic{epic},
		checked:      map[string]bool{"/my-epic/01.md": true},
		checkOrder:   map[string]uint64{"/my-epic/01.md": 1},
	}

	updated, cmd := m.handleReplaceQueueKey()
	m = updated.(Model)
	if cmd == nil {
		t.Fatal("expected a notify command, got nil")
	}
	msg := cmd()
	notifyMsg, ok := msg.(notify.NotifyMsg)
	if !ok || notifyMsg.Message != "Can't replace a live queue" {
		t.Fatalf("cmd() = %#v, want the disabled notification", msg)
	}
	if m.confirm.IsOpen {
		t.Fatal("expected no confirmation while disabled")
	}
	if got := store.Snapshot().Status[stalePending]; got != queueStatusPending {
		t.Fatalf("pending entry status = %v, want untouched pending", got)
	}
	if len(m.checked) != 1 || !m.checked["/my-epic/01.md"] {
		t.Fatalf("checked set = %v, want untouched", m.checked)
	}
}

// TestModel_AddToQueueKeyNotRunningEpic covers ticket 10: "a" against an
// epic under the cursor that has no live run is a no-op with an info
// notification, and never opens the confirmation modal.
func TestModel_AddToQueueKeyNotRunningEpic(t *testing.T) {
	epic := tickets.Epic{Name: "alpha", Tickets: []tickets.Ticket{
		{Number: 1, Identifier: "01", Path: "/alpha/01.md", Status: "open"},
	}}
	m := Model{epics: []tickets.Epic{epic}, checked: map[string]bool{"/alpha/01.md": true}}

	updated, cmd := m.handleAddToQueueKey()
	m = updated.(Model)
	if cmd == nil {
		t.Fatal("expected a notify command, got nil")
	}
	msg := cmd()
	notifyMsg, ok := msg.(notify.NotifyMsg)
	if !ok || !strings.Contains(notifyMsg.Message, "isn't running") {
		t.Fatalf("cmd() = %#v, want an \"isn't running\" notification", msg)
	}
	if m.confirm.IsOpen {
		t.Fatal("expected no confirmation for a non-running epic")
	}
}

// TestModel_AddToQueueKeyNothingChecked covers ticket 10: "a" against a
// running epic with nothing checked from it is a no-op with an info
// notification, and never opens the confirmation modal.
func TestModel_AddToQueueKeyNothingChecked(t *testing.T) {
	epic := tickets.Epic{Name: "alpha", Tickets: []tickets.Ticket{
		{Number: 1, Identifier: "01", Path: "/alpha/01.md", Status: "open"},
	}}
	r := newLoopRegistry(1)
	r.tryStart("alpha", 0, 1)
	previous := ralphLoopRegistry
	ralphLoopRegistry = r
	t.Cleanup(func() {
		r.finish("alpha", nil)
		ralphLoopRegistry = previous
	})

	m := Model{epics: []tickets.Epic{epic}, checked: map[string]bool{}}

	updated, cmd := m.handleAddToQueueKey()
	m = updated.(Model)
	if cmd == nil {
		t.Fatal("expected a notify command, got nil")
	}
	msg := cmd()
	notifyMsg, ok := msg.(notify.NotifyMsg)
	if !ok || !strings.Contains(notifyMsg.Message, "check at least one ticket") {
		t.Fatalf("cmd() = %#v, want a \"check at least one ticket\" notification", msg)
	}
	if m.confirm.IsOpen {
		t.Fatal("expected no confirmation with nothing checked")
	}
}

// TestModel_AddToQueueKeyOpensConfirmationWithCount covers ticket 10: "a"
// against a running epic with checked tickets opens the confirmation naming
// the checked count, and never widens the scope before it's accepted.
func TestModel_AddToQueueKeyOpensConfirmationWithCount(t *testing.T) {
	epic := tickets.Epic{Name: "alpha", Tickets: []tickets.Ticket{
		{Number: 1, Identifier: "01", Path: "/alpha/01.md", Status: "open"},
		{Number: 2, Identifier: "02", Path: "/alpha/02.md", Status: "open"},
	}}
	r := newLoopRegistry(1)
	r.tryStart("alpha", 0, 1)
	scope, err := ralphloop.ResolveRunScope(epic, []string{"01"})
	if err != nil {
		t.Fatal(err)
	}
	r.setScope("alpha", scope)
	previous := ralphLoopRegistry
	ralphLoopRegistry = r
	t.Cleanup(func() {
		r.finish("alpha", nil)
		ralphLoopRegistry = previous
	})

	m := Model{epics: []tickets.Epic{epic}, checked: map[string]bool{"/alpha/02.md": true}}

	updated, cmd := m.handleAddToQueueKey()
	m = updated.(Model)
	if cmd != nil {
		t.Fatal("expected no cmd until the confirmation is accepted")
	}
	if !m.confirm.IsOpen {
		t.Fatal("expected the confirmation modal to open")
	}
	if !strings.Contains(m.confirm.View(80), "Add 1 ticket(s) to the live queue?") {
		t.Fatalf("confirm view = %q, want it to name the checked count", m.confirm.View(80))
	}
	if scope.Contains(epic.Tickets[1], epic) {
		t.Fatal("expected the scope untouched before the confirmation is accepted")
	}
}

// TestCmdAddToLiveQueueWidensRunningScope covers ticket 10's core mechanism:
// accepting "a"'s confirmation widens the targeted epic's live RunScope via
// ralphloop.RunScope.Add (ticket 09), making the added ticket claimable.
func TestCmdAddToLiveQueueWidensRunningScope(t *testing.T) {
	epic := tickets.Epic{Name: "alpha", Tickets: []tickets.Ticket{
		{Number: 1, Identifier: "01", Status: "open"},
		{Number: 2, Identifier: "02", Status: "open"},
	}}
	r := newLoopRegistry(1)
	r.tryStart("alpha", 0, 1)
	scope, err := ralphloop.ResolveRunScope(epic, []string{"01"})
	if err != nil {
		t.Fatal(err)
	}
	r.setScope("alpha", scope)
	previous := ralphLoopRegistry
	ralphLoopRegistry = r
	t.Cleanup(func() {
		r.finish("alpha", nil)
		ralphLoopRegistry = previous
	})

	ticket02 := epic.Tickets[1]
	if scope.Contains(ticket02, epic) {
		t.Fatal("precondition: ticket 02 should not yet be in scope")
	}

	msg := cmdAddToLiveQueue("alpha", []string{"02"})()
	notifyMsg, ok := msg.(notify.NotifyMsg)
	if !ok || notifyMsg.Kind != notify.KindInfo {
		t.Fatalf("cmdAddToLiveQueue msg = %#v, want an info notification", msg)
	}

	widened, ok := r.scopeFor("alpha")
	if !ok {
		t.Fatal("expected alpha still running")
	}
	if !widened.Contains(ticket02, epic) {
		t.Fatal("expected ticket 02 to be in the widened scope after accepting")
	}
}
