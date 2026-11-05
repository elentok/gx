package sidebar

import (
	"slices"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/ui/tickets/rowfmt"
	"github.com/elentok/gx/ui/tree"
)

// noParentTicket is nearestVisibleAncestor's sentinel for a ticket that isn't
// nested under another ticket (either a top-level ticket, or a nested one
// whose containing ancestor was filtered out by hideDone).
const noParentTicket = -1

// NodeKind distinguishes the three row kinds the sidebar tree can
// hold: a section header ("Open epics", "Merged epics", …), an epic, or a
// ticket. tree.Model[T] itself has no notion of a heterogeneous node — this
// sum type plus idFn/childrenFn is what makes one tree.Model[sidebarNode]
// stand in for what used to be two hand-flattened levels (epics, then each
// epic's own tickets).
type NodeKind int

const (
	NodeSection NodeKind = iota
	NodeEpic
	NodeTicket
	NodeLoading   // "loading…" placeholder, child of the Archived section while its lazy load is in flight
	NodeLoadError // inline error placeholder, child of the Archived section after a failed lazy load
)

// Section is which section-header root a sidebarNode belongs under.
// Declaration order is the sidebar's render order.
type Section int

const (
	SectionOpen Section = iota
	SectionMapsInProgress
	SectionOpenMaps
	SectionComplete
	SectionMerged
	SectionCompleteMaps
	SectionArchived
	SectionCount
)

// SectionCollapsedByDefault marks the sections holding finished work: they
// start collapsed so what still needs merging stays on screen.
var SectionCollapsedByDefault = map[Section]bool{
	SectionComplete:     true,
	SectionMerged:       true,
	SectionCompleteMaps: true,
	SectionArchived:     true,
}

// EpicSection classifies an active (non-archived) epic. A zero-ticket epic is
// never AllDone, so it stays open.
func EpicSection(epic tickets.Epic) Section {
	switch {
	case epic.IsMap && epic.AllDone():
		return SectionCompleteMaps
	case epic.IsMap && hasDoneTicket(epic):
		return SectionMapsInProgress
	case epic.IsMap:
		return SectionOpenMaps
	case !epic.AllDone():
		return SectionOpen
	case !epic.MergedAt.IsZero():
		return SectionMerged
	default:
		return SectionComplete
	}
}

// hasDoneTicket reports whether any ticket renders as done. Cancelled and
// waiting-for-children tickets don't count.
func hasDoneTicket(epic tickets.Epic) bool {
	return slices.ContainsFunc(epic.Tickets, func(t tickets.Ticket) bool {
		return epic.RenderedStatus(t) == tickets.StatusDone
	})
}

// groupEpicIndexesBySection buckets m.epics' indexes by EpicSection,
// preserving input order within each section.
func groupEpicIndexesBySection(epics []tickets.Epic) map[Section][]int {
	groups := make(map[Section][]int, SectionCount)
	for i, epic := range epics {
		s := EpicSection(epic)
		groups[s] = append(groups[s], i)
	}
	return groups
}

