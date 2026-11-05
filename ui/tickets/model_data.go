package tickets

import (
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/ui/notify"
	"github.com/elentok/gx/ui/tickets/rowfmt"
	"github.com/elentok/gx/ui/tickets/tabapi"
	"github.com/elentok/gx/ui/tree"
)

// noParentTicket is nearestVisibleAncestor's sentinel for a ticket that isn't
// nested under another ticket (either a top-level ticket, or a nested one
// whose containing ancestor was filtered out by hideDone).
const noParentTicket = -1

// sidebarNodeKind distinguishes the three row kinds the sidebar tree can
// hold: a section header ("Open epics", "Merged epics", …), an epic, or a
// ticket. tree.Model[T] itself has no notion of a heterogeneous node — this
// sum type plus idFn/childrenFn is what makes one tree.Model[sidebarNode]
// stand in for what used to be two hand-flattened levels (epics, then each
// epic's own tickets).
type sidebarNodeKind int

const (
	nodeSection sidebarNodeKind = iota
	nodeEpic
	nodeTicket
	nodeLoading   // "loading…" placeholder, child of the Archived section while its lazy load is in flight
	nodeLoadError // inline error placeholder, child of the Archived section after a failed lazy load
)

// sidebarSection is which section-header root a sidebarNode belongs under.
// Declaration order is the sidebar's render order.
type sidebarSection int

const (
	sectionOpen sidebarSection = iota
	sectionMapsInProgress
	sectionOpenMaps
	sectionComplete
	sectionMerged
	sectionCompleteMaps
	sectionArchived
	sectionCount
)

// sectionCollapsedByDefault marks the sections holding finished work: they
// start collapsed so what still needs merging stays on screen.
var sectionCollapsedByDefault = map[sidebarSection]bool{
	sectionComplete:     true,
	sectionMerged:       true,
	sectionCompleteMaps: true,
	sectionArchived:     true,
}

// epicSection classifies an active (non-archived) epic. A zero-ticket epic is
// never AllDone, so it stays open.
func epicSection(epic tickets.Epic) sidebarSection {
	switch {
	case epic.IsMap && epic.AllDone():
		return sectionCompleteMaps
	case epic.IsMap && hasDoneTicket(epic):
		return sectionMapsInProgress
	case epic.IsMap:
		return sectionOpenMaps
	case !epic.AllDone():
		return sectionOpen
	case !epic.MergedAt.IsZero():
		return sectionMerged
	default:
		return sectionComplete
	}
}

// hasDoneTicket reports whether any ticket renders as done. Cancelled and
// waiting-for-children tickets don't count.
func hasDoneTicket(epic tickets.Epic) bool {
	return slices.ContainsFunc(epic.Tickets, func(t tickets.Ticket) bool {
		return epic.RenderedStatus(t) == tickets.StatusDone
	})
}

