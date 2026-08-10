package tickets

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/keys"
)

// TestQueueRunStateAndTitleReflectParkedEpics covers 11a5's queueRunState/
// queueHeaderTitle parked-aware branches: parked wins over a concurrently
// running epic (m.parkedEpics and m.runningEpics are independent per-epic
// maps, per queueRunState's doc comment), and the title names the lowest
// epic name's lowest ticket identifier deterministically.
func TestQueueRunStateAndTitleReflectParkedEpics(t *testing.T) {
	root := t.TempDir()
	writeTicket(t, root, "alpha", "01-first.md", "Status: open\n\nBody.\n")
	writeTicket(t, root, "beta", "01-first.md", "Status: open\n\nBody.\n")
	checked := map[string]bool{
		ticketPath(root, "alpha", "01-first.md"): true,
		ticketPath(root, "beta", "01-first.md"):  true,
	}
	m := loadQueueModel(t, NewQueueModel(root, ui.Settings{}, checked, keys.Manager{}))

	m.parkedEpics["beta"] = []ralphloop.StalledTicket{{Identifier: "02"}}
	m.parkedEpics["alpha"] = []ralphloop.StalledTicket{{Identifier: "01"}}
	m.runningEpics["beta"] = true

	if got := m.queueRunState(); got != queueRunParked {
		t.Fatalf("queueRunState() = %v, want queueRunParked even with a concurrently running epic", got)
	}
	title := m.queueHeaderTitle()
	if !strings.Contains(title, "parked") || !strings.Contains(title, "01") {
		t.Fatalf("queueHeaderTitle() = %q, want it to mention parked and alpha's ticket 01 (lowest epic name)", title)
	}
	if strings.Contains(title, "02") {
		t.Fatalf("queueHeaderTitle() = %q, want it not to name beta's ticket 02", title)
	}
	if m.queueHeaderBodyLines() != nil {
		t.Fatalf("queueHeaderBodyLines() = %v, want nil for parked (title already carries the detail)", m.queueHeaderBodyLines())
	}
}

// TestQueueModelEnterOnParkedRowResumesEvenWhenCheckedAndLaunchable covers
// the priority case 11a4's "enter" branch was written to resolve: a parked
// epic's row still resumes (Gate.WakeParked) instead of opening the
// implement-agent menu, even though its ticket is checked and would
// otherwise be launchable.
func TestQueueModelEnterOnParkedRowResumesEvenWhenCheckedAndLaunchable(t *testing.T) {
	root := t.TempDir()
	writeTicket(t, root, "alpha", "01-first.md", "Status: open\n\nBody.\n")
	checked := map[string]bool{ticketPath(root, "alpha", "01-first.md"): true}
	m := loadQueueModel(t, NewQueueModel(root, ui.Settings{}, checked, keys.Manager{}))

	r := newLoopRegistry(1)
	r.tryStart("alpha", 0, 1)
	r.reduceLiveEvent("alpha", ralphloop.LiveEvent{
		Kind:    ralphloop.LiveEventEpicParked,
		Stalled: []ralphloop.StalledTicket{{Identifier: "01"}},
	})
	previous := ralphLoopRegistry
	ralphLoopRegistry = r
	t.Cleanup(func() {
		r.finish("alpha", nil)
		ralphLoopRegistry = previous
	})
	m.parkedEpics["alpha"] = []ralphloop.StalledTicket{{Identifier: "01"}}

	wake := r.gateFor("alpha").ParkWake()

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(QueueModel)

	select {
	case <-wake:
	case <-time.After(time.Second):
		t.Fatal("expected enter on a parked row to call resumeParked/WakeParked")
	}
	if m.implementAgentMenuOpen {
		t.Fatal("expected enter on a parked row not to open the implement-agent menu")
	}
}

