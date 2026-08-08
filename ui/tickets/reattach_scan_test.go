package tickets

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/herdr"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/keys"
	"github.com/elentok/gx/ui/nav"
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

func TestHandleReattachSignals_NoSignals_NoOp(t *testing.T) {
	root := t.TempDir()
	m := NewModel(root, ui.Settings{}, keys.New(nil))

	_, cmd := m.handleReattachSignals(reattachSignalsMsg{})
	if cmd != nil {
		t.Fatal("handleReattachSignals: want nil cmd when there are no signals")
	}
}
