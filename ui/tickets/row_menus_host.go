package tickets

import (
	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/ui/contextmenu"
	"github.com/elentok/gx/ui/terminalrun"
	"github.com/elentok/gx/ui/tickets/rowactions"
)

// The root half of rowMenus: each tab model adapts itself to menuTab and
// forwards its Update cases. Nothing here belongs with the menus, which never
// see Model or QueueModel.

// modelMenuTab and queueMenuTab adapt a tab model to menuTab.
type modelMenuTab struct{ m Model }

type queueMenuTab struct{ m QueueModel }

func (m Model) menuEnv() menuEnv {
	return menuEnv{
		serverAPI: m.serverAPI, admin: m.admin, reads: m.reads, sessions: m.sessions,
		tickets: m.tickets, queue: m.queue, link: m.link, settings: m.settings,
		width: m.width, worktreeRoot: m.worktreeRoot,
	}
}

func (m QueueModel) menuEnv() menuEnv {
	return menuEnv{
		serverAPI: m.serverAPI, admin: m.admin, reads: m.reads, sessions: m.sessions,
		tickets: m.tickets, queue: m.queue, link: m.link, settings: m.settings,
		width: m.width, worktreeRoot: m.worktreeRoot,
	}
}

func (t modelMenuTab) env() menuEnv                             { return t.m.menuEnv() }
func (t modelMenuTab) modalOpen() bool                          { return t.m.ModalOpen() }
func (t modelMenuTab) yankSummary() tea.Cmd                     { return t.m.yankTicketSummary() }
func (t modelMenuTab) yankFilePath() tea.Cmd                    { return t.m.yankTicketFilePath() }
func (t modelMenuTab) editFile(s terminalrun.SplitType) tea.Cmd { return t.m.cmdEditSelectedFile(s) }

func (t modelMenuTab) selection() menuSelection {
	m := t.m
	r, ok := m.selectedRow()
	if !ok {
		return menuSelection{}
	}
	epic := m.epicAt(r)
	if r.isEpic() {
		return menuSelection{
			ok: true, isEpic: true, epic: epic, archived: r.archived,
			session:   rowactions.EpicSession(m.vm.Sessions, epic.Path),
			queueable: epicQueueable(epic, m.vm.Queue),
			actions:   epicSummaryActions(m.vm.Epics, epic.Name),
		}
	}
	ticket := epic.Tickets[r.ticketIdx]
	return menuSelection{
		ok: true, epic: epic, ticket: ticket, archived: r.archived,
		session:    rowactions.TicketSession(m.vm.Sessions, epic.Path, ticket),
		queueAgent: rowactions.QueueAgentOf(m.vm.Sessions, ticket.Path),
	}
}

func (t queueMenuTab) env() menuEnv                             { return t.m.menuEnv() }
func (t queueMenuTab) modalOpen() bool                          { return t.m.ModalOpen() }
func (t queueMenuTab) yankSummary() tea.Cmd                     { return t.m.yankQueueTicketSummary() }
func (t queueMenuTab) yankFilePath() tea.Cmd                    { return t.m.yankQueueTicketFilePath() }
func (t queueMenuTab) editFile(s terminalrun.SplitType) tea.Cmd { return t.m.cmdEditSelectedFile(s) }

func (t queueMenuTab) selection() menuSelection {
	m := t.m
	if epic, ok := m.selectedQueueEpic(); ok {
		return menuSelection{
			ok: true, isEpic: true, epic: epic,
			queueable: epicQueueable(epic, m.checkedPaths()),
			actions:   m.summaries[epic.Name].Actions,
		}
	}
	r, ok := m.selectedQueueRow()
	if !ok {
		return menuSelection{}
	}
	return menuSelection{ok: true, epic: r.epic, ticket: r.ticket}
}

// handleSuggestedActionsKey applies the "m" keymap.
func (m Model) handleSuggestedActionsKey() (tea.Model, tea.Cmd) {
	cmd := m.menus.handleKey(modelMenuTab{m})
	return m, cmd
}

func (m Model) handleTicketMenuReady(msg ticketMenuReadyMsg) (Model, tea.Cmd) {
	cmd := m.menus.handleReady(msg, modelMenuTab{m})
	return m, cmd
}

