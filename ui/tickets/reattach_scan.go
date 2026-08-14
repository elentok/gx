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

// reattachRescanMsg carries the result of a cmdReattachRescan pass — every
// signal currently found, used only to check which of m.reattachPending are
// still live (see handleReattachRescan). Unlike reattachSignalsMsg it is not
// itself a source of new notifications.
type reattachRescanMsg struct {
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

// cmdReattachRescan re-checks every currently-pending "recoverable session
// detected" notification (m.reattachPending) against a fresh scan — unlike
// cmdReattachScan it is not gated by reattachScanOnce and is safe to call on
// every OnPageActivated: it's how a notification opened by
// handleReattachSignals eventually clears once the session it points at is
// no longer live, without needing loopRegistry.reduceLiveEvent to ever fire
// (ticket 04's fix, preserved — see handleReattachRescan). A no-op (nil cmd)
// once nothing is pending.
func (m Model) cmdReattachRescan() tea.Cmd {
	if len(m.reattachPending) == 0 {
		return nil
	}
	scratchDir := m.scratchDir()
	return func() tea.Msg {
		epics, err := tickets.Load(scratchDir)
		if err != nil {
			return nil
		}
		signals, err := ralphloop.ScanForReattachable(reattachFindWorkspace, reattachTabList, epics)
		if err != nil {
			return nil
		}
		return reattachRescanMsg{signals: signals}
	}
}

// handleReattachSignals surfaces each detect-only ReattachSignal as a
// persistent (notify.KindProgress) notification, keyed so a ticket already
// signaled doesn't duplicate, and records it in m.reattachPending so a later
// cmdReattachRescan can tell when it's safe to clear (handleReattachRescan).
// KindProgress alone would leave the notification stuck forever if
// loopRegistry.reduceLiveEvent's Close never fires (e.g. an epic with no
// active run) — cmdReattachRescan is the self-clearing half that doesn't
// depend on it, restoring ticket 04's guarantee alongside ticket 12's
// persist-until-cleared requirement. This is detect-only: no ticket state is
// touched and nothing is auto-resumed or auto-navigated to, matching
// ScanForReattachable's own contract — resuming happens only via the Queue
// tab's Detached+Live confirmation (queue_reattach.go).
func (m Model) handleReattachSignals(msg reattachSignalsMsg) (tea.Model, tea.Cmd) {
	if len(msg.signals) == 0 {
		return m, nil
	}
	cmds := make([]tea.Cmd, 0, len(msg.signals))
	for _, s := range msg.signals {
		id := reattachNotifyID(s.EpicName, s.Ticket.Identifier)
		message := fmt.Sprintf("epic %q ticket %s: recoverable session detected", s.EpicName, s.Ticket.Identifier)
		cmds = append(cmds, func() tea.Msg {
			return notify.NotifyMsg{ID: id, Kind: notify.KindProgress, Message: message}
		})
		m.reattachPending = append(m.reattachPending, s)
	}
	return m, tea.Batch(cmds...)
}

// handleReattachRescan closes the notification for any m.reattachPending
// entry that msg's fresh scan no longer reports live, and drops it from
// m.reattachPending — see cmdReattachRescan.
func (m Model) handleReattachRescan(msg reattachRescanMsg) (tea.Model, tea.Cmd) {
	stillLive := make(map[string]bool, len(msg.signals))
	for _, s := range msg.signals {
		stillLive[reattachNotifyID(s.EpicName, s.Ticket.Identifier)] = true
	}

	var cmds []tea.Cmd
	remaining := m.reattachPending[:0]
	for _, s := range m.reattachPending {
		id := reattachNotifyID(s.EpicName, s.Ticket.Identifier)
		if stillLive[id] {
			remaining = append(remaining, s)
			continue
		}
		cmds = append(cmds, func() tea.Msg {
			return notify.CloseMsg{ID: id}
		})
	}
	m.reattachPending = remaining

	if len(cmds) == 0 {
		return m, nil
	}
	return m, tea.Batch(cmds...)
}

// reattachNotifyID is the notification id for a signaled ticket's
// "recoverable session detected" notification — shared by
// handleReattachSignals (which opens it) and loopRegistry.reduceLiveEvent
// (which closes it early once that ticket's session is actually reattached
// or resumed), so the two ends can never drift out of sync.
func reattachNotifyID(epicName, identifier string) string {
	return "reattach-scan-" + epicName + "-" + identifier
}
