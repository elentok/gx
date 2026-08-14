package tickets

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/herdr"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/keys"
	"github.com/elentok/gx/ui/nav"
	"github.com/elentok/gx/ui/notify"
)

// withFakeReattachHerdr swaps reattachFindWorkspace/reattachTabList for the
// duration of the test, restoring the real herdr-backed funcs after — so
// cmdReattachScan never shells out to a real herdr process in tests.
func withFakeReattachHerdr(t *testing.T, findWorkspace func(string) (string, error), tabList func(string) ([]herdr.Tab, error)) {
	t.Helper()
	prevFind, prevTabs := reattachFindWorkspace, reattachTabList
	reattachFindWorkspace, reattachTabList = findWorkspace, tabList
	t.Cleanup(func() {
		reattachFindWorkspace, reattachTabList = prevFind, prevTabs
	})
}

func TestCmdReattachScan_FirstActivation_ScansAndSecondIsNoOp(t *testing.T) {
	// not parallel-safe: reassigns the package-level reattachFindWorkspace/reattachTabList/reattachScanOnce singletons
	resetReattachScanOnce()
	t.Cleanup(resetReattachScanOnce)

	root := t.TempDir()
	writeTicket(t, root, "epic", "01-first.md", "Status: claimed\n\nBody.\n")

	calls := 0
	withFakeReattachHerdr(t, func(label string) (string, error) {
		calls++
		return "ws1", nil
	}, func(workspaceID string) ([]herdr.Tab, error) {
		return []herdr.Tab{{Label: "epic-iter-01"}}, nil
	})

	m := NewModel(root, ui.Settings{}, keys.New(nil))

	first := m.cmdReattachScan()
	if first == nil {
		t.Fatal("cmdReattachScan: want a scan cmd on first activation")
	}
	msg, ok := first().(reattachSignalsMsg)
	if !ok {
		t.Fatalf("cmdReattachScan(): got %#v, want reattachSignalsMsg", first())
	}
	if len(msg.signals) != 1 || msg.signals[0].EpicName != "epic" {
		t.Fatalf("signals = %+v, want one signal for epic", msg.signals)
	}
	if calls != 1 {
		t.Fatalf("findWorkspace calls = %d, want 1", calls)
	}

	second := m.cmdReattachScan()
	if second != nil {
		t.Fatal("cmdReattachScan: want nil on second activation within the same process")
	}
	if calls != 1 {
		t.Fatalf("findWorkspace calls after second activation = %d, want still 1 (no re-scan)", calls)
	}
}

func TestCmdReattachScan_FreshProcess_ReScans(t *testing.T) {
	// not parallel-safe: reassigns the package-level reattachFindWorkspace/reattachTabList/reattachScanOnce singletons
	resetReattachScanOnce()
	t.Cleanup(resetReattachScanOnce)

	root := t.TempDir()
	writeTicket(t, root, "epic", "01-first.md", "Status: claimed\n\nBody.\n")
	withFakeReattachHerdr(t, func(label string) (string, error) {
		return "ws1", nil
	}, func(workspaceID string) ([]herdr.Tab, error) {
		return []herdr.Tab{{Label: "iter-01"}}, nil
	})

	m := NewModel(root, ui.Settings{}, keys.New(nil))
	if cmd := m.cmdReattachScan(); cmd == nil {
		t.Fatal("cmdReattachScan: want a scan cmd on first activation")
	}

	// A fresh process re-trips the guard.
	resetReattachScanOnce()
	if cmd := m.cmdReattachScan(); cmd == nil {
		t.Fatal("cmdReattachScan: want a scan cmd again after a fresh-process reset")
	}
}

func TestHandleReattachSignals_NoTicketOrProcessStateMutated(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeTicket(t, root, "epic", "01-first.md", "Status: claimed\n\nBody.\n")
	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)

	before := m.epics[0].Tickets[0].Status
	updated, cmd := m.handleReattachSignals(reattachSignalsMsg{signals: []ralphloop.ReattachSignal{
		{EpicName: "epic", Ticket: m.epics[0].Tickets[0]},
	}})
	nm := updated.(Model)

	if nm.epics[0].Tickets[0].Status != before {
		t.Fatalf("ticket status = %q, want unchanged %q", nm.epics[0].Tickets[0].Status, before)
	}
	if cmd == nil {
		t.Fatal("handleReattachSignals: want a notify cmd (the detect-only indicator)")
	}
}

func TestHandleReattachSignals_NeverSwitchesToQueueTab(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeTicket(t, root, "epic", "01-first.md", "Status: claimed\n\nBody.\n")
	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)

	_, cmd := m.handleReattachSignals(reattachSignalsMsg{signals: []ralphloop.ReattachSignal{
		{EpicName: "epic", Ticket: m.epics[0].Tickets[0]},
	}})
	if cmd == nil {
		t.Fatal("handleReattachSignals: want a cmd when a signal is found")
	}

	if batchContainsQueueSwitch(cmd) {
		t.Fatal("handleReattachSignals: want no nav.Switch to TabQueue among the returned cmds")
	}
}

