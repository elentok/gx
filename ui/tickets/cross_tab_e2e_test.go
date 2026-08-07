package tickets

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/keys"
	"github.com/elentok/gx/ui/notify"
)

// TestCrossTabSwitchingDuringTwoLiveRunsStaysConsistent switches repeatedly
// between the Tickets and Queue tabs while two epics run concurrently,
// proving both tabs recover the same registry-owned truth on every
// (re)activation — neither tab keeps its own event reader or a duplicated
// live-event map, per ticket 17's snapshot-only migration.
func TestCrossTabSwitchingDuringTwoLiveRunsStaysConsistent(t *testing.T) {
	root := t.TempDir()
	writeTicket(t, root, "epic-a", "01-first.md", "Status: claimed\n\nBody.\n")
	writeTicket(t, root, "epic-b", "01-first.md", "Status: claimed\n\nBody.\n")
	checked := map[string]bool{
		ticketPath(root, "epic-a", "01-first.md"): true,
		ticketPath(root, "epic-b", "01-first.md"): true,
	}

	tm := NewModel(root, ui.Settings{}, keys.New(nil))
	tm = deliverLoad(t, tm)
	updated, _ := tm.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	tm = updated.(Model)

	qm := loadQueueModel(t, NewQueueModel(root, ui.Settings{}, checked, keys.Manager{}))

	r := newLoopRegistry(2)
	r.tryStart("epic-a", 0, 1)
	r.tryStart("epic-b", 0, 1)
	r.reduceLiveEvent("epic-a", ralphloop.LiveEvent{Kind: ralphloop.LiveEventIterationStarted, Label: "iter-01a", Identifier: "01"})
	r.reduceLiveEvent("epic-b", ralphloop.LiveEvent{Kind: ralphloop.LiveEventIterationStarted, Label: "iter-01b", Identifier: "01"})
	previous := ralphLoopRegistry
	ralphLoopRegistry = r
	t.Cleanup(func() {
		ralphLoopRegistry = previous
	})

	// Neither tm nor qm ever received an implementStartedMsg for either
	// epic — as if both runs were launched, and both switches below
	// happened, entirely while each tab was backgrounded.
	tm = deliverCmd(t, tm, tm.OnPageActivated())
	if content := tm.View().Content; strings.Count(content, "implementing...") != 2 {
		t.Fatalf("Tickets after switch-in: want both epics running, got:\n%s", content)
	}

	updated, _ = qm.Update(qm.OnPageActivated()())
	qm = updated.(QueueModel)
	if !qm.runningEpics["epic-a"] || !qm.runningEpics["epic-b"] {
		t.Fatalf("Queue after switch-in: want both epics running, got %v", qm.runningEpics)
	}

	// epic-a fails and epic-b keeps progressing while both tabs are away
	// again — no page ever sees the raw events, only the registry's
	// resulting snapshot.
	wantErr := errors.New("epic-a boom")
	r.finish("epic-a", wantErr)
	r.reduceLiveEvent("epic-b", ralphloop.LiveEvent{Kind: ralphloop.LiveEventContextOccupancy, Identifier: "01", Tokens: 4000})

	tm = deliverCmd(t, tm, tm.OnPageActivated())
	if tm.implementingEpics["epic-a"] {
		t.Fatalf("Tickets after epic-a failed: want epic-a cleared, got %#v", tm.implementingEpics)
	}
	if !tm.implementingEpics["epic-b"] {
		t.Fatalf("Tickets after epic-a failed: want epic-b still tracked, got %#v", tm.implementingEpics)
	}

	updated, _ = qm.Update(qm.OnPageActivated()())
	qm = updated.(QueueModel)
	if qm.runningEpics["epic-a"] {
		t.Fatalf("Queue after epic-a failed: want epic-a cleared, got %v", qm.runningEpics)
	}
	if !qm.runningEpics["epic-b"] {
		t.Fatalf("Queue after epic-a failed: want epic-b still tracked, got %v", qm.runningEpics)
	}

	// Both tabs derive a finished run's toast from the same single source
	// (implementFinishedNotifyCmd, gated on the registry's lastError) — so
	// epic-a's failure surfaces as an error on whichever tab notices it
	// first, and can never be raced into a success toast by the other.
	msg := implementFinishedNotifyCmd("epic-a")()
	notifyMsg, ok := msg.(notify.NotifyMsg)
	if !ok || notifyMsg.Kind != notify.KindError {
		t.Fatalf("implementFinishedNotifyCmd(epic-a) = %#v, want an error notification", msg)
	}

	r.finish("epic-b", nil)
}

