package queue

import (
	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/ui/contextmenu"
	"github.com/elentok/gx/ui/notify"
	"github.com/elentok/gx/ui/terminalrun"
	"github.com/elentok/gx/ui/tickets/rowactions"
)

// The queue half of rowactions.Menus: QueueModel adapts itself to
// rowactions.Tab and forwards its Update cases.

// queueMenuTab adapts QueueModel to rowactions.Tab.
type queueMenuTab struct{ m QueueModel }

func (m QueueModel) menuEnv() rowactions.Env {
	return rowactions.Env{
		ServerAPI: m.server.ServerAPI, Admin: m.srv.Admin, Reads: m.srv.Reads, Sessions: m.sessions,
		Tickets: m.srv.Tickets, Queue: m.srv.Queue, Link: m.server.Link, Settings: m.settings,
		Width: m.width, WorktreeRoot: m.worktreeRoot,
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
			Actions:   m.server.Summaries[epic.Name].Actions,
		}
	}
	r, ok := m.selectedQueueRow()
	if !ok {
		return rowactions.Selection{}
	}
	return rowactions.Selection{OK: true, Epic: r.Epic, Ticket: r.Ticket}
}

// handleQueueTicketMenuKey applies the "m" keymap.
func (m QueueModel) handleQueueTicketMenuKey() (tea.Model, tea.Cmd) {
	cmd := m.menus.HandleKey(queueMenuTab{m})
	return m, cmd
}

// selectedQueuePath is the selected ticket row's path, or "" off a ticket.
func (m QueueModel) selectedQueuePath() string {
	r, _ := m.selectedQueueRow()
	return r.Ticket.Path
}

// selectedQueueEpic is the selected epic header's epic; ok is false off one.
func (m QueueModel) selectedQueueEpic() (tickets.Epic, bool) {
	entries := m.rows.Tree.Entries()
	idx := m.rows.Tree.SelectedIndex()
	if idx < 0 || idx >= len(entries) || entries[idx].Value.Kind != NodeEpicName {
		return tickets.Epic{}, false
	}
	return entries[idx].Value.Epic, true
}

// menuAnchor is the selected queue row on screen, under the panel's header
// line and the queue header, for the Context menu to open next to.
func (m QueueModel) menuAnchor() contextmenu.Anchor {
	line, h, ok := m.rows.Tree.SelectedBodyLine()
	if !ok {
		return contextmenu.Anchor{}
	}
	return contextmenu.Anchor{X: 1, Y: 1 + HeaderReservedLines + line, H: h}
}

// yankQueueTicketSummary is the Queue tab's counterpart to
// Model.yankTicketSummary ("yy").
func (m QueueModel) yankQueueTicketSummary() tea.Cmd {
	if epic, ok := m.selectedQueueEpic(); ok {
		return m.editor.Yank(epic.Name, "epic")
	}
	r, ok := m.selectedQueueRow()
	if !ok {
		return notify.Warning("no ticket selected")
	}
	return m.editor.Yank(rowactions.FormatTicketSummary(r.Epic.Name, r.Ticket), "ticket")
}

// yankQueueTicketFilePath is the Queue tab's counterpart to
// Model.yankTicketFilePath ("yf").
func (m QueueModel) yankQueueTicketFilePath() tea.Cmd {
	r, ok := m.selectedQueueRow()
	if !ok {
		return notify.Warning("no ticket selected")
	}
	return m.editor.Yank(r.Ticket.FilePath(), "file path")
}
