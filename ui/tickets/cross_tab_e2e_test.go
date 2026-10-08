package tickets

import (
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/keys"
)

// TestCrossTabCheckThenQueueResetsTicketsCheckboxWhileQueueTabKeepsEntries
// covers ticket 15's end-to-end flow: checking tickets in the Tickets tab,
// then pressing "r" ("Replace queue", renamed from "i" by ticket 10) to
// queue them, must reset the Tickets tab's checkboxes
// (the independent checked set) while the same tickets remain visible and
// pending in the Queue tab — the two tabs share one QueueStore, so this
// pins that the clear-on-queue write is actually observable cross-tab, not
// just within the Model that performed it.
func TestCrossTabCheckThenQueueResetsTicketsCheckboxWhileQueueTabKeepsEntries(t *testing.T) {
	// not parallel-safe: reassigns the package-level queueStateDirFn singleton
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
	if !tm.confirm.IsOpen {
		t.Fatal("expected the confirmation modal to open")
	}
	confirmedMsg := cmdConfirmReplaceQueue(tm.worktreeRoot)()
	updated, _ = tm.handleReplaceQueueConfirmed(confirmedMsg.(replaceQueueConfirmedMsg))
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
