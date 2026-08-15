package tickets

import (
	"sort"
	"strings"

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
	nodeEmpty     // "no open/closed/archived epics" placeholder, child of a nodeSection with zero epics
	nodeLoading   // "loading…" placeholder, child of the Archived section while its lazy load is in flight
	nodeLoadError // inline error placeholder, child of the Archived section after a failed lazy load
)

// sidebarSection is which of the three section-header roots a sidebarNode
// belongs under.
type sidebarSection int

const (
	sectionOpen sidebarSection = iota
	sectionClosed
	sectionArchived
)

// sidebarNode is ui/tree.Model[sidebarNode]'s value type. ticketIdx is -1 for
// nodeSection/nodeEpic. archived marks a node sourced from m.archivedEpics
// rather than m.epics — always false until ticket 04's Archived section
// starts populating entries from it; carried through to row (rowFromEntry)
// so epicAt(row) knows which slice to resolve against.
type sidebarNode struct {
	kind      sidebarNodeKind
	section   sidebarSection
	epicIdx   int
	ticketIdx int
	archived  bool
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

// buildEpicTicketTree computes epic's visible-ticket parent/child shape:
// sortedTicketIndexes' plan order, filtered by hideDone, then nested via each
// ticket's nearest visible ancestor (a hideDone-filtered parent reattaches
// its children one level further up instead of stranding them). Takes the
// Epic value directly (rather than an index into m.epics) so it works
// equally for m.archivedEpics — indices in that slice mean nothing against
// m.epics.
func (m Model) buildEpicTicketTree(epic tickets.Epic) epicTicketTree {
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

	epicTreesOpen := make(map[int]epicTicketTree, len(m.epics))
	epicTreesArchived := make(map[int]epicTicketTree, len(m.archivedEpics))
	epicTree := func(archived bool, epicIdx int) epicTicketTree {
		cache := epicTreesOpen
		epics := m.epics
		if archived {
			cache, epics = epicTreesArchived, m.archivedEpics
		}
		t, ok := cache[epicIdx]
		if !ok {
			t = m.buildEpicTicketTree(epics[epicIdx])
			cache[epicIdx] = t
		}
		return t
	}

	roots := []sidebarNode{
		{kind: nodeSection, section: sectionOpen, ticketIdx: -1},
		{kind: nodeSection, section: sectionClosed, ticketIdx: -1},
		{kind: nodeSection, section: sectionArchived, ticketIdx: -1},
	}

	idFn := func(n sidebarNode) string {
		switch n.kind {
		case nodeSection:
			return sidebarSectionID(n.section)
		case nodeEmpty:
			return sidebarSectionID(n.section) + ":empty"
		case nodeLoading:
			return sidebarSectionID(n.section) + ":loading"
		case nodeLoadError:
			return sidebarSectionID(n.section) + ":error"
		case nodeEpic:
			return m.epicAt(row{epicIdx: n.epicIdx, archived: n.archived}).Path
		default:
			return m.epicAt(row{epicIdx: n.epicIdx, archived: n.archived}).Tickets[n.ticketIdx].Path
		}
	}

	childrenFn := func(n sidebarNode) []sidebarNode {
		switch n.kind {
		case nodeSection:
			if n.section == sectionArchived {
				return m.archivedSectionChildren()
			}
			order := openIdxs
			if n.section == sectionClosed {
				order = closedIdxs
			}
			if len(order) == 0 {
				return []sidebarNode{{kind: nodeEmpty, section: n.section, ticketIdx: -1}}
			}
			children := make([]sidebarNode, len(order))
			for i, epicIdx := range order {
				children[i] = sidebarNode{kind: nodeEpic, epicIdx: epicIdx, ticketIdx: -1}
			}
			return children
		case nodeEmpty, nodeLoading, nodeLoadError:
			return nil
		case nodeEpic:
			roots := epicTree(n.archived, n.epicIdx).roots
			children := make([]sidebarNode, len(roots))
			for i, idx := range roots {
				children[i] = sidebarNode{kind: nodeTicket, epicIdx: n.epicIdx, ticketIdx: idx, archived: n.archived}
			}
			return children
		default:
			kids := epicTree(n.archived, n.epicIdx).childrenOf[n.ticketIdx]
			children := make([]sidebarNode, len(kids))
			for i, idx := range kids {
				children[i] = sidebarNode{kind: nodeTicket, epicIdx: n.epicIdx, ticketIdx: idx, archived: n.archived}
			}
			return children
		}
	}

	return tree.BuildEntriesFromValues(roots, idFn, childrenFn, m.sidebarTree.CollapsedIDs())
}

// archivedSectionChildren computes the Archived section header's synthetic
// children: the shared nodeEmpty placeholder when the up-front count is
// zero (so an empty archive renders identically to an empty Open/Closed
// section, never as a bare non-expandable row), otherwise a loading/error
// placeholder or the real archived-epic rows depending on m.archivedLazy's
// current state.
func (m Model) archivedSectionChildren() []sidebarNode {
	if m.archivedEpicCount == 0 {
		return []sidebarNode{{kind: nodeEmpty, section: sectionArchived, ticketIdx: -1}}
	}
	switch m.archivedLazy.State() {
	case tree.LazyLoaded:
		children := make([]sidebarNode, len(m.archivedEpics))
		for i := range m.archivedEpics {
			children[i] = sidebarNode{kind: nodeEpic, epicIdx: i, ticketIdx: -1, archived: true}
		}
		return children
	case tree.LazyFailed:
		return []sidebarNode{{kind: nodeLoadError, section: sectionArchived, ticketIdx: -1}}
	default:
		return []sidebarNode{{kind: nodeLoading, section: sectionArchived, ticketIdx: -1}}
	}
}

type epicsLoadedMsg struct {
	epics             []tickets.Epic
	err               error
	archivedEpicCount int
}

// cmdLoad reads the tab's `.scratch/` directory in the background. A missing
// directory is not an error (tickets.Load reports it as zero epics), so it
// renders the same empty state as an absent `.scratch/`. It also computes
// the cheap archive count (tickets.CountArchivedEpics — a flat directory
// listing, no ticket parsing) so the Archived section's up-front count stays
// current on both manual refresh and the auto-refresh timer, even though
// ticket 04's lazy load is what actually populates m.archivedEpics. A count
// error is treated as zero rather than surfaced alongside msg.err, since a
// missing/unreadable ".archive" just means "nothing archived to show".
func (m Model) cmdLoad() tea.Cmd {
	scratchDir := m.scratchDir()
	return func() tea.Msg {
		epics, err := tickets.Load(scratchDir)
		archivedEpicCount, _ := tickets.CountArchivedEpics(scratchDir)
		return epicsLoadedMsg{epics: epics, err: err, archivedEpicCount: archivedEpicCount}
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
	// archived mirrors sidebarNode.archived — which of m.epics/m.archivedEpics
	// epicIdx indexes into. Read via epicAt(row) rather than directly.
	archived bool
}

func (r row) isEpic() bool { return r.ticketIdx < 0 }

// rowFromEntry adapts one sidebar tree.Entry into row's flat shape. A
// nodeSection entry has no row representation — a section header is
// cursor-reachable but isn't an epic or ticket, so there's nothing to check
// or preview — so it reports ok = false.
func rowFromEntry(e tree.Entry[sidebarNode]) (row, bool) {
	switch e.Value.kind {
	case nodeEpic:
		return row{epicIdx: e.Value.epicIdx, ticketIdx: -1, archived: e.Value.archived}, true
	case nodeTicket:
		return row{
			epicIdx:     e.Value.epicIdx,
			ticketIdx:   e.Value.ticketIdx,
			depth:       e.Depth - 2, // section, epic ancestors
			hasChildren: e.HasChildren,
			expanded:    e.Expanded,
			archived:    e.Value.archived,
		}, true
	default:
		return row{}, false
	}
}

// epicAt resolves r's epic against m.epics or m.archivedEpics depending on
// which one r.archived says it was sourced from — the single accessor every
// row-carrying call site uses instead of indexing m.epics directly, so a
// row sourced from the (ticket 04) Archived section resolves correctly too.
func (m Model) epicAt(r row) tickets.Epic {
	if r.archived {
		return m.archivedEpics[r.epicIdx]
	}
	return m.epics[r.epicIdx]
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

// sidebarSectionID is the tree.Entry ID for a section's nodeSection root row
// — the single source of truth idFn and closedEpicDefaults both share
// for "section:open"/"section:closed"/"section:archived".
func sidebarSectionID(section sidebarSection) string {
	switch section {
	case sectionOpen:
		return "section:open"
	case sectionClosed:
		return "section:closed"
	default:
		return "section:archived"
	}
}

// closedEpicDefaults is the sidebar's declared-default collapse policy: the
// "Closed epics" section starts collapsed, and every closed epic's own row
// (splitEpicIndexesBySection's AllDone rule) starts collapsed too — so
// expanding the section doesn't dump every closed epic's full ticket list
// on screen at once. An open epic gets no entry (defaults to expanded).
// Declared fresh from epics every call — never mutated or persisted — so an
// epic that reopens (a ticket moves back off done) simply stops appearing
// here on the very next call, with nothing to un-seed.
func closedEpicDefaults(epics []tickets.Epic, archivedEpics []tickets.Epic) map[string]bool {
	defaults := map[string]bool{
		sidebarSectionID(sectionClosed):   true,
		sidebarSectionID(sectionArchived): true,
	}
	for _, epic := range epics {
		if epic.AllDone() {
			defaults[epic.Path] = true
		}
	}
	for _, epic := range archivedEpics {
		defaults[epic.Path] = true
	}
	return defaults
}

// searchExpandOverrides is the transient half of the sidebar's collapse
// layering: every epic containing a query match (recomputeSearchMatches'
// own matching rule, duplicated here as a pure function so
// deriveCollapsedSidebar can call it without a *Model) reports its own path
// and containing section as wanting to render expanded. Declared fresh from
// epics/query every call — nil once query is empty, so it carries no state
// of its own between searches.
func searchExpandOverrides(epics []tickets.Epic, query string) map[string]bool {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return nil
	}
	var overrides map[string]bool
	for _, epic := range epics {
		matched := false
		for _, t := range epic.Tickets {
			text := strings.ToLower(t.Title + " " + epic.RenderedStatus(t).Word())
			if strings.Contains(text, q) {
				matched = true
				break
			}
		}
		if !matched {
			continue
		}
		if overrides == nil {
			overrides = map[string]bool{}
		}
		overrides[epic.Path] = false
		section := sectionOpen
		if epic.AllDone() {
			section = sectionClosed
		}
		overrides[sidebarSectionID(section)] = false
	}
	return overrides
}

// deriveCollapsedSidebar computes the sidebar's effective/derived collapse
// map fresh every call, from three layers: explicit (the user's own
// recorded expand/collapse/toggle choices — the only durable map, owned by
// Model.explicitCollapsed, never written to by this function or its
// callers) as the base; closedEpicDefaults layered under it via
// tree.ApplyDefaults (an ID absent from explicit takes its declared
// default; an ID already in explicit — however it got there — keeps that
// choice); then searchExpandOverrides layered on top, but only for IDs
// explicit hasn't touched, so a user's explicit toggle always wins over
// both the declared default and the transient search override, in either
// direction. The result is safe to hand straight to
// sidebarTree.SetCollapsedIDs — it must never be read back into explicit,
// or a default/override would calcify into a permanent choice the way the
// old read-back-and-write-back defaultCollapsedSidebar did.
func deriveCollapsedSidebar(explicit map[string]bool, epics []tickets.Epic, archivedEpics []tickets.Epic, query string) map[string]bool {
	effective := tree.ApplyDefaults(explicit, closedEpicDefaults(epics, archivedEpics))
	for id := range searchExpandOverrides(epics, query) {
		if _, ok := explicit[id]; ok {
			continue
		}
		effective[id] = false
	}
	return effective
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