// Node is ui/tree.Model[Node]'s value type. ticketIdx is -1 for
// nodeSection/nodeEpic. archived marks a node sourced from m.archivedEpics
// rather than m.epics — always false until ticket 04's Archived section
// starts populating entries from it; carried through to row (rowFromEntry)
// so epicAt(row) knows which slice to resolve against.
type Node struct {
	Kind      NodeKind
	Section   Section
	EpicIdx   int
	TicketIdx int
	Archived  bool
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
func (s Model) buildEpicTicketTree(epic tickets.Epic) epicTicketTree {
	sorted := rowfmt.SortedTicketIndexes(epic)

	visible := make(map[int]bool, len(sorted))
	for _, idx := range sorted {
		if s.HideDone && rowfmt.TicketStatus(epic, epic.Tickets[idx]).Terminal() {
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

// BuildSidebarEntries flattens m.epics into the sidebar's full tree.Entry
// list: one nodeSection root per non-empty section (per EpicSection), each
// epic's root tickets beneath its nodeEpic entry, and each ticket's nested
// children (epicTicketTree.childrenOf) beneath its nodeTicket entry. Called
// on every epicsLoadedMsg/hideDone toggle/collapse mutation via
// clampSelected.
func (s Model) BuildSidebarEntries(env Env) []tree.Entry[Node] {
	groups := groupEpicIndexesBySection(s.Epics)

	epicTreesOpen := make(map[int]epicTicketTree, len(s.Epics))
	epicTreesArchived := make(map[int]epicTicketTree, len(s.ArchivedEpics))
	epicTree := func(archived bool, epicIdx int) epicTicketTree {
		cache := epicTreesOpen
		epics := s.Epics
		if archived {
			cache, epics = epicTreesArchived, s.ArchivedEpics
		}
		t, ok := cache[epicIdx]
		if !ok {
			t = s.buildEpicTicketTree(epics[epicIdx])
			cache[epicIdx] = t
		}
		return t
	}

	var roots []Node
	for section := range SectionCount {
		if len(groups[section]) == 0 && (section != SectionArchived || s.ArchivedEpicCount == 0) {
			continue
		}
		roots = append(roots, Node{Kind: NodeSection, Section: section, TicketIdx: -1})
	}

	idFn := func(n Node) string {
		switch n.Kind {
		case NodeSection:
			return SectionID(n.Section)
		case NodeLoading:
			return SectionID(n.Section) + ":loading"
		case NodeLoadError:
			return SectionID(n.Section) + ":error"
		case NodeEpic:
			return s.EpicAt(Row{EpicIdx: n.EpicIdx, Archived: n.Archived}).Path
		default:
			return s.EpicAt(Row{EpicIdx: n.EpicIdx, Archived: n.Archived}).Tickets[n.TicketIdx].Path
		}
	}

	childrenFn := func(n Node) []Node {
		switch n.Kind {
		case NodeSection:
			if n.Section == SectionArchived {
				return s.archivedSectionChildren()
			}
			order := groups[n.Section]
			children := make([]Node, len(order))
			for i, epicIdx := range order {
				children[i] = Node{Kind: NodeEpic, EpicIdx: epicIdx, TicketIdx: -1}
			}
			return children
		case NodeLoading, NodeLoadError:
			return nil
		case NodeEpic:
			roots := epicTree(n.Archived, n.EpicIdx).roots
			children := make([]Node, len(roots))
			for i, idx := range roots {
				children[i] = Node{Kind: NodeTicket, EpicIdx: n.EpicIdx, TicketIdx: idx, Archived: n.Archived}
			}
			return children
		default:
			kids := epicTree(n.Archived, n.EpicIdx).childrenOf[n.TicketIdx]
			children := make([]Node, len(kids))
			for i, idx := range kids {
				children[i] = Node{Kind: NodeTicket, EpicIdx: n.EpicIdx, TicketIdx: idx, Archived: n.Archived}
			}
			return children
		}
	}

	entries := tree.BuildEntriesFromValues(roots, idFn, childrenFn, s.Tree.CollapsedIDs())
	for i := range entries {
		if line, ok := s.pendingSubtext(env, entries[i]); ok {
			entries[i].Body = []string{line}
		}
	}
	return entries
}

// pendingSubtext is a queued ticket row's explain verdict, indented under the
// row's title. ok is false for any entry that isn't a queued ticket.
func (s Model) pendingSubtext(env Env, entry tree.Entry[Node]) (string, bool) {
	if entry.Value.Kind != NodeTicket {
		return "", false
	}
	r, _ := RowFromEntry(entry)
	epic := s.EpicAt(r)
	t := epic.Tickets[r.TicketIdx]
	text, ok := env.PendingText(t.Path)
	if !ok {
		return "", false
	}
	icon, _ := rowfmt.StatusIconAndStyle(env.Icons, rowfmt.TicketStatus(epic, t))
	width := rowfmt.TriangleColumnWidth(env.Icons) + 1 + lipgloss.Width(icon) + 1
	return strings.Repeat(" ", width) + rowfmt.BlockedBySuffixStyle.Render(rowfmt.Ellipsize(text, rowfmt.PauseReasonMaxRunes, env.Icons.Ellipsis)), true
}

// archivedSectionChildren computes the Archived section header's synthetic
// children: a loading/error placeholder or the real archived-epic rows
// depending on s.archivedLazy's current state. The section itself is hidden
// while the up-front count is zero.
func (s Model) archivedSectionChildren() []Node {
	switch s.ArchivedLazy.State() {
	case tree.LazyLoaded:
		children := make([]Node, len(s.ArchivedEpics))
		for i := range s.ArchivedEpics {
			children[i] = Node{Kind: NodeEpic, EpicIdx: i, TicketIdx: -1, Archived: true}
		}
		return children
	case tree.LazyFailed:
		return []Node{{Kind: NodeLoadError, Section: SectionArchived, TicketIdx: -1}}
	default:
		return []Node{{Kind: NodeLoading, Section: SectionArchived, TicketIdx: -1}}
	}
}

// Row is the shim view.go's render functions and model_preview_focus.go's
// selectedRow() consumers read, adapted from a tree.Entry[sidebarNode] by
// rowFromEntry. depth/hasChildren/expanded mirror ui/tree's Entry (ticket 02)
// for a ticket Row nested under another ticket via Parent/Children (ticket
// 03): depth 0 is a ticket directly under the epic (rowFromEntry subtracts
// the section+epic ancestor levels from the entry's own Depth), and
// hasChildren/expanded drive the Row's own fold glyph.
type Row struct {
	EpicIdx     int
	TicketIdx   int // -1 for an epic row
	Depth       int
	HasChildren bool
	Expanded    bool
	// Archived mirrors sidebarNode.Archived — which of m.epics/m.archivedEpics
	// epicIdx indexes into. Read via epicAt(row) rather than directly.
	Archived bool
}

func (r Row) IsEpic() bool { return r.TicketIdx < 0 }

// RowFromEntry adapts one sidebar tree.Entry into row's flat shape. A
// nodeSection entry has no row representation — a section header is
// cursor-reachable but isn't an epic or ticket, so there's nothing to check
// or preview — so it reports ok = false.
func RowFromEntry(e tree.Entry[Node]) (Row, bool) {
	switch e.Value.Kind {
	case NodeEpic:
		return Row{EpicIdx: e.Value.EpicIdx, TicketIdx: -1, Archived: e.Value.Archived}, true
	case NodeTicket:
		return Row{
			EpicIdx:     e.Value.EpicIdx,
			TicketIdx:   e.Value.TicketIdx,
			Depth:       e.Depth - 2, // section, epic ancestors
			HasChildren: e.HasChildren,
			Expanded:    e.Expanded,
			Archived:    e.Value.Archived,
		}, true
	default:
		return Row{}, false
	}
}

// EpicAt resolves r's epic against s.epics or s.archivedEpics depending on
// which one r.archived says it was sourced from — the single accessor every
// row-carrying call site uses instead of indexing s.epics directly, so a
// row sourced from the (ticket 04) Archived section resolves correctly too.
func (s Model) EpicAt(r Row) tickets.Epic {
	if r.Archived {
		return s.ArchivedEpics[r.EpicIdx]
	}
	return s.Epics[r.EpicIdx]
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

func (s Model) IsCollapsed(epic tickets.Epic) bool {
	return s.Tree.CollapsedIDs()[epic.Path]
}

// SectionID is the tree.Entry ID for a section's nodeSection root row
// — the single source of truth idFn and epicCollapseDefaults both share.
func SectionID(section Section) string {
	switch section {
	case SectionOpen:
		return "section:open"
	case SectionMapsInProgress:
		return "section:maps-in-progress"
	case SectionOpenMaps:
		return "section:open-maps"
	case SectionComplete:
		return "section:complete"
	case SectionMerged:
		return "section:merged"
	case SectionCompleteMaps:
		return "section:complete-maps"
	default:
		return "section:archived"
	}
}

// epicCollapseDefaults is the sidebar's declared-default collapse policy: the
// SectionCollapsedByDefault sections start collapsed, and every epic's own
// row starts collapsed too, open or closed — so the sidebar first shows one
// row per epic instead of every ticket. Declared fresh from epics every call
// — never mutated or persisted.
func epicCollapseDefaults(epics []tickets.Epic, archivedEpics []tickets.Epic) map[string]bool {
	defaults := map[string]bool{}
	for s := range SectionCollapsedByDefault {
		defaults[SectionID(s)] = true
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
		overrides[SectionID(EpicSection(epic))] = false
	}
	return overrides
}

// DeriveCollapsed computes the sidebar's effective/derived collapse
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
func DeriveCollapsed(explicit map[string]bool, epics []tickets.Epic, archivedEpics []tickets.Epic, query string) map[string]bool {
	effective := tree.ApplyDefaults(explicit, epicCollapseDefaults(epics, archivedEpics))
	for id := range searchExpandOverrides(epics, query) {
		if _, ok := explicit[id]; ok {
			continue
		}
		effective[id] = false
	}
	return effective
}
