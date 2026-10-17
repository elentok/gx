package tickets

import (
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/ui/clip"
	"github.com/elentok/gx/ui/notify"
)

var ticketsClipboardWrite = clip.Write

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
		if err := ticketsClipboardWrite(epic.Name); err != nil {
			return notify.Error("clipboard copy failed: " + err.Error())
		}
		return notify.Info("yanked epic")
	}
	text := formatTicketSummary(epic.Name, epic.Tickets[r.ticketIdx])
	if err := ticketsClipboardWrite(text); err != nil {
		return notify.Error("clipboard copy failed: " + err.Error())
	}
	return notify.Info("yanked ticket")
}

// yankTicketFilePath implements the Tickets tab's "yf" chord: copies the
// selected row's file path (an epic row's ticket.md) to the clipboard.
func (m Model) yankTicketFilePath() tea.Cmd {
	path, ok, warning := m.selectedEditTarget()
	if !ok {
		return notify.Warning(warning)
	}
	if err := ticketsClipboardWrite(path); err != nil {
		return notify.Error("clipboard copy failed: " + err.Error())
	}
	return notify.Info("yanked file path")
}

// yankQueueTicketSummary is the Queue tab's counterpart to
// Model.yankTicketSummary ("yy").
func (m QueueModel) yankQueueTicketSummary() tea.Cmd {
	if epic, ok := m.selectedQueueEpic(); ok {
		if err := ticketsClipboardWrite(epic.Name); err != nil {
			return notify.Error("clipboard copy failed: " + err.Error())
		}
		return notify.Info("yanked epic")
	}
	r, ok := m.selectedQueueRow()
	if !ok {
		return notify.Warning("no ticket selected")
	}
	text := formatTicketSummary(r.epic.Name, r.ticket)
	if err := ticketsClipboardWrite(text); err != nil {
		return notify.Error("clipboard copy failed: " + err.Error())
	}
	return notify.Info("yanked ticket")
}

// yankQueueTicketFilePath is the Queue tab's counterpart to
// Model.yankTicketFilePath ("yf").
func (m QueueModel) yankQueueTicketFilePath() tea.Cmd {
	r, ok := m.selectedQueueRow()
	if !ok {
		return notify.Warning("no ticket selected")
	}
	path := r.ticket.FilePath()
	if err := ticketsClipboardWrite(path); err != nil {
		return notify.Error("clipboard copy failed: " + err.Error())
	}
	return notify.Info("yanked file path")
}
