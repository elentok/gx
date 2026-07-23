package tickets

import (
	"path/filepath"
	"sort"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/ui/notify"
)

type epicsLoadedMsg struct {
	epics []tickets.Epic
	// worktreeNames is the ordered list of worktrees represented in epics,
	// only populated in --all mode (nil in the single-worktree view).
	worktreeNames []string
	err           error
}

// cmdLoad reads the tab's `.scratch/` directory (or, in --all mode, every
// worktree's) in the background. A missing directory is not an error
// (tickets.Load reports it as zero epics), so it renders the same empty
// state as an absent `.scratch/`.
func (m Model) cmdLoad() tea.Cmd {
	if m.allRepos {
		return m.cmdLoadAll()
	}
	scratchDir := m.scratchDir()
	return func() tea.Msg {
		epics, err := tickets.Load(scratchDir)
		return epicsLoadedMsg{epics: epics, err: err}
	}
}

// cmdLoadAll aggregates `.scratch/` across every worktree of the repo (the
// `gx tickets --all` scope): each worktree's epics are tagged with
// Epic.WorktreeName and worktreeNames records the display order (git's
// `worktree list` order) so rows can be grouped per worktree.
func (m Model) cmdLoadAll() tea.Cmd {
	worktreeRoot := m.worktreeRoot
	return func() tea.Msg {
		repo, err := git.FindRepo(worktreeRoot)
		if err != nil {
			return epicsLoadedMsg{err: err}
		}
		worktrees, err := git.ListWorktrees(*repo)
		if err != nil {
			return epicsLoadedMsg{err: err}
		}

		var allEpics []tickets.Epic
		names := make([]string, 0, len(worktrees))
		for _, wt := range worktrees {
			epics, loadErr := tickets.Load(filepath.Join(wt.Path, ".scratch"))
			if loadErr != nil {
				// Best-effort aggregation: an unreadable worktree is dropped
				// rather than failing the whole --all load.
				continue
			}
			for i := range epics {
				epics[i].WorktreeName = wt.Name
			}
			allEpics = append(allEpics, epics...)
			names = append(names, wt.Name)
		}
		return epicsLoadedMsg{epics: allEpics, worktreeNames: names}
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

// epicGroup is one top-level group of epic indexes: a single implicit group
// (worktreeName == "") in the single-worktree view, or one group per
// worktree (in git's `worktree list` order) in --all mode — see
// (Model).epicGroups.
type epicGroup struct {
	worktreeName string
	epicIdxs     []int
}

// epicGroups partitions m.epics' indexes into the tab's top-level grouping:
// in the single-worktree view it's one unnamed group holding every epic
// (directory-scan order, matching pre-`--all` behavior exactly); in --all
// mode it's one named group per worktree, in m.worktreeNames order, so rows
// render as worktree (header) → epic → ticket.
func (m Model) epicGroups() []epicGroup {
	if !m.allRepos {
		idxs := make([]int, len(m.epics))
		for i := range m.epics {
			idxs[i] = i
		}
		return []epicGroup{{epicIdxs: idxs}}
	}

	groups := make([]epicGroup, 0, len(m.worktreeNames))
	for _, name := range m.worktreeNames {
		var idxs []int
		for i, e := range m.epics {
			if e.WorktreeName == name {
				idxs = append(idxs, i)
			}
		}
		groups = append(groups, epicGroup{worktreeName: name, epicIdxs: idxs})
	}
	return groups
}

// visibleRows flattens the loaded epics into the tab's rendered row order:
// each epicGroups() group in turn, open epics (per splitEpicIndexesBySection)
// before closed ones, each epic row followed by its tickets (grouped by
// rendered status — unblocked → blocked → needs-info → done → error, ticket
// number ascending within each group) unless the epic is collapsed, in which
// case its tickets are excluded entirely and navigation moves past the epic
// directly to the next visible row. Worktree header rows themselves are not
// part of this navigable list — see view.go's sidebarLines, which renders
// them purely for display, interleaved around the same row positions.
func (m Model) visibleRows() []row {
	var rows []row
	for _, g := range m.epicGroups() {
		openIdxs, closedIdxs := splitEpicIndexesBySection(m.epics, g.epicIdxs)
		rows = append(rows, m.rowsForEpicOrder(openIdxs)...)
		rows = append(rows, m.rowsForEpicOrder(closedIdxs)...)
	}
	return rows
}

// rowsForEpicOrder expands an ordered slice of epic indexes into their rows:
// each epic row followed by its (sorted) ticket rows, unless collapsed.
func (m Model) rowsForEpicOrder(order []int) []row {
	var rows []row
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

// splitEpicIndexesBySection splits idxs (indexes into epics) into the "Open
// epics" and "Closed epics" sections shown in the sidebar (mirroring the
// PRs tab's Actionable/Non-actionable split): an epic is closed once every
// one of its tickets is done (Epic.AllDone) — a zero-ticket epic is never
// closed, same rule the default-collapse behavior already uses. Order
// within each group follows idxs' input order.
func splitEpicIndexesBySection(epics []tickets.Epic, idxs []int) (open, closed []int) {
	for _, i := range idxs {
		if epics[i].AllDone() {
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
