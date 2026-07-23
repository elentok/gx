package tickets

import (
	"sort"

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
// No collapse/expand yet (ticket 04) — every epic's tickets render
// immediately after their epic, grouped by rendered status (unblocked →
// blocked → needs-info → done → error) and ticket number ascending within
// each group, so actionable work sorts to the top.
func (m Model) visibleRows() []row {
	var rows []row
	for epicIdx, epic := range m.epics {
		rows = append(rows, row{epicIdx: epicIdx, ticketIdx: -1})
		for _, ticketIdx := range sortedTicketIndexes(epic) {
			rows = append(rows, row{epicIdx: epicIdx, ticketIdx: ticketIdx})
		}
	}
	return rows
}

// sortedTicketIndexes orders epic.Tickets' indexes by rendered-status group,
// then ticket number ascending within each group.
func sortedTicketIndexes(epic tickets.Epic) []int {
	indexes := make([]int, len(epic.Tickets))
	for i := range indexes {
		indexes[i] = i
	}
	sort.SliceStable(indexes, func(i, j int) bool {
		a, b := epic.Tickets[indexes[i]], epic.Tickets[indexes[j]]
		groupA, groupB := tickets.GroupOrder(epic.RenderedStatus(a)), tickets.GroupOrder(epic.RenderedStatus(b))
		if groupA != groupB {
			return groupA < groupB
		}
		return a.Number < b.Number
	})
	return indexes
}
