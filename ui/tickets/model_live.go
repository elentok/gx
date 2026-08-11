package tickets

import (
	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/ui/notify"
)

// syncRunSnapshot replaces presentation state atomically from the registry's
// durable projection; it never reads the producer stream. The returned cmd
// closes any reattach-scan notification the registry queued up while this
// tab wasn't polling (see epicRun.pendingNotifyCloses) — nil if none.
func (m *Model) syncRunSnapshot(epicName string) tea.Cmd {
	snapshot, ok := ralphLoopRegistry.runSnapshot(epicName)
	if !ok {
		return nil
	}
	if m.live == nil {
		m.live = map[string]map[string]liveTicketState{}
	}
	if m.labelIdentifier == nil {
		m.labelIdentifier = map[string]map[string]string{}
	}
	live := projectLiveTickets(snapshot)
	labels := make(map[string]string, len(snapshot.Tickets))
	for identifier, ticket := range snapshot.Tickets {
		labels[ticket.Label] = identifier
	}
	m.live[epicName] = live
	m.labelIdentifier[epicName] = labels
	if m.implementingEpics == nil {
		m.implementingEpics = map[string]bool{}
	}
	m.implementingEpics[epicName] = snapshot.State == RunStateRunning
	var reloadCmd tea.Cmd
	if ralphLoopRegistry.drainPendingReload(epicName) {
		reloadCmd = m.cmdLoad()
	}
	return tea.Batch(
		closeNotifyCmd(ralphLoopRegistry.drainPendingNotifyCloses(epicName)),
		toastNotifyCmd(ralphLoopRegistry.drainPendingToasts(epicName)),
		reloadCmd,
	)
}

// closeNotifyCmd batches a notify.Close cmd per id, or nil if ids is empty.
func closeNotifyCmd(ids []string) tea.Cmd {
	if len(ids) == 0 {
		return nil
	}
	cmds := make([]tea.Cmd, len(ids))
	for i, id := range ids {
		cmds[i] = notify.Close(id)
	}
	return tea.Batch(cmds...)
}

// toastNotifyCmd batches a notify cmd per queued toast (see
// epicRun.pendingToasts), or nil if toasts is empty.
func toastNotifyCmd(toasts []notify.NotifyMsg) tea.Cmd {
	if len(toasts) == 0 {
		return nil
	}
	cmds := make([]tea.Cmd, len(toasts))
	for i, msg := range toasts {
		cmds[i] = func() tea.Msg { return msg }
	}
	return tea.Batch(cmds...)
}

// clearLiveTracking resets m.implementEpic's live-orchestrator state — kept
// as this zero-arg convenience for call sites/tests that only ever track one
// epic at a time; a poll/sync learning of a *specific* epic's finish (which
// might not be m.implementEpic once more than one epic is tracked, ticket 05)
// goes through clearLiveTrackingFor(epicName) directly so it can't wipe a
// different, still-running epic's live state.
func (m *Model) clearLiveTracking() {
	m.clearLiveTrackingFor(m.implementEpic)
}

// clearLiveTrackingFor removes epicName's live-orchestrator state once its
// run has finished (or once this Model learns of a finish it missed while
// backgrounded — see handleImplementSync), reverting that epic's ticket/epic
// rendering to the normal disk-based view.
func (m *Model) clearLiveTrackingFor(epicName string) {
	delete(m.live, epicName)
	delete(m.labelIdentifier, epicName)
	delete(m.implementingEpics, epicName)
	if m.implementEpic == epicName {
		m.implementEpic = ""
	}
}
