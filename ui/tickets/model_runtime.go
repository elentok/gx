package tickets

import (
	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/ui/notify"
	"github.com/elentok/gx/ui/terminalrun"
	"github.com/elentok/gx/ui/tickets/preview"
)

// cmdEditSelectedFile opens the selected row's underlying file for editing:
// a ticket's own file, or an epic's ticket.md. An epic without a ticket.md
// has nothing to edit, so it's a no-op with an inline message rather than a
// crash. An archived row's file opens read-only.
func (m Model) cmdEditSelectedFile(splitType terminalrun.SplitType) tea.Cmd {
	target, ok, warning := m.selectedEditTarget()
	if !ok {
		return notify.Warning(warning)
	}
	r, _ := m.sidebar.SelectedRow()
	return m.editor.Edit(target, splitType, r.Archived)
}

// selectedEditTarget resolves the file path to edit for the current
// selection. A ticket row always has one; an epic row only has one when it
// has a ticket.md. On failure it also returns a warning message describing why.
func (m Model) selectedEditTarget() (path string, ok bool, warning string) {
	r, ok := m.sidebar.SelectedRow()
	if !ok {
		return "", false, "nothing selected"
	}
	epic := m.sidebar.EpicAt(r)
	if r.IsEpic() {
		path, ok := preview.EpicTicketMDPath(epic)
		if !ok {
			return "", false, "epic has no ticket.md to edit"
		}
		return path, true, ""
	}
	return epic.Tickets[r.TicketIdx].FilePath(), true, ""
}
