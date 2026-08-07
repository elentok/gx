package tickets

import (
	"fmt"
	"sync"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/herdr"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/ui/notify"
)

// reattachFindWorkspace/reattachTabList are package-level so tests can swap
// in fakes (see ralphLoopRegistry for the same swap-and-restore pattern
// elsewhere in this package) instead of cmdReattachScan shelling out to a
// real herdr process.
var (
	reattachFindWorkspace = herdr.FindWorkspace
	reattachTabList       = herdr.TabList
)

// reattachScanOnce guards cmdReattachScan against running more than once per
// gx process: OnPageActivated fires every time the Tickets tab (re)gains
// focus, but the restart-recovery scan it answers only has anything new to
// find right after a crash/restart — a live herdr pane list on every tab
// switch thereafter would be wasted work for a signal that can't have
// changed on its own.
var reattachScanOnce sync.Once

// resetReattachScanOnce is a test seam simulating a fresh process, since
// package-level state otherwise only trips once for the whole test binary.
func resetReattachScanOnce() {
	reattachScanOnce = sync.Once{}
}

// reattachSignalsMsg carries this process's one-time restart-recovery scan
// result back into Update.
type reattachSignalsMsg struct {
	signals []ralphloop.ReattachSignal
}

// cmdReattachScan runs ralphloop.ScanForReattachable across every epic in
// m.scratchDir() at most once per process (see reattachScanOnce). Returns
// nil on every call after the first.
func (m Model) cmdReattachScan() tea.Cmd {
	var cmd tea.Cmd
	reattachScanOnce.Do(func() {
		scratchDir := m.scratchDir()
		cmd = func() tea.Msg {
			epics, err := tickets.Load(scratchDir)
			if err != nil {
				return nil
			}
			signals, err := ralphloop.ScanForReattachable(reattachFindWorkspace, reattachTabList, epics)
			if err != nil {
				return nil
			}
			return reattachSignalsMsg{signals: signals}
		}
	})
	return cmd
}

// handleReattachSignals surfaces each detect-only ReattachSignal as a
// persistent progress-style (spinner) notification, keyed so a ticket
// already signaled doesn't duplicate, and switches the active tab to Queue
// so the recoverable work is immediately visible rather than left for the
// user to stumble onto. This is otherwise detect-only: no ticket state is
// touched and nothing is auto-resumed, matching ScanForReattachable's own
// contract.
func (m Model) handleReattachSignals(msg reattachSignalsMsg) (tea.Model, tea.Cmd) {
	if len(msg.signals) == 0 {
		return m, nil
	}
	cmds := make([]tea.Cmd, 0, len(msg.signals)+1)
	for _, s := range msg.signals {
		id := reattachNotifyID(s.EpicName, s.Ticket.Identifier)
		cmds = append(cmds, notify.Progress(id, fmt.Sprintf(
			"epic %q ticket %s: recoverable session detected", s.EpicName, s.Ticket.Identifier,
		)))
	}
	cmds = append(cmds, cmdOpenQueueTab(m.worktreeRoot))
	return m, tea.Batch(cmds...)
}

// reattachNotifyID is the notify.Progress/notify.Close id for a signaled
// ticket's "recoverable session detected" notification — shared by
// handleReattachSignals (which opens it) and loopRegistry.reduceLiveEvent
// (which closes it once that ticket's session is actually reattached or
// resumed), so the two ends can never drift out of sync.
func reattachNotifyID(epicName, identifier string) string {
	return "reattach-scan-" + epicName + "-" + identifier
}
