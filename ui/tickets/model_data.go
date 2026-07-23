package tickets

import (
	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/tickets"
)

type epicsLoadedMsg struct {
	epics []tickets.Epic
	err   error
}

// cmdLoad reads the current worktree's `.scratch/` directory in the
// background. A missing directory is not an error (tickets.Load reports it
// as zero epics), so it renders the same empty state as an absent
// `.scratch/`.
func (m Model) cmdLoad() tea.Cmd {
	scratchDir := m.scratchDir()
	return func() tea.Msg {
		epics, err := tickets.Load(scratchDir)
		return epicsLoadedMsg{epics: epics, err: err}
	}
}

// row is one flat, navigable line in the sidebar: either an epic header or
// one of its tickets.
type row struct {
	epicIdx   int
	ticketIdx int // -1 for an epic row
}

func (r row) isEpic() bool { return r.ticketIdx < 0 }

// visibleRows flattens the loaded epics into the tab's rendered row order.
// No collapse/expand or grouping yet (tickets 03/04) — every epic's tickets
// render in loader order, immediately after their epic.
func (m Model) visibleRows() []row {
	var rows []row
	for epicIdx, epic := range m.epics {
		rows = append(rows, row{epicIdx: epicIdx, ticketIdx: -1})
		for ticketIdx := range epic.Tickets {
			rows = append(rows, row{epicIdx: epicIdx, ticketIdx: ticketIdx})
		}
	}
	return rows
}
