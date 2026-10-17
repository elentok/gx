package tickets

import (
	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/ui/notify"
)

// actionsMenuResult is an accepted Context menu action, keyed by the row's
// address (Path), for dispatchMenuAction.
type actionsMenuResult struct {
	Done     bool
	Accepted bool
	Path     string
	EpicName string
	TicketID string
	Action   string
}

// handleSuggestedActionsKey applies the "m" keymap: opens the Context menu
// for the selected ticket or epic row.
func (m Model) handleSuggestedActionsKey() (tea.Model, tea.Cmd) {
	r, ok := m.selectedRow()
	if !ok {
		return m, notify.Info("select a ticket or epic to see its actions")
	}
	epic := m.epicAt(r)
	if r.isEpic() {
		return m.openEpicMenu(epic, r.archived), nil
	}
	return m.openTicketMenu(epic, epic.Tickets[r.ticketIdx], r.archived)
}

// selectedTicketPath is the selected ticket row's path, or "" off a ticket.
func (m Model) selectedTicketPath() string {
	r, ok := m.selectedRow()
	if !ok || r.isEpic() {
		return ""
	}
	return m.epicAt(r).Tickets[r.ticketIdx].Path
}