// TestCrossTabCheckThenQueueResetsTicketsCheckboxWhileQueueTabKeepsEntries
// covers ticket 15's end-to-end flow: checking tickets in the Tickets tab,
// then pressing "r" ("Replace queue", renamed from "i" by ticket 10) to
// queue them, must reset the Tickets tab's checkboxes
// (the independent checked set) while the same tickets remain visible and
// pending in the Queue tab — the two tabs share one QueueStore, so this
// pins that the clear-on-queue write is actually observable cross-tab, not
// just within the Model that performed it.
func TestCrossTabCheckThenQueueResetsTicketsCheckboxWhileQueueTabKeepsEntries(t *testing.T) {
	withQueueStateDir(t)
	root := t.TempDir()
	writeTicket(t, root, "my-epic", "01-first.md", "Status: open\n\nBody.\n")
	writeTicket(t, root, "my-epic", "02-second.md", "Status: open\n\nBody.\n")
	first := ticketPath(root, "my-epic", "01-first.md")
	second := ticketPath(root, "my-epic", "02-second.md")

	store := loadQueueStoreAt(filepath.Join(t.TempDir(), "queue.json"))

	tm := NewModelWithStore(root, ui.Settings{}, keys.New(nil), store)
	tm = deliverLoad(t, tm)
	updated, _ := tm.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	tm = updated.(Model)

	if err := store.SetTicketChecked([]string{first, second}, true); err != nil {
		t.Fatal(err)
	}
	tm.refreshQueueSnapshot()
	if len(tm.checked) != 2 {
		t.Fatalf("Tickets checked set before queueing = %v, want both tickets checked", tm.checked)
	}

	updated, _ = tm.handleReplaceQueueKey()
	tm = updated.(Model)

	if len(tm.checked) != 0 {
		t.Fatalf("Tickets checked set after queueing = %v, want empty", tm.checked)
	}

	qm := loadQueueModel(t, NewQueueModelWithStore(root, ui.Settings{}, keys.Manager{}, store))
	if content := qm.View().Content; !strings.Contains(content, "First") || !strings.Contains(content, "Second") {
		t.Fatalf("Queue tab after queueing: want both tickets listed, got:\n%s", content)
	}
	status := store.Snapshot().Status
	if status[first] != queueStatusPending || status[second] != queueStatusPending {
		t.Fatalf("queue status after queueing = %v, want both pending", status)
	}
}

// TestCrossTabLiveMetricsRenderSameFiguresFromSharedProjection asserts both
// tabs render the *actual* elapsed-time and token-count figures from a live
// run, not just an "implementing..." marker — the gap ticket 06 found: the
// two tabs' syncRunSnapshot copies had already drifted once (one dropped
// startedAt) and no test caught it because coverage only checked for the
// marker's presence. Now that both tabs' syncRunSnapshot call the shared
// projectLiveTickets (ticket 21), this pins the rendered numbers so a future
// divergence between the tabs' bookkeeping fails here.
func TestCrossTabLiveMetricsRenderSameFiguresFromSharedProjection(t *testing.T) {
	root := t.TempDir()
	writeTicket(t, root, "epic-a", "01-first.md", "Status: claimed\n\nBody.\n")

	tm := NewModel(root, ui.Settings{}, keys.New(nil))
	tm = deliverLoad(t, tm)
	// Wide enough that the sidebar panel's half-width column (ticket 06 folded
	// the live suffix and elapsed/token metrics onto the title's own line) has
	// room for the full combined text without truncating.
	updated, _ := tm.Update(tea.WindowSizeMsg{Width: 220, Height: 40})
	tm = updated.(Model)

	qm := loadQueueModel(t, NewQueueModel(root, ui.Settings{}, map[string]bool{
		ticketPath(root, "epic-a", "01-first.md"): true,
	}, keys.Manager{}))

	r := newLoopRegistry(1)
	r.tryStart("epic-a", 0, 1)
	r.reduceLiveEvent("epic-a", ralphloop.LiveEvent{Kind: ralphloop.LiveEventIterationStarted, Label: "iter-01a", Identifier: "01"})
	r.reduceLiveEvent("epic-a", ralphloop.LiveEvent{Kind: ralphloop.LiveEventContextOccupancy, Identifier: "01", Tokens: 4200})
	// Backdate the ticket's own start so elapsed time renders as a real
	// nonzero duration ("2m03s") instead of racing the clock for a nonzero
	// "0s"/"1s".
	ticket := r.runs["epic-a"].tickets["01"]
	ticket.StartedAt = time.Now().Add(-123 * time.Second)
	r.runs["epic-a"].tickets["01"] = ticket
	previous := ralphLoopRegistry
	ralphLoopRegistry = r
	t.Cleanup(func() {
		ralphLoopRegistry = previous
	})

	tm = deliverCmd(t, tm, tm.OnPageActivated())
	ticketsContent := tm.View().Content
	if !strings.Contains(ticketsContent, "2m03s") || !strings.Contains(ticketsContent, "4.2k tok") {
		t.Fatalf("Tickets tab: want rendered elapsed %q and tokens %q, got:\n%s", "2m03s", "4.2k tok", ticketsContent)
	}

	updated, _ = qm.Update(qm.OnPageActivated()())
	qm = updated.(QueueModel)
	queueContent := qm.View().Content
	if !strings.Contains(queueContent, "2m03s") || !strings.Contains(queueContent, "4.2k tok") {
		t.Fatalf("Queue tab: want rendered elapsed %q and tokens %q, got:\n%s", "2m03s", "4.2k tok", queueContent)
	}

	r.finish("epic-a", nil)
}