// groupEpicIndexesBySection buckets m.epics' indexes by epicSection,
// preserving input order within each section.
func groupEpicIndexesBySection(epics []tickets.Epic) map[sidebarSection][]int {
	groups := make(map[sidebarSection][]int, sectionCount)
	for i, epic := range epics {
		s := epicSection(epic)
		groups[s] = append(groups[s], i)
	}
	return groups
}

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
// Epic value directly (rather than an index into s.epics) so it works
// equally for s.archivedEpics — indices in that slice mean nothing against
// s.epics.
func (s sidebar) buildEpicTicketTree(epic tickets.Epic) epicTicketTree {
	sorted := rowfmt.SortedTicketIndexes(epic)

	visible := make(map[int]bool, len(sorted))
	for _, idx := range sorted {
		if s.hideDone && rowfmt.TicketStatus(epic, epic.Tickets[idx]).Terminal() {
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
// list: one nodeSection root per non-empty section (per epicSection), each
// epic's root tickets beneath its nodeEpic entry, and each ticket's nested
// children (epicTicketTree.childrenOf) beneath its nodeTicket entry. Called
// on every epicsLoadedMsg/hideDone toggle/collapse mutation via
// clampSelected.
func (s sidebar) buildSidebarEntries(env sidebarEnv) []tree.Entry[sidebarNode] {
	groups := groupEpicIndexesBySection(s.epics)

	epicTreesOpen := make(map[int]epicTicketTree, len(s.epics))
	epicTreesArchived := make(map[int]epicTicketTree, len(s.archivedEpics))
	epicTree := func(archived bool, epicIdx int) epicTicketTree {
		cache := epicTreesOpen
		epics := s.epics
		if archived {
			cache, epics = epicTreesArchived, s.archivedEpics
		}
		t, ok := cache[epicIdx]
		if !ok {
			t = s.buildEpicTicketTree(epics[epicIdx])
			cache[epicIdx] = t
		}
		return t
	}

	var roots []sidebarNode
	for section := range sectionCount {
		if len(groups[section]) == 0 && (section != sectionArchived || s.archivedEpicCount == 0) {
			continue
		}
		roots = append(roots, sidebarNode{kind: nodeSection, section: section, ticketIdx: -1})
	}

	idFn := func(n sidebarNode) string {
		switch n.kind {
		case nodeSection:
			return sidebarSectionID(n.section)
		case nodeLoading:
			return sidebarSectionID(n.section) + ":loading"
		case nodeLoadError:
			return sidebarSectionID(n.section) + ":error"
		case nodeEpic:
			return s.epicAt(row{epicIdx: n.epicIdx, archived: n.archived}).Path
		default:
			return s.epicAt(row{epicIdx: n.epicIdx, archived: n.archived}).Tickets[n.ticketIdx].Path
		}
	}

	childrenFn := func(n sidebarNode) []sidebarNode {
		switch n.kind {
		case nodeSection:
			if n.section == sectionArchived {
				return s.archivedSectionChildren()
			}
			order := groups[n.section]
			children := make([]sidebarNode, len(order))
			for i, epicIdx := range order {
				children[i] = sidebarNode{kind: nodeEpic, epicIdx: epicIdx, ticketIdx: -1}
			}
			return children
		case nodeLoading, nodeLoadError:
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

	entries := tree.BuildEntriesFromValues(roots, idFn, childrenFn, s.tree.CollapsedIDs())
	for i := range entries {
		if line, ok := s.pendingSubtext(env, entries[i]); ok {
			entries[i].Body = []string{line}
		}
	}
	return entries
}

// pendingSubtext is a queued ticket row's explain verdict, indented under the
// row's title. ok is false for any entry that isn't a queued ticket.
func (s sidebar) pendingSubtext(env sidebarEnv, entry tree.Entry[sidebarNode]) (string, bool) {
	if entry.Value.kind != nodeTicket {
		return "", false
	}
	r, _ := rowFromEntry(entry)
	epic := s.epicAt(r)
	t := epic.Tickets[r.ticketIdx]
	text, ok := env.pendingText(t.Path)
	if !ok {
		return "", false
	}
	icon, _ := rowfmt.StatusIconAndStyle(env.icons, rowfmt.TicketStatus(epic, t))
	width := rowfmt.TriangleColumnWidth(env.icons) + 1 + lipgloss.Width(icon) + 1
	return strings.Repeat(" ", width) + rowfmt.BlockedBySuffixStyle.Render(rowfmt.Ellipsize(text, rowfmt.PauseReasonMaxRunes, env.icons.Ellipsis)), true
}

// archivedSectionChildren computes the Archived section header's synthetic
// children: a loading/error placeholder or the real archived-epic rows
// depending on s.archivedLazy's current state. The section itself is hidden
// while the up-front count is zero.
func (s sidebar) archivedSectionChildren() []sidebarNode {
	switch s.archivedLazy.State() {
	case tree.LazyLoaded:
		children := make([]sidebarNode, len(s.archivedEpics))
		for i := range s.archivedEpics {
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

// cmdLoad reloads from wherever the rows currently come from: the server's
// snapshot, or the store itself in down mode.
func (m Model) cmdLoad() tea.Cmd {
	if m.readsDisk() {
		return m.cmdLoadDisk()
	}
	return func() tea.Msg { return tabapi.ResnapshotRequestedMsg{} }
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

// epicAt resolves r's epic against s.epics or s.archivedEpics depending on
// which one r.archived says it was sourced from — the single accessor every
// row-carrying call site uses instead of indexing s.epics directly, so a
// row sourced from the (ticket 04) Archived section resolves correctly too.
func (s sidebar) epicAt(r row) tickets.Epic {
	if r.archived {
		return s.archivedEpics[r.epicIdx]
	}
	return s.epics[r.epicIdx]
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

func (s sidebar) isCollapsed(epic tickets.Epic) bool {
	return s.tree.CollapsedIDs()[epic.Path]
}

// sidebarSectionID is the tree.Entry ID for a section's nodeSection root row
// — the single source of truth idFn and epicCollapseDefaults both share.
func sidebarSectionID(section sidebarSection) string {
	switch section {
	case sectionOpen:
		return "section:open"
	case sectionMapsInProgress:
		return "section:maps-in-progress"
	case sectionOpenMaps:
		return "section:open-maps"
	case sectionComplete:
		return "section:complete"
	case sectionMerged:
		return "section:merged"
	case sectionCompleteMaps:
		return "section:complete-maps"
	default:
		return "section:archived"
	}
}

// epicCollapseDefaults is the sidebar's declared-default collapse policy: the
// sectionCollapsedByDefault sections start collapsed, and every epic's own
// row starts collapsed too, open or closed — so the sidebar first shows one
// row per epic instead of every ticket. Declared fresh from epics every call
// — never mutated or persisted.
func epicCollapseDefaults(epics []tickets.Epic, archivedEpics []tickets.Epic) map[string]bool {
	defaults := map[string]bool{}
	for s := range sectionCollapsedByDefault {
		defaults[sidebarSectionID(s)] = true
	}
	for _, epic := range epics {
		defaults[epic.Path] = true
	}
	for _, epic := range archivedEpics {
		defaults[epic.Path] = true
	}
	return defaults
}

// searchExpandOverrides is the transient half of the sidebar's collapse
// layering: every epic containing a ticket match reports its own path and
// containing section as wanting to render expanded, and an epic whose own
// row matches reports just its section (so the row is reachable without
// unfolding its tickets). Declared fresh from epics/query every call — nil
// once query is empty, so it carries no state of its own between searches.
func searchExpandOverrides(epics []tickets.Epic, query string) map[string]bool {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return nil
	}
	var overrides map[string]bool
	for _, epic := range epics {
		ticketMatched := false
		for _, t := range epic.Tickets {
			if rowfmt.TicketMatchesQuery(epic, t, q) {
				ticketMatched = true
				break
			}
		}
		if !ticketMatched && !rowfmt.EpicMatchesQuery(epic, q) {
			continue
		}
		if overrides == nil {
			overrides = map[string]bool{}
		}
		if ticketMatched {
			overrides[epic.Path] = false
		}
		overrides[sidebarSectionID(epicSection(epic))] = false
	}
	return overrides
}

// deriveCollapsedSidebar computes the sidebar's effective/derived collapse
// map fresh every call, from three layers: explicit (the user's own
// recorded expand/collapse/toggle choices — the only durable map, owned by
// Model.explicitCollapsed, never written to by this function or its
// callers) as the base; epicCollapseDefaults layered under it via
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
	effective := tree.ApplyDefaults(explicit, epicCollapseDefaults(epics, archivedEpics))
	for id := range searchExpandOverrides(epics, query) {
		if _, ok := explicit[id]; ok {
			continue
		}
		effective[id] = false
	}
	return effective
}
