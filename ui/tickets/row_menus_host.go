package tickets

import (
	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/ui/contextmenu"
	"github.com/elentok/gx/ui/terminalrun"
	"github.com/elentok/gx/ui/tickets/rowactions"
)

// The root half of rowactions.Menus: each tab model adapts itself to
// rowactions.Tab and forwards its Update cases. Nothing here belongs with the
// menus, which never see Model.

// modelMenuTab adapts the Tickets tab model to rowactions.Tab.
type modelMenuTab struct{ m Model }

func (m Model) menuEnv() rowactions.Env {
	return rowactions.Env{
		ServerAPI: m.server.api, Admin: m.admin, Reads: m.reads, Sessions: m.sessions,
		Tickets: m.tickets, Queue: m.queue, Link: m.server.link, Settings: m.settings,
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
	vm := m.server.vm
	r, ok := m.sidebar.SelectedRow()
	if !ok {
		return rowactions.Selection{}
	}
	epic := m.sidebar.EpicAt(r)
	if r.IsEpic() {
		return rowactions.Selection{
			OK: true, IsEpic: true, Epic: epic, Archived: r.Archived,
			Session:   rowactions.EpicSession(vm.Sessions, epic.Path),
			Queueable: rowactions.EpicQueueable(epic, vm.Queue),
			Actions:   rowactions.EpicSummaryActions(vm.Epics, epic.Name),
		}
	}
	ticket := epic.Tickets[r.TicketIdx]
	return rowactions.Selection{
		OK: true, Epic: epic, Ticket: ticket, Archived: r.Archived,
		Session:    rowactions.TicketSession(vm.Sessions, epic.Path, ticket),
		QueueAgent: rowactions.QueueAgentOf(vm.Sessions, ticket.Path),
	}
}

// handleSuggestedActionsKey applies the "m" keymap.
func (m Model) handleSuggestedActionsKey() (tea.Model, tea.Cmd) {
	cmd := m.menus.HandleKey(modelMenuTab{m})
	return m, cmd
}

func (m Model) handleTicketMenuPick(msg rowactions.TicketPickMsg) (tea.Model, tea.Cmd) {
	return m, m.menus.HandleTicketPick(msg, modelMenuTab{m}).Cmd
}

// selectedTicketPath is the selected ticket row's path, or "" off a ticket.
func (m Model) selectedTicketPath() string {
	return modelMenuTab{m}.Selection().TicketPath()
}

// menuAnchor is the selected sidebar row on screen, under the panel's
// header line, for the Context menu to open next to.
func (m Model) menuAnchor() contextmenu.Anchor {
	line, h, ok := m.sidebar.Tree.SelectedBodyLine()
	if !ok {
		return contextmenu.Anchor{}
	}
	return contextmenu.Anchor{X: 1, Y: 1 + line, H: h}
}
