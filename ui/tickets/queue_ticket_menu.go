package tickets

import (
	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/ui/notify"
)

// handleQueueTicketMenuKey applies the "m" keymap: the Tickets tab's ticket
// Context menu plus the Queue group.
func (m QueueModel) handleQueueTicketMenuKey() (tea.Model, tea.Cmd) {
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

func (m QueueModel) handleTicketMenuReady(msg ticketMenuReadyMsg) QueueModel {
	pending := m.ticketMenuPending
	m.ticketMenuPending = ""
	if ticketMenuReadyApplies(msg, pending, m.selectedQueuePath(), m.ModalOpen()) {
		m.ticketMenu = m.ticketMenu.Open(msg.target, msg.prompt, msg.items)
	}
	return m
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
