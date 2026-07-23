package tickets

import (
	"sort"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/ui/notify"
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

// cmdRefresh reloads .scratch/ from disk, matching every other tab's manual
// refresh convention (`R`): a success notification alongside the reload.
func (m Model) cmdRefresh() tea.Cmd {
	return tea.Batch(notify.Success("refreshed"), m.cmdLoad())
}

// row is one flat, navigable line in the sidebar: either an epic header or
// one of its tickets.
type row struct {
	epicIdx   int
	ticketIdx int // -1 for an epic row
}

func (r row) isEpic() bool { return r.ticketIdx < 0 }

// visibleRows flattens the loaded epics into the tab's rendered row order:
// open epics (per splitEpicIndexesBySection) before closed ones, each epic
// row followed by its tickets (grouped by rendered status — unblocked →
// blocked → needs-info → done → error, ticket number ascending within each
// group) unless the epic is collapsed, in which case its tickets are
// excluded entirely and navigation moves past the epic directly to the next
// visible row.
func (m Model) visibleRows() []row {
	var rows []row
	openIdxs, closedIdxs := splitEpicIndexesBySection(m.epics)
	order := make([]int, 0, len(m.epics))
	order = append(order, openIdxs...)
	order = append(order, closedIdxs...)
	for _, epicIdx := range order {
		epic := m.epics[epicIdx]
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

// splitEpicIndexesBySection splits m.epics' indexes into the "Open epics"
// and "Closed epics" sections shown in the sidebar (mirroring the PRs tab's
// Actionable/Non-actionable split): an epic is closed once every one of its
// tickets is done (Epic.AllDone) — a zero-ticket epic is never closed, same
// rule the default-collapse behavior already uses. Order within each group
// follows m.epics' original (directory-scan) order.
func splitEpicIndexesBySection(epics []tickets.Epic) (open, closed []int) {
	for i, e := range epics {
		if e.AllDone() {
			closed = append(closed, i)
		} else {
			open = append(open, i)
		}
	}
	return open, closed
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
