package tickets

import (
	"sort"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/ui/notify"
	"github.com/elentok/gx/ui/tree"
)

// noParentTicket is nearestVisibleAncestor's sentinel for a ticket that isn't
// nested under another ticket (either a top-level ticket, or a nested one
// whose containing ancestor was filtered out by hideDone).
const noParentTicket = -1

// sidebarNodeKind distinguishes the three row kinds the sidebar tree can
// hold: a section header ("Open epics"/"Closed epics"), an epic, or a
// ticket. tree.Model[T] itself has no notion of a heterogeneous node — this
// sum type plus idFn/childrenFn is what makes one tree.Model[sidebarNode]
// stand in for what used to be two hand-flattened levels (epics, then each
// epic's own tickets).
type sidebarNodeKind int

const (
	nodeSection sidebarNodeKind = iota
	nodeEpic
	nodeTicket
)

// sidebarSection is which of the two section-header roots a sidebarNode
// belongs under.
type sidebarSection int

const (
	sectionOpen sidebarSection = iota
	sectionClosed
)

// sidebarNode is ui/tree.Model[sidebarNode]'s value type. ticketIdx is -1 for
// nodeSection/nodeEpic.
type sidebarNode struct {
	kind      sidebarNodeKind
	section   sidebarSection
	epicIdx   int
	ticketIdx int
}

// epicTicketTree is one epic's ticketRows-equivalent shape: which tickets are
// top-level roots vs. nested under another ticket (Parent/Children, ticket
// 03), computed once per epic per buildSidebarEntries call via
// nearestVisibleAncestor and shared between that epic's nodeEpic (roots) and
// nodeTicket (childrenOf) childrenFn lookups.
type epicTicketTree struct {
	roots      []int
	childrenOf map[int][]int
}

// buildEpicTicketTree computes epicIdx's visible-ticket parent/child shape:
// sortedTicketIndexes' plan order, filtered by hideDone, then nested via each
// ticket's nearest visible ancestor (a hideDone-filtered parent reattaches
// its children one level further up instead of stranding them).
func (m Model) buildEpicTicketTree(epicIdx int) epicTicketTree {
	epic := m.epics[epicIdx]
	sorted := sortedTicketIndexes(epic)

	visible := make(map[int]bool, len(sorted))
	for _, idx := range sorted {
		if m.hideDone && epic.RenderedStatus(epic.Tickets[idx]) == tickets.StatusDone {
			continue
		}
		visible[idx] = true
	}

	byIdentifier := make(map[string]int, len(epic.Tickets))
	for i, t := range epic.Tickets {
		if t.Identifier != "" {
			byIdentifier[t.Identifier] = i
		}
	}

	childrenOf := make(map[int][]int, len(sorted))
	var roots []int
	for _, idx := range sorted {
		if !visible[idx] {
			continue
		}
		parentIdx := nearestVisibleAncestor(epic, idx, visible, byIdentifier)
		if parentIdx == noParentTicket {
			roots = append(roots, idx)
		} else {
			childrenOf[parentIdx] = append(childrenOf[parentIdx], idx)
		}
	}
	return epicTicketTree{roots: roots, childrenOf: childrenOf}
}

// buildSidebarEntries flattens m.epics into the sidebar's full tree.Entry
// list: two nodeSection roots ("Open epics"/"Closed epics", per
// splitEpicIndexesBySection), each epic's root tickets beneath its nodeEpic
// entry, and each ticket's nested children (epicTicketTree.childrenOf)
// beneath its nodeTicket entry. Called on every epicsLoadedMsg/hideDone
// toggle/collapse mutation via clampSelected.
func (m Model) buildSidebarEntries() []tree.Entry[sidebarNode] {
	idxs := make([]int, len(m.epics))
	for i := range m.epics {
		idxs[i] = i
	}
	openIdxs, closedIdxs := splitEpicIndexesBySection(m.epics, idxs)

	epicTrees := make(map[int]epicTicketTree, len(m.epics))
	epicTree := func(epicIdx int) epicTicketTree {
		t, ok := epicTrees[epicIdx]
		if !ok {
			t = m.buildEpicTicketTree(epicIdx)
			epicTrees[epicIdx] = t
		}
		return t
	}

	roots := []sidebarNode{
		{kind: nodeSection, section: sectionOpen, ticketIdx: -1},
		{kind: nodeSection, section: sectionClosed, ticketIdx: -1},
	}

	idFn := func(n sidebarNode) string {
		switch n.kind {
		case nodeSection:
			if n.section == sectionOpen {
				return "section:open"
			}
			return "section:closed"
		case nodeEpic:
			return m.epics[n.epicIdx].Path
		default:
			return m.epics[n.epicIdx].Tickets[n.ticketIdx].Path
		}
	}

	childrenFn := func(n sidebarNode) []sidebarNode {
		switch n.kind {
		case nodeSection:
			order := openIdxs
			if n.section == sectionClosed {
				order = closedIdxs
			}
			children := make([]sidebarNode, len(order))
			for i, epicIdx := range order {
				children[i] = sidebarNode{kind: nodeEpic, epicIdx: epicIdx, ticketIdx: -1}
			}
			return children
		case nodeEpic:
			roots := epicTree(n.epicIdx).roots
			children := make([]sidebarNode, len(roots))
			for i, idx := range roots {
				children[i] = sidebarNode{kind: nodeTicket, epicIdx: n.epicIdx, ticketIdx: idx}
			}
			return children
		default:
			kids := epicTree(n.epicIdx).childrenOf[n.ticketIdx]
			children := make([]sidebarNode, len(kids))
			for i, idx := range kids {
				children[i] = sidebarNode{kind: nodeTicket, epicIdx: n.epicIdx, ticketIdx: idx}
			}
			return children
		}
	}

	return tree.BuildEntriesFromValues(roots, idFn, childrenFn, m.sidebarTree.CollapsedIDs())
}

