package tickets

import (
	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/ui/contextmenu"
	"github.com/elentok/gx/ui/terminalrun"
	"github.com/elentok/gx/ui/tickets/rowactions"
)

// The root half of rowactions.Menus: each tab model adapts itself to
// rowactions.Tab and forwards its Update cases. Nothing here belongs with the
// menus, which never see Model or QueueModel.

// modelMenuTab and queueMenuTab adapt a tab model to rowactions.Tab.
type modelMenuTab struct{ m Model }

type queueMenuTab struct{ m QueueModel }

func (m Model) menuEnv() rowactions.Env {
	return rowactions.Env{
		ServerAPI: m.serverAPI, Admin: m.admin, Reads: m.reads, Sessions: m.sessions,
		Tickets: m.tickets, Queue: m.queue, Link: m.link, Settings: m.settings,
		Width: m.width, WorktreeRoot: m.worktreeRoot,
	}
}

func (m QueueModel) menuEnv() rowactions.Env {
	return rowactions.Env{
		ServerAPI: m.server.serverAPI, Admin: m.srv.admin, Reads: m.srv.reads, Sessions: m.sessions,
		Tickets: m.srv.tickets, Queue: m.srv.queue, Link: m.server.link, Settings: m.settings,
		Width: m.width, WorktreeRoot: m.worktreeRoot,
	}
}

func (t modelMenuTab) Env() rowactions.Env                      { return t.m.menuEnv() }
func (t modelMenuTab) ModalOpen() bool                          { return t.m.ModalOpen() }
func (t modelMenuTab) YankSummary() tea.Cmd                     { return t.m.yankTicketSummary() }
func (t modelMenuTab) YankFilePath() tea.Cmd                    { return t.m.yankTicketFilePath() }
func (t modelMenuTab) EditFile(s terminalrun.SplitType) tea.Cmd { return t.m.cmdEditSelectedFile(s) }

func (t modelMenuTab) Selection() rowactions.Selection {
	m := t.m
	r, ok := m.selectedRow()
	if !ok {
		return rowactions.Selection{}
	}
	epic := m.epicAt(r)
	if r.isEpic() {
		return rowactions.Selection{
			OK: true, IsEpic: true, Epic: epic, Archived: r.archived,
			Session:   rowactions.EpicSession(m.vm.Sessions, epic.Path),
			Queueable: rowactions.EpicQueueable(epic, m.vm.Queue),
			Actions:   rowactions.EpicSummaryActions(m.vm.Epics, epic.Name),
		}
	}
	ticket := epic.Tickets[r.ticketIdx]
	return rowactions.Selection{
		OK: true, Epic: epic, Ticket: ticket, Archived: r.archived,
		Session:    rowactions.TicketSession(m.vm.Sessions, epic.Path, ticket),
		QueueAgent: rowactions.QueueAgentOf(m.vm.Sessions, ticket.Path),
	}
}

func (t queueMenuTab) Env() rowactions.Env                      { return t.m.menuEnv() }
func (t queueMenuTab) ModalOpen() bool                          { return t.m.ModalOpen() }
func (t queueMenuTab) YankSummary() tea.Cmd                     { return t.m.yankQueueTicketSummary() }
func (t queueMenuTab) YankFilePath() tea.Cmd                    { return t.m.yankQueueTicketFilePath() }
func (t queueMenuTab) EditFile(s terminalrun.SplitType) tea.Cmd { return t.m.cmdEditSelectedFile(s) }

func (t queueMenuTab) Selection() rowactions.Selection {
	m := t.m
	if epic, ok := m.selectedQueueEpic(); ok {
		return rowactions.Selection{
			OK: true, IsEpic: true, Epic: epic,
			Queueable: rowactions.EpicQueueable(epic, m.checkedPaths()),
			Actions:   m.server.summaries[epic.Name].Actions,
		}
	}
	r, ok := m.selectedQueueRow()
	if !ok {
		return rowactions.Selection{}
	}
	return rowactions.Selection{OK: true, Epic: r.epic, Ticket: r.ticket}
}

// handleSuggestedActionsKey applies the "m" keymap.
func (m Model) handleSuggestedActionsKey() (tea.Model, tea.Cmd) {
	cmd := m.menus.HandleKey(modelMenuTab{m})
	return m, cmd
}

func (m Model) handleTicketMenuReady(msg rowactions.TicketReadyMsg) (Model, tea.Cmd) {
	cmd := m.menus.HandleReady(msg, modelMenuTab{m})
	return m, cmd
}

func (m Model) handleTicketMenuPick(msg rowactions.TicketPickMsg) (tea.Model, tea.Cmd) {
	return m, m.menus.HandleTicketPick(msg, modelMenuTab{m}).Cmd
}

func (m Model) handleEpicMenuPick(msg rowactions.EpicPickMsg) (tea.Model, tea.Cmd) {
	return m, m.menus.HandleEpicPick(msg, modelMenuTab{m})
}

func (m Model) handleServerResumeEnter() (cmd tea.Cmd, handled bool) {
	return rowactions.ResumeEnter(modelMenuTab{m})
}

func (m Model) rowHasSuggestedActions(epic tickets.Epic, ticket tickets.Ticket, archived bool) bool {
	return m.menus.HasSuggestedActions(m.menuEnv(), epic, ticket, archived)
}

// selectedTicketPath is the selected ticket row's path, or "" off a ticket.
func (m Model) selectedTicketPath() string {
	return modelMenuTab{m}.Selection().TicketPath()
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

// handleStatusChanged applies rowactions.StatusChangedMsg. Up mode does
// nothing: the store watch turns the file write into a ticket-changed event.
// Down mode has no stream, so it reloads from disk.
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
	cmd := m.menus.HandleKey(queueMenuTab{m})
	return m, cmd
}

// selectedQueuePath is the selected ticket row's path, or "" off a ticket.
func (m QueueModel) selectedQueuePath() string {
	r, _ := m.selectedQueueRow()
	return r.ticket.Path
}

// selectedQueueEpic is the selected epic header's epic; ok is false off one.
func (m QueueModel) selectedQueueEpic() (tickets.Epic, bool) {
	entries := m.rows.queueTree.Entries()
	idx := m.rows.queueTree.SelectedIndex()
	if idx < 0 || idx >= len(entries) || entries[idx].Value.kind != nodeEpicName {
		return tickets.Epic{}, false
	}
	return entries[idx].Value.epic, true
}

// menuAnchor is the selected queue row on screen, under the panel's header
// line and the queue header, for the Context menu to open next to.
func (m QueueModel) menuAnchor() contextmenu.Anchor {
	line, h, ok := m.rows.queueTree.SelectedBodyLine()
	if !ok {
		return contextmenu.Anchor{}
	}
	return contextmenu.Anchor{X: 1, Y: 1 + queueHeaderReservedLines + line, H: h}
}
