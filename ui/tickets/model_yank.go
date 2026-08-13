package tickets

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
	"github.com/atotto/clipboard"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/ui/notify"
)

var ticketsClipboardWrite = clipboard.WriteAll

// formatTicketSummary renders a ticket for the "yy" chord: "{epic} - {id}
// {title}", mirroring the "%s %s" id+title format already used across this
// package's views/menus (see view.go, queue_actions_menu.go).
func formatTicketSummary(epicName string, t tickets.Ticket) string {
	return fmt.Sprintf("%s - %s %s", epicName, t.DisplayNumber(), t.Title)
}

// yankTicketSummary implements the Tickets tab's "yy" chord: copies the
// selected ticket's epic/id/title to the clipboard. No-ops (with a warning)
// on an epic row, which has no ticket to summarize.
func (m Model) yankTicketSummary() tea.Cmd {
	r, ok := m.selectedRow()
	if !ok || r.isEpic() {
		return notify.Warning("no ticket selected")
	}
	epic := m.epics[r.epicIdx]
	text := formatTicketSummary(epic.Name, epic.Tickets[r.ticketIdx])
	if err := ticketsClipboardWrite(text); err != nil {
		return notify.Error("clipboard copy failed: " + err.Error())
	}
	return notify.Info("yanked ticket")
}

// yankTicketFilePath implements the Tickets tab's "yf" chord: copies the
// selected ticket's file path to the clipboard.
func (m Model) yankTicketFilePath() tea.Cmd {
	r, ok := m.selectedRow()
	if !ok || r.isEpic() {
		return notify.Warning("no ticket selected")
	}
	epic := m.epics[r.epicIdx]
	path := epic.Tickets[r.ticketIdx].Path
	if err := ticketsClipboardWrite(path); err != nil {
		return notify.Error("clipboard copy failed: " + err.Error())
	}
	return notify.Info("yanked file path")
}

// yankQueueTicketSummary is the Queue tab's counterpart to
// Model.yankTicketSummary ("yy") — every queue row already carries its own
// ticket (queueRow), so there's no epic-row case to guard against.
func (m QueueModel) yankQueueTicketSummary() tea.Cmd {
	rows := m.rows()
	if m.selected < 0 || m.selected >= len(rows) {
		return notify.Warning("no ticket selected")
	}
	r := rows[m.selected]
	text := formatTicketSummary(r.epic.Name, r.ticket)
	if err := ticketsClipboardWrite(text); err != nil {
		return notify.Error("clipboard copy failed: " + err.Error())
	}
	return notify.Info("yanked ticket")
}

// yankQueueTicketFilePath is the Queue tab's counterpart to
// Model.yankTicketFilePath ("yf").
func (m QueueModel) yankQueueTicketFilePath() tea.Cmd {
	rows := m.rows()
	if m.selected < 0 || m.selected >= len(rows) {
		return notify.Warning("no ticket selected")
	}
	path := rows[m.selected].ticket.Path
	if err := ticketsClipboardWrite(path); err != nil {
		return notify.Error("clipboard copy failed: " + err.Error())
	}
	return notify.Info("yanked file path")
}
