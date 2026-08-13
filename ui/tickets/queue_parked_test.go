package tickets

import (
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/keys"
)

// installParkedRegistry swaps in a loop registry whose named epics are parked
// on the given stalled tickets. The Queue tab derives parked-ness from run
// snapshots (ticket 13c), so a parked epic is set up by driving the registry
// rather than by writing model state.
func installParkedRegistry(t *testing.T, parked map[string][]ralphloop.StalledTicket) *loopRegistry {
	t.Helper()
	r := newLoopRegistry(max(len(parked), 1))
	for name, stalled := range parked {
		r.tryStart(name, 0, 1)
		r.reduceLiveEvent(name, ralphloop.LiveEvent{Kind: ralphloop.LiveEventEpicParked, Stalled: stalled})
	}
	previous := ralphLoopRegistry
	ralphLoopRegistry = r
	t.Cleanup(func() {
		for name := range parked {
			r.finish(name, nil)
		}
		ralphLoopRegistry = previous
	})
	return r
}

// TestQueueRunStateAndTitleReflectParkedEpics covers 11a5's queueRunState/
// queueHeaderTitle parked-aware branches: parked wins over a concurrently
// running epic (park is tracked per run, independently of m.runningEpics, per
// queueRunState's doc comment), and the title names the lowest epic name's
// lowest ticket identifier deterministically.
func TestQueueRunStateAndTitleReflectParkedEpics(t *testing.T) {
	// not parallel-safe: installParkedRegistry reassigns the package-level ralphLoopRegistry singleton
	root := t.TempDir()
	writeTicket(t, root, "alpha", "01-first.md", "Status: open\n\nBody.\n")
	writeTicket(t, root, "beta", "01-first.md", "Status: open\n\nBody.\n")
	checked := map[string]bool{
		ticketPath(root, "alpha", "01-first.md"): true,
		ticketPath(root, "beta", "01-first.md"):  true,
	}
	m := loadQueueModel(t, NewQueueModel(root, ui.Settings{}, checked, keys.Manager{}))

	installParkedRegistry(t, map[string][]ralphloop.StalledTicket{
		"beta":  {{Identifier: "02"}},
		"alpha": {{Identifier: "01"}},
	})
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
	want := []string{""}
	if lines := m.queueHeaderBodyLines(); !slices.Equal(lines, want) {
		t.Fatalf("queueHeaderBodyLines() = %v, want %v for parked (title already carries the detail)", lines, want)
	}
}

// TestQueueModelEnterOnParkedRowResumesEvenWhenCheckedAndLaunchable covers
// the priority case 11a4's "enter" branch was written to resolve: a parked
// epic's row still resumes (Gate.WakeParked) instead of opening the
// implement-agent menu, even though its ticket is checked and would
// otherwise be launchable.
func TestQueueModelEnterOnParkedRowResumesEvenWhenCheckedAndLaunchable(t *testing.T) {
	// not parallel-safe: reassigns the package-level ralphLoopRegistry singleton
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

	wake := r.gateFor("alpha").ParkWake()

	m.View() // populate m.queueTree.Entries()
	m = selectFirstQueueTicketRow(t, m)

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
	// not parallel-safe: installParkedRegistry reassigns the package-level ralphLoopRegistry singleton
	root := t.TempDir()
	writeTicket(t, root, "alpha", "01-first.md", "Status: open\n\nBody.\n")
	writeTicket(t, root, "beta", "01-first.md", "Status: open\n\nBody.\n")
	checked := map[string]bool{
		ticketPath(root, "alpha", "01-first.md"): true,
		ticketPath(root, "beta", "01-first.md"):  true,
	}
	m := loadQueueModel(t, NewQueueModel(root, ui.Settings{}, checked, keys.Manager{}))
	installParkedRegistry(t, map[string][]ralphloop.StalledTicket{"alpha": {{Identifier: "01"}}})

	m.View() // populate m.queueTree.Entries() (queue_view.go's View())
	betaIdx := -1
	for i, e := range m.queueTree.Entries() {
		if e.Value.kind == nodeQueueTicket && e.Value.ticket.epic.Name == "beta" {
			betaIdx = i
		}
	}
	if betaIdx == -1 {
		t.Fatalf("expected a beta row among %+v", m.queueTree.Entries())
	}
	m.queueTree.SetSelectedIndex(betaIdx)

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(QueueModel)

	if !m.implementAgentMenuOpen {
		t.Fatal("expected enter on a non-parked row to still open the implement-agent menu")
	}
}

// TestQueueModelParkedEpicResumesToRunningRendering covers the last "What to
// build" bullet: once a parked epic resumes (LiveEventIterationStarted or
// LiveEventTicketReattached — see reduceLiveEvent), its row returns to
// running/queued rendering and stops being reported as parked.
func TestQueueModelParkedEpicResumesToRunningRendering(t *testing.T) {
	// not parallel-safe: reassigns the package-level ralphLoopRegistry singleton
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

	if _, parked := r.parkedStalledFor("alpha"); !parked {
		t.Fatal("expected alpha reflected as parked")
	}
	content := ansi.Strip(m.View().Content)
	if !strings.Contains(content, "parked") {
		t.Fatalf("expected parked rendering while stalled:\n%s", content)
	}

	r.reduceLiveEvent("alpha", ralphloop.LiveEvent{Kind: ralphloop.LiveEventIterationStarted, Identifier: "01", Label: "iter-01"})

	updated, cmd = m.Update(implementPollMsg{epicName: "alpha"})
	m = updated.(QueueModel)
	m = deliverQueueCommands(t, m, cmd)

	if _, parked := r.parkedStalledFor("alpha"); parked {
		t.Fatal("expected alpha to stop being reported as parked after resuming")
	}
	if !m.runningEpics["alpha"] {
		t.Fatal("expected alpha reflected as running after resuming")
	}
	content = ansi.Strip(m.View().Content)
	if strings.Contains(content, "parked") {
		t.Fatalf("expected parked text gone after resuming:\n%s", content)
	}
}