// batchContainsQueueSwitch recursively unwraps a (possibly batched) tea.Cmd
// looking for a nav.Switch message targeting TabQueue.
func batchContainsQueueSwitch(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, c := range batch {
			if batchContainsQueueSwitch(c) {
				return true
			}
		}
		return false
	}
	vs, ok := nav.IsSwitch(msg)
	return ok && vs.Tab == nav.TabQueue
}

// TestHandleReattachSignals_NotificationPersistsUntilRescanClears verifies
// (ticket 12) that the "recoverable session detected" notification stays up
// across multiple rescans that still find the session live, and only clears
// once a rescan no longer reports it — not merely that its Kind differs from
// KindProgress (that was ticket 04's over-corrected fix: a fixed 5s TTL that
// could expire before anyone looked at the screen).
func TestHandleReattachSignals_NotificationPersistsUntilRescanClears(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeTicket(t, root, "epic", "01-first.md", "Status: claimed\n\nBody.\n")
	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)

	signal := ralphloop.ReattachSignal{EpicName: "epic", Ticket: m.epics[0].Tickets[0]}
	updated, cmd := m.handleReattachSignals(reattachSignalsMsg{signals: []ralphloop.ReattachSignal{signal}})
	m = updated.(Model)
	if cmd == nil {
		t.Fatal("handleReattachSignals: want a notify cmd when a signal is found")
	}

	id := reattachNotifyID("epic", m.epics[0].Tickets[0].Identifier)
	n, ok := findNotifyMsg(cmd, id)
	if !ok {
		t.Fatalf("handleReattachSignals: want a NotifyMsg for %q among returned cmds", id)
	}
	if n.Kind != notify.KindProgress {
		t.Fatalf("handleReattachSignals: notification for %q kind = %v, want KindProgress (explicit-close, no fixed TTL)", id, n.Kind)
	}

	// A rescan that still finds the session live must not close it — across
	// several ticks/renders, not just once.
	for i := range 3 {
		updated, rescanCmd := m.handleReattachRescan(reattachRescanMsg{signals: []ralphloop.ReattachSignal{signal}})
		m = updated.(Model)
		if findCloseMsg(rescanCmd, id) {
			t.Fatalf("handleReattachRescan (still live, iteration %d): notification %q closed, want it to remain", i, id)
		}
	}
	if len(m.reattachPending) != 1 {
		t.Fatalf("reattachPending = %d entries, want 1 while still live", len(m.reattachPending))
	}

	// A rescan that no longer finds the session clears it.
	updated, rescanCmd := m.handleReattachRescan(reattachRescanMsg{})
	m = updated.(Model)
	if rescanCmd == nil {
		t.Fatal("handleReattachRescan: want a close cmd once the session is no longer found")
	}
	if !findCloseMsg(rescanCmd, id) {
		t.Fatalf("handleReattachRescan: want a CloseMsg for %q once the scan no longer finds it", id)
	}
	if len(m.reattachPending) != 0 {
		t.Fatalf("reattachPending = %d entries, want 0 after the clearing scan", len(m.reattachPending))
	}
}

// TestCmdReattachRescan_NoPending_NoOp verifies cmdReattachRescan skips the
// scan entirely (nil cmd) when nothing is currently pending, so
// OnPageActivated doesn't shell out to herdr on every tab focus once
// notifications have already cleared.
func TestCmdReattachRescan_NoPending_NoOp(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	m := NewModel(root, ui.Settings{}, keys.New(nil))

	calls := 0
	withFakeReattachHerdr(t, func(label string) (string, error) {
		calls++
		return "ws1", nil
	}, func(workspaceID string) ([]herdr.Tab, error) {
		return nil, nil
	})

	if cmd := m.cmdReattachRescan(); cmd != nil {
		t.Fatal("cmdReattachRescan: want nil cmd when reattachPending is empty")
	}
	if calls != 0 {
		t.Fatalf("findWorkspace calls = %d, want 0 (no pending notifications to rescan)", calls)
	}
}

// findNotifyMsg recursively unwraps a (possibly batched) tea.Cmd looking for
// a notify.NotifyMsg with the given id.
func findNotifyMsg(cmd tea.Cmd, id string) (notify.NotifyMsg, bool) {
	if cmd == nil {
		return notify.NotifyMsg{}, false
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, c := range batch {
			if n, ok := findNotifyMsg(c, id); ok {
				return n, true
			}
		}
		return notify.NotifyMsg{}, false
	}
	n, ok := msg.(notify.NotifyMsg)
	if ok && n.ID == id {
		return n, true
	}
	return notify.NotifyMsg{}, false
}

func TestHandleReattachSignals_NoSignals_NoOp(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	m := NewModel(root, ui.Settings{}, keys.New(nil))

	_, cmd := m.handleReattachSignals(reattachSignalsMsg{})
	if cmd != nil {
		t.Fatal("handleReattachSignals: want nil cmd when there are no signals")
	}
}
