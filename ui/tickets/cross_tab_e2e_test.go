package tickets

import (
	"errors"
	"strings"
	"testing"

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

	qm := loadQueueModel(t, NewQueueModel(root, ui.Settings{}, checked))

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
	updated, _ = tm.Update(tm.OnPageActivated()())
	tm = updated.(Model)
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

	updated, _ = tm.Update(tm.OnPageActivated()())
	tm = updated.(Model)
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
