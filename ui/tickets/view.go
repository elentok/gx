package tickets

import (
	"fmt"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/ui"
)

// sidebarLines renders the flat epic/ticket tree: each epic's name +
// (open/total) count, each ticket's title indented beneath it. No status
// icons, grouping, or collapse behavior yet (tickets 03/04).
func (m Model) sidebarLines() []string {
	if !m.loaded {
		return []string{ui.StyleDim.Render("  loading…")}
	}
	if len(m.epics) == 0 {
		return []string{ui.StyleMuted.Render("  no .scratch/ directory found")}
	}

	rows := m.visibleRows()
	lines := make([]string, 0, len(rows))
	for _, r := range rows {
		if r.isEpic() {
			lines = append(lines, m.renderEpicRow(m.epics[r.epicIdx]))
		} else {
			lines = append(lines, m.renderTicketRow(m.epics[r.epicIdx].Tickets[r.ticketIdx]))
		}
	}
	return lines
}

func (m Model) renderEpicRow(epic tickets.Epic) string {
	return fmt.Sprintf("  %s (%d/%d)", epic.Name, epic.OpenCount(), epic.TotalCount())
}

func (m Model) renderTicketRow(t tickets.Ticket) string {
	return "    " + t.Title
}