// TestQueueModelEnterOnNonParkedRowUnchanged is a regression guard for the
// parked early-return in handleQueueKey's "enter" case: with one epic parked
// and another selected and checked/launchable, enter must still open the
// implement-agent menu for the selected (non-parked) row instead of the
// parked check swallowing it.
func TestQueueModelEnterOnNonParkedRowUnchanged(t *testing.T) {
	root := t.TempDir()
	writeTicket(t, root, "alpha", "01-first.md", "Status: open\n\nBody.\n")
	writeTicket(t, root, "beta", "01-first.md", "Status: open\n\nBody.\n")
	checked := map[string]bool{
		ticketPath(root, "alpha", "01-first.md"): true,
		ticketPath(root, "beta", "01-first.md"):  true,
	}
	m := loadQueueModel(t, NewQueueModel(root, ui.Settings{}, checked, keys.Manager{}))
	m.parkedEpics["alpha"] = []ralphloop.StalledTicket{{Identifier: "01"}}

	rows := m.rows()
	betaIdx := -1
	for i, r := range rows {
		if r.epic.Name == "beta" {
			betaIdx = i
		}
	}
	if betaIdx == -1 {
		t.Fatalf("expected a beta row among %+v", rows)
	}
	m.selected = betaIdx

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(QueueModel)

	if !m.implementAgentMenuOpen {
		t.Fatal("expected enter on a non-parked row to still open the implement-agent menu")
	}
}

// TestQueueModelParkedEpicResumesToRunningRendering covers the last "What to
// build" bullet: once a parked epic resumes (LiveEventIterationStarted or
// LiveEventTicketReattached — see reduceLiveEvent), its row returns to
// running/queued rendering and it drops out of m.parkedEpics.
func TestQueueModelParkedEpicResumesToRunningRendering(t *testing.T) {
	root := t.TempDir()
	writeTicket(t, root, "alpha", "01-first.md", "Status: claimed\n\nBody.\n")
	checked := map[string]bool{ticketPath(root, "alpha", "01-first.md"): true}
	m := loadQueueModel(t, NewQueueModel(root, ui.Settings{}, checked, keys.Manager{}))

	r := newLoopRegistry(1)
	r.tryStart("alpha", 0, 1)
	r.reduceLiveEvent("alpha", ralphloop.LiveEvent{
		Kind:    ralphloop.LiveEventEpicParked,
		Stalled: []ralphloop.StalledTicket{{Identifier: "01"}},
	})
	previous := ralphLoopRegistry
	ralphLoopRegistry = r
	t.Cleanup(func() {
		r.finish("alpha", nil)
		ralphLoopRegistry = previous
	})

	updated, cmd := m.Update(implementPollMsg{epicName: "alpha"})
	m = updated.(QueueModel)
	m = deliverQueueCommands(t, m, cmd)

	if _, parked := m.parkedEpics["alpha"]; !parked {
		t.Fatalf("expected alpha reflected as parked, got %v", m.parkedEpics)
	}
	content := ansi.Strip(m.View().Content)
	if !strings.Contains(content, "parked") {
		t.Fatalf("expected parked rendering while stalled:\n%s", content)
	}

	r.reduceLiveEvent("alpha", ralphloop.LiveEvent{Kind: ralphloop.LiveEventIterationStarted, Identifier: "01", Label: "iter-01"})

	updated, cmd = m.Update(implementPollMsg{epicName: "alpha"})
	m = updated.(QueueModel)
	m = deliverQueueCommands(t, m, cmd)

	if _, parked := m.parkedEpics["alpha"]; parked {
		t.Fatalf("expected alpha to drop out of parkedEpics after resuming, got %v", m.parkedEpics)
	}
	if !m.runningEpics["alpha"] {
		t.Fatal("expected alpha reflected as running after resuming")
	}
	content = ansi.Strip(m.View().Content)
	if strings.Contains(content, "parked") {
		t.Fatalf("expected parked text gone after resuming:\n%s", content)
	}
}