type epicsLoadedMsg struct {
	epics []tickets.Epic
	err   error
}

// cmdLoad reads the tab's `.scratch/` directory in the background. A missing
// directory is not an error (tickets.Load reports it as zero epics), so it
// renders the same empty state as an absent `.scratch/`.
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

// row is the shim view.go's render functions and model_preview_focus.go's
// selectedRow() consumers read, adapted from a tree.Entry[sidebarNode] by
// rowFromEntry. depth/hasChildren/expanded mirror ui/tree's Entry (ticket 02)
// for a ticket row nested under another ticket via Parent/Children (ticket
// 03): depth 0 is a ticket directly under the epic (rowFromEntry subtracts
// the section+epic ancestor levels from the entry's own Depth), and
// hasChildren/expanded drive the row's own fold glyph.
type row struct {
	epicIdx     int
	ticketIdx   int // -1 for an epic row
	depth       int
	hasChildren bool
	expanded    bool
}

func (r row) isEpic() bool { return r.ticketIdx < 0 }

// rowFromEntry adapts one sidebar tree.Entry into row's flat shape. A
// nodeSection entry has no row representation — section headers were never
// cursor-reachable pre-migration (see skipSectionHeader) — so it reports ok
// = false.
func rowFromEntry(e tree.Entry[sidebarNode]) (row, bool) {
	switch e.Value.kind {
	case nodeEpic:
		return row{epicIdx: e.Value.epicIdx, ticketIdx: -1}, true
	case nodeTicket:
		return row{
			epicIdx:     e.Value.epicIdx,
			ticketIdx:   e.Value.ticketIdx,
			depth:       e.Depth - 2, // section, epic ancestors
			hasChildren: e.HasChildren,
			expanded:    e.Expanded,
		}, true
	default:
		return row{}, false
	}
}

// nearestVisibleAncestor walks idx's Parent chain (ticket 03's schema field)
// up to the first ancestor still present in visible, so a hideDone-filtered
// parent doesn't strand its children — they nest one level further up
// instead of vanishing along with it. Returns noParentTicket once the chain
// runs out (or hits a Parent token with no matching ticket in the epic) or
// on a cyclical chain (never expected in practice, guarded via seen so this
// can't loop forever).
func nearestVisibleAncestor(epic tickets.Epic, idx int, visible map[int]bool, byIdentifier map[string]int) int {
	seen := map[int]bool{idx: true}
	cur := epic.Tickets[idx]
	for cur.Parent != nil {
		parentIdx, ok := byIdentifier[*cur.Parent]
		if !ok || seen[parentIdx] {
			return noParentTicket
		}
		if visible[parentIdx] {
			return parentIdx
		}
		seen[parentIdx] = true
		cur = epic.Tickets[parentIdx]
	}
	return noParentTicket
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
	return m.sidebarTree.CollapsedIDs()[epic.Path]
}

// defaultCollapsedEpics computes the per-epic collapse state for epics,
// preserving any entry already present in existing (e.g. a user's manual
// toggle) and only filling in a default for epic paths not yet in existing:
// an epic where every ticket is done starts collapsed; every other epic
// (including a zero-ticket epic) starts expanded.
func defaultCollapsedEpics(epics []tickets.Epic, existing map[string]bool) map[string]bool {
	collapsed := make(map[string]bool, len(epics))
	for _, epic := range epics {
		if v, ok := existing[epic.Path]; ok {
			// Preserve the entry itself, not just its value, so an explicit
			// false (manually expanded) survives into the next reload's
			// existing map instead of vanishing back to "unseen".
			collapsed[epic.Path] = v
			continue
		}
		if epic.AllDone() {
			collapsed[epic.Path] = true
		}
	}
	return collapsed
}

// sortedTicketIndexes orders epic.Tickets' indexes in plan order — ticket
// number ascending, so the list reads as the epic's intended order of
// execution and a ticket (done or otherwise) never jumps out of its place
// once it finishes. Lettered siblings sharing a Number (e.g. 04,
// 04a, 04b) tie-break on DisplayNumber, so the original sorts before its
// replacements in filename order.
func sortedTicketIndexes(epic tickets.Epic) []int {
	indexes := make([]int, len(epic.Tickets))
	for i := range indexes {
		indexes[i] = i
	}
	sort.SliceStable(indexes, func(i, j int) bool {
		a, b := epic.Tickets[indexes[i]], epic.Tickets[indexes[j]]
		if a.Number != b.Number {
			return a.Number < b.Number
		}
		return a.DisplayNumber() < b.DisplayNumber()
	})
	return indexes
}
