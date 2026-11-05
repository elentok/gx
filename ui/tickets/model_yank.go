package tickets

import (
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/ui/notify"
)

// formatTicketSummary renders a ticket for the "yy" chord: "{epic} - {id}
// {title}", mirroring the "%s %s" id+title format already used across this
// package's views/menus (see view.go, queue_actions_menu.go).
func formatTicketSummary(epicName string, t tickets.Ticket) string {
	return fmt.Sprintf("%s - %s %s", epicName, t.DisplayNumber(), t.Title)
}

// yankTicketSummary implements the Tickets tab's "yy" chord: copies the
// selected ticket's epic/id/title to the clipboard, or an epic row's name.
func (m Model) yankTicketSummary() tea.Cmd {
	r, ok := m.selectedRow()
	if !ok {
		return notify.Warning("nothing selected")
	}
	epic := m.epicAt(r)
	if r.isEpic() {
		return m.editor.Yank(epic.Name, "epic")
	}
	return m.editor.Yank(formatTicketSummary(epic.Name, epic.Tickets[r.ticketIdx]), "ticket")
}

// yankTicketFilePath implements the Tickets tab's "yf" chord: copies the
// selected row's file path (an epic row's ticket.md) to the clipboard.
func (m Model) yankTicketFilePath() tea.Cmd {
	path, ok, warning := m.selectedEditTarget()
	if !ok {
		return notify.Warning(warning)
	}
	return m.editor.Yank(path, "file path")
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
	return m.editor.Yank(formatTicketSummary(r.epic.Name, r.ticket), "ticket")
}

// yankQueueTicketFilePath is the Queue tab's counterpart to
// Model.yankTicketFilePath ("yf").
func (m QueueModel) yankQueueTicketFilePath() tea.Cmd {
	r, ok := m.selectedQueueRow()
	if !ok {
		return notify.Warning("no ticket selected")
	}
	return m.editor.Yank(r.ticket.FilePath(), "file path")
}