func (m Model) handleTicketMenuPick(msg ticketMenuPickMsg) (tea.Model, tea.Cmd) {
	return m, m.menus.handleTicketPick(msg, modelMenuTab{m}).cmd
}

func (m Model) handleEpicMenuPick(msg epicMenuPickMsg) (tea.Model, tea.Cmd) {
	return m, m.menus.handleEpicPick(msg, modelMenuTab{m})
}

func (m Model) handleServerResumeEnter() (cmd tea.Cmd, handled bool) {
	return resumeEnter(modelMenuTab{m})
}

func (m Model) rowHasSuggestedActions(epic tickets.Epic, ticket tickets.Ticket, archived bool) bool {
	return m.menus.hasSuggestedActions(m.menuEnv(), epic, ticket, archived)
}

// selectedTicketPath is the selected ticket row's path, or "" off a ticket.
func (m Model) selectedTicketPath() string {
	return modelMenuTab{m}.selection().ticketPath()
}

// menuAnchor is the selected sidebar row on screen, under the panel's
// header line, for the Context menu to open next to.
func (m Model) menuAnchor() contextmenu.Anchor {
	line, h, ok := m.sidebarTree.SelectedBodyLine()
	if !ok {
		return contextmenu.Anchor{}
	}
	return contextmenu.Anchor{X: 1, Y: 1 + line, H: h}
}

// handleStatusChanged applies statusChangedMsg. Up mode does nothing: the
// store watch turns the file write into a ticket-changed event. Down mode has
// no stream, so it reloads from disk.
func (m Model) handleStatusChanged() (tea.Model, tea.Cmd) {
	if m.readsDisk() {
		return m, m.cmdLoadDisk()
	}
	return m, nil
}

func (m Model) handleConfirmUpdate(msg tea.Msg) (tea.Model, tea.Cmd) {
	next, cmd, _ := m.confirm.Update(msg)
	m.confirm = next
	return m, cmd
}

func (m Model) handleConfirmMouseUpdate(msg tea.MouseClickMsg) (tea.Model, tea.Cmd) {
	next, cmd, _ := m.confirm.UpdateMouse(msg, m.width, m.width, m.height)
	m.confirm = next
	return m, cmd
}

// handleQueueTicketMenuKey applies the "m" keymap.
func (m QueueModel) handleQueueTicketMenuKey() (tea.Model, tea.Cmd) {
	cmd := m.menus.handleKey(queueMenuTab{m})
	return m, cmd
}

// selectedQueuePath is the selected ticket row's path, or "" off a ticket.
func (m QueueModel) selectedQueuePath() string {
	r, _ := m.selectedQueueRow()
	return r.ticket.Path
}

func (m QueueModel) handleTicketMenuReady(msg ticketMenuReadyMsg) (QueueModel, tea.Cmd) {
	cmd := m.menus.handleReady(msg, queueMenuTab{m})
	return m, cmd
}

func (m QueueModel) handleTicketMenuPick(msg ticketMenuPickMsg) (tea.Model, tea.Cmd) {
	res := m.menus.handleTicketPick(msg, queueMenuTab{m})
	if res.approve {
		return m.handleServerApproveKey()
	}
	return m, res.cmd
}

func (m QueueModel) handleQueueEpicMenuPick(msg epicMenuPickMsg) (tea.Model, tea.Cmd) {
	return m, m.menus.handleEpicPick(msg, queueMenuTab{m})
}

func (m QueueModel) rowHasSuggestedActions(epic tickets.Epic, ticket tickets.Ticket) bool {
	return m.menus.hasSuggestedActions(m.menuEnv(), epic, ticket, false)
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

// menuAnchor is the selected queue row on screen, under the panel's header
// line and the queue header, for the Context menu to open next to.
func (m QueueModel) menuAnchor() contextmenu.Anchor {
	line, h, ok := m.queueTree.SelectedBodyLine()
	if !ok {
		return contextmenu.Anchor{}
	}
	return contextmenu.Anchor{X: 1, Y: 1 + queueHeaderReservedLines + line, H: h}
}
