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

// visibleRows flattens the loaded epics into the tab's rendered row order:
// every epic row, followed by its tickets (grouped by rendered status —
// unblocked → blocked → needs-info → done → error, ticket number ascending
// within each group) unless the epic is collapsed, in which case its
// tickets are excluded entirely and navigation moves past the epic directly
// to the next visible row.
func (m Model) visibleRows() []row {
	var rows []row
	for epicIdx, epic := range m.epics {
		rows = append(rows, row{epicIdx: epicIdx, ticketIdx: -1})
		if m.isCollapsed(epic) {
			continue
		}
		for _, ticketIdx := range sortedTicketIndexes(epic) {
			rows = append(rows, row{epicIdx: epicIdx, ticketIdx: ticketIdx})
		}
	}
	return rows
}

func (m Model) isCollapsed(epic tickets.Epic) bool {
	return m.collapsedEpics[epic.Path]
}

// defaultCollapsedEpics computes the initial per-epic collapse state: an
// epic where every ticket is done starts collapsed; every other epic
// (including a zero-ticket epic) starts expanded.
func defaultCollapsedEpics(epics []tickets.Epic) map[string]bool {
	collapsed := make(map[string]bool, len(epics))
	for _, epic := range epics {
		if epic.AllDone() {
			collapsed[epic.Path] = true
		}
	}
	return collapsed
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
