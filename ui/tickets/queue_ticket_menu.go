package tickets

import (
	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/ui/notify"
	"github.com/elentok/gx/ui/terminalrun"
)

// handleQueueTicketMenuKey applies the "m" keymap: the Tickets tab's ticket
// Context menu plus the Queue group, or on an epic header the epic menu.
func (m QueueModel) handleQueueTicketMenuKey() (tea.Model, tea.Cmd) {
	if epic, ok := m.selectedQueueEpic(); ok {
		m.ticketMenu = openEpicContextMenu(m.ticketMenu, epicMenuInput{
			epic:         epic,
			branch:       epic.MergeTarget,
			queueable:    epicQueueable(epic, m.checkedPaths()),
			serverReason: serverReason(m.serverAPI, m.link),
			actions:      m.summaries[epic.Name].Actions,
		})
		return m, nil
	}
	r, ok := m.selectedQueueRow()
	if !ok {
		return m, notify.Info("select a ticket to see its actions")
	}
	in := newTicketMenuInput(r.epic, r.ticket, m.settings.HerdrUnavailable, serverReason(m.serverAPI, m.link))
	in.onQueue, in.hasProposal = true, hasPendingProposal(r.ticket.Body)
	m.ticketMenuPending = r.ticket.Path
	return m, cmdTicketMenu(m.serverAPI, in)
}

// selectedQueuePath is the selected ticket row's path, or "" off a ticket.
func (m QueueModel) selectedQueuePath() string {
	r, _ := m.selectedQueueRow()
	return r.ticket.Path
}

func (m QueueModel) handleTicketMenuReady(msg ticketMenuReadyMsg) (QueueModel, tea.Cmd) {
	pending := m.ticketMenuPending
	m.ticketMenuPending = ""
	if !ticketMenuReadyApplies(msg, pending, m.selectedQueuePath(), m.ModalOpen()) {
		return m, nil
	}
	m.ticketMenu = m.ticketMenu.Open(msg.target, msg.prompt, msg.items)
	return m, msg.toast()
}

// handleTicketMenuPick runs a pick only while its ticket is still the
// selected one; the status writes and the approve confirm work on the row.
func (m QueueModel) handleTicketMenuPick(msg ticketMenuPickMsg) (tea.Model, tea.Cmd) {
	r, ok := m.selectedQueueRow()
	if !ok || r.ticket.Path != msg.target.address {
		return m, nil
	}
	switch {
	case msg.status != "":
		return m, statusChangeCmd(m.serverAPI, m.settings.TicketStore, r.epic, r.ticket, msg.status)
	case msg.action == actionApprove:
		return m.handleServerApproveKey()
	}
	if cmd, ok := ticketMenuEditCopy(msg.action, m.yankQueueTicketSummary, m.yankQueueTicketFilePath, m.cmdEditSelectedFile); ok {
		return m, cmd
	}
	return m, dispatchMenuAction(m.serverAPI, m.worktreeRoot, m.settings, m.width, msg.target.result(msg.action))
}

// selectedQueueEpic is the selected epic header's epic; ok is false off one.
func (m QueueModel) selectedQueueEpic() (tickets.Epic, bool) {
	entries := m.queueTree.Entries()
	idx := m.queueTree.SelectedIndex()
	if idx < 0 || idx >= len(entries) || entries[idx].Value.kind != nodeEpicName {
		return tickets.Epic{}, false
	}
	return entries[idx].Value.epic, true
}

// handleQueueEpicMenuPick mirrors Model.handleEpicMenuPick.
func (m QueueModel) handleQueueEpicMenuPick(msg epicMenuPickMsg) (tea.Model, tea.Cmd) {
	epic, ok := m.selectedQueueEpic()
	if !ok || epic.Path != msg.target.address {
		return m, nil
	}
	if msg.status != "" {
		return m, cmdApplyEpicStatus(m.serverAPI, m.settings.TicketStore, epic, msg.status)
	}
	switch msg.action {
	case actionQueueEpic:
		return m, cmdQueueEpic(m.serverAPI, epic.Path)
	case actionEditEpic:
		return m, m.cmdEditSelectedFile(terminalrun.InPlace)
	case actionYankEpic:
		return m, m.yankQueueTicketSummary()
	}
	return m, dispatchMenuAction(m.serverAPI, m.worktreeRoot, m.settings, m.width, msg.target.result(msg.action))
}
