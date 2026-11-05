package tickets

import (
	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/ui/notify"
	"github.com/elentok/gx/ui/tickets/rowactions"
)

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
	return m.editor.Yank(rowactions.FormatTicketSummary(epic.Name, epic.Tickets[r.ticketIdx]), "ticket")
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
