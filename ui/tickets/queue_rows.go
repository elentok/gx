package tickets

import (
	"reflect"
	"slices"

	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/ui/tree"
)

// depth/hasChildren/expanded/parentPath mirror the Tickets tab's row (ticket
// 09, model_data.go) one level down for the Queue tab's own row model: a
// ticket nested under another ticket via Parent/Children (ticket 03).
// parentPath is "" for a top-level row, keyed by Ticket.Path (globally
// unique) rather than an epic.Tickets index since queueRow doesn't carry
// one.
type queueRow struct {
	epic        tickets.Epic
	ticket      tickets.Ticket
	depth       int
	hasChildren bool
	expanded    bool
	parentPath  string
	// actionable is false for a row injected by attachQueueAncestors to keep
	// a scheduling candidate nested under its real parent chain (ticket 08):
	// the ticket itself isn't a candidate, so renderQueueTicketRow dims it to
	// tell it apart from rows the user can actually act on.
	actionable bool
}

// queueNodeKind distinguishes the five row kinds the Queue tab's tree can
// hold: a per-epic blank separator, its status/context-window header lines,
// an optional plan-error line, and a ticket. tree.Model[T] has no notion of a
// heterogeneous node — this sum type plus buildQueueEntries' idFn/childrenFn
// is what makes one tree.Model[queueNode] stand in for buildQueueLines'
// former hand-spliced header/blank/error strings interleaved with the ticket
// rows. A ticket's park-reason subtext is not a node of its own — it's a
// second physical line on the ticket's own entry (queueTicketReasonLine, set
// as Entry.Body in buildQueueEntries).
type queueNodeKind int

const (
	nodeEpicSeparator queueNodeKind = iota // blank line before every epic
	nodeEpicStatus                         // epic name + status icon/text (epicStatusLine)
	nodeEpicContext                        // context-window metrics line
	nodeEpicError                          // present only when planErrs[epic.Name] != nil
	nodeQueueTicket                        // wraps a queueRow
)

// queueNode is ui/tree.Model[queueNode]'s value type. err is set only for
// nodeEpicError, ticket only for nodeQueueTicket.
type queueNode struct {
	kind   queueNodeKind
	epic   tickets.Epic
	err    error
	ticket queueRow
}

// queueEntriesCache memoizes buildQueueEntries' output (see QueueModel.
// entriesCache) against the four inputs that actually change its structure.
// Anything read live at draw time instead (running-epic elapsed seconds,
// spinner frame, parked/stalled state — queueRenderOpts' Label callback) is
// excluded on purpose: it doesn't affect which entries exist, only how a
// cached entry's row text renders, so it can't go stale from reusing this
// cache.
type queueEntriesCache struct {
	epics        []tickets.Epic
	checked      map[string]bool
	hideComplete bool
	collapsed    map[string]bool
	entries      []tree.Entry[queueNode]
}

// buildQueueEntriesCached returns buildQueueEntries' result, reusing the
// previous build when none of its inputs changed since the last render.
// Queue is polled every 2s by cmdAutoRefresh (auto_refresh.go) whether or
// not anything on disk actually changed, and View() (queue_view.go) must
// call this on every single render regardless — so without this cache an
// idle Queue tab left open still redoes the full tree rebuild (icons,
// labels, per-ticket wave/order computation) forever.
func (m QueueModel) buildQueueEntriesCached() []tree.Entry[queueNode] {
	if m.entriesCache == nil {
		return m.buildQueueEntries()
	}
	collapsed := m.queueTree.CollapsedIDs()
	c := m.entriesCache
	if c.entries != nil &&
		c.hideComplete == m.hideComplete &&
		reflect.DeepEqual(c.epics, m.epics) &&
		reflect.DeepEqual(c.checked, m.checked) &&
		reflect.DeepEqual(c.collapsed, collapsed) {
		return c.entries
	}
	entries := m.buildQueueEntries()
	c.epics = m.epics
	c.checked = m.checked
	c.hideComplete = m.hideComplete
	c.collapsed = collapsed
	c.entries = entries
	return entries
}

// buildQueueEntries flattens m.epics into the Queue tab's full tree.Entry
// list: each epic with at least one candidate ticket contributes a blank
// separator, its status/context-window header lines, an optional plan-error
// line, and its own candidate tickets nested by Parent/Children — mirroring
// buildSidebarEntries (model_data.go) one level down. Reuses
// epicWaves/epicRowOrder/filterDoneTickets/attachQueueAncestors to
// compute each epic's plan-ordered candidate tickets, then wraps them into
// queueNode tree entries. Called on every
// queueEpicsLoadedMsg/hideComplete toggle/collapse mutation via
// clampSelected.
func (m QueueModel) buildQueueEntries() []tree.Entry[queueNode] {
	candidates := m.candidates
	if candidates == nil {
		candidates = make(map[string]bool, len(m.checked))
	}
	for path := range m.checked {
		candidates[path] = true
	}

	var roots []queueNode
	childrenOf := map[string][]queueNode{}

	for _, epic := range m.epics {
		waves, planErr := epicWaves(epic, candidates, m.settings.MaxConcurrentTicketsPerEpic())
		ordered := epicRowOrder(epic, waves, candidates)
		if m.hideComplete {
			ordered = filterDoneTickets(epic, ordered)
		}
		if len(ordered) == 0 {
			continue
		}

		roots = append(roots, queueNode{kind: nodeEpicSeparator, epic: epic})
		roots = append(roots, queueNode{kind: nodeEpicStatus, epic: epic})
		roots = append(roots, queueNode{kind: nodeEpicContext, epic: epic})
		if planErr != nil {
			roots = append(roots, queueNode{kind: nodeEpicError, epic: epic, err: planErr})
		}

		visible := make(map[string]bool, len(ordered))
		for _, t := range ordered {
			visible[t.Path] = true
		}
		byIdentifier := make(map[string]tickets.Ticket, len(epic.Tickets))
		for _, t := range epic.Tickets {
			if t.Identifier != "" {
				byIdentifier[t.Identifier] = t
			}
		}
		injected := map[string]bool{}
		for _, t := range ordered {
			row := queueRow{epic: epic, ticket: t, actionable: true}
			node := queueNode{kind: nodeQueueTicket, epic: epic, ticket: row}
			parentPath := attachQueueAncestors(t, epic, visible, injected, byIdentifier, childrenOf, &roots)
			if parentPath != "" {
				childrenOf[parentPath] = append(childrenOf[parentPath], node)
			} else {
				roots = append(roots, node)
			}
		}
	}

	// tree.BuildEntriesFromValues computes each Entry's own HasChildren/Expanded
	// from childrenFn/collapsed, but that's only visible on the returned
	// tree.Entry — nothing feeds it back into queueNode.ticket, which
	// renderQueueTicketRow reads its fold triangle from (mirroring the
	// sidebar's rowFromEntry, model_data.go, one level down). Populate it here,
	// now that childrenOf is complete for every epic.
	collapsed := m.queueTree.CollapsedIDs()
	setFoldState := func(n *queueNode) {
		if n.kind != nodeQueueTicket {
			return
		}
		n.ticket.hasChildren = len(childrenOf[n.ticket.ticket.Path]) > 0
		n.ticket.expanded = n.ticket.hasChildren && !collapsed[n.ticket.ticket.Path]
	}
	for i := range roots {
		setFoldState(&roots[i])
	}
	for path := range childrenOf {
		for i := range childrenOf[path] {
			setFoldState(&childrenOf[path][i])
		}
	}

	idFn := func(n queueNode) string {
		switch n.kind {
		case nodeEpicSeparator:
			return "epic:" + n.epic.Name + ":sep"
		case nodeEpicStatus:
			return "epic:" + n.epic.Name + ":status"
		case nodeEpicContext:
			return "epic:" + n.epic.Name + ":context"
		case nodeEpicError:
			return "epic:" + n.epic.Name + ":err"
		default:
			return n.ticket.ticket.Path
		}
	}
	childrenFn := func(n queueNode) []queueNode {
		if n.kind != nodeQueueTicket {
			return nil
		}
		return childrenOf[n.ticket.ticket.Path]
	}

	entries := tree.BuildEntriesFromValues(roots, idFn, childrenFn, m.queueTree.CollapsedIDs())
	for i := range entries {
		if entries[i].Value.kind != nodeQueueTicket {
			continue
		}
		if line, ok := m.queueTicketReasonLine(entries[i].Value.ticket); ok {
			entries[i].Body = []string{line}
		}
	}
	return entries
}

// filterDoneTickets drops a done ticket from ordered before nesting
// (buildQueueEntries) rather than after, so a done parent doesn't strand its
// children — nearestVisibleQueueAncestor reattaches them to the nearest
// surviving ancestor instead of losing them.
func filterDoneTickets(epic tickets.Epic, ordered []tickets.Ticket) []tickets.Ticket {
	filtered := make([]tickets.Ticket, 0, len(ordered))
	for _, t := range ordered {
		if epic.RenderedStatus(t) == tickets.StatusDone {
			continue
		}
		filtered = append(filtered, t)
	}
	return filtered
}

// attachQueueAncestors walks t's Parent chain (ticket 03's schema field)
// looking for the nearest ancestor already in visible, mirroring the Tickets
// tab's nearestVisibleAncestor (ticket 09, model_data.go) one level down —
// keyed by Ticket.Path/Identifier rather than an epic.Tickets index since
// queueRow doesn't carry one. Unlike a plain lookup, any ancestor found along
// the way that ISN'T a scheduling candidate gets injected into roots/
// childrenOf as a dimmed (actionable: false) row of its own instead of being
// skipped (ticket 08) — otherwise t would get promoted to root and the
// parent-child connection the tree exists to show would be lost. injected
// dedupes across tickets sharing the same non-candidate ancestor within an
// epic, and visible is extended in place so a later call sees an ancestor
// this call just injected as already visible. Returns "" once the chain runs
// out, hits a Parent token with no matching ticket in the epic, or would loop
// (guarded via seen) — same fallback as before: t is promoted to root.
func attachQueueAncestors(t tickets.Ticket, epic tickets.Epic, visible map[string]bool, injected map[string]bool, byIdentifier map[string]tickets.Ticket, childrenOf map[string][]queueNode, roots *[]queueNode) string {
	seen := map[string]bool{t.Path: true}
	var chain []tickets.Ticket // non-candidate ancestors to inject, nearest first
	cur := t
	attachPoint := ""
	for cur.Parent != nil {
		parent, ok := byIdentifier[*cur.Parent]
		if !ok || seen[parent.Path] {
			break
		}
		if visible[parent.Path] {
			attachPoint = parent.Path
			break
		}
		chain = append(chain, parent)
		seen[parent.Path] = true
		cur = parent
	}

	// Inject farthest-first so each new dimmed row's own parent is already
	// attached (either an existing visible ancestor or an injected one from
	// an earlier iteration of this same loop) before it's linked in.
	parentPath := attachPoint
	for _, ancestor := range slices.Backward(chain) {
		if !injected[ancestor.Path] {
			node := queueNode{
				kind:   nodeQueueTicket,
				epic:   epic,
				ticket: queueRow{epic: epic, ticket: ancestor, actionable: false},
			}
			if parentPath != "" {
				childrenOf[parentPath] = append(childrenOf[parentPath], node)
			} else {
				*roots = append(*roots, node)
			}
			injected[ancestor.Path] = true
			visible[ancestor.Path] = true
		}
		parentPath = ancestor.Path
	}
	return parentPath
}

// epicRowOrder flattens waves (blockers before dependents, ties in ticket
// number order per PlanWaves) into the epic's Queue row order. When waves is
// nil — epicWaves errored, e.g. a dependency cycle — it falls back to plain
// ticket-number order so the epic's candidates still render instead of
// vanishing from the list.
func epicRowOrder(epic tickets.Epic, waves [][]tickets.Ticket, candidates map[string]bool) []tickets.Ticket {
	if waves == nil {
		var fallback []tickets.Ticket
		for _, idx := range sortedTicketIndexes(epic) {
			t := epic.Tickets[idx]
			if candidates[t.Path] {
				fallback = append(fallback, t)
			}
		}
		return fallback
	}
	var out []tickets.Ticket
	for _, wave := range waves {
		out = append(out, wave...)
	}
	return out
}

// epicWaves resolves candidates (the checked-ticket paths within epic) into a
// ralphloop.RunScope and hands off to ralphloop.PlanWaves, the same planner
// Run uses to claim tickets — see buildQueueEntries.
func epicWaves(epic tickets.Epic, candidates map[string]bool, maxParallel int) ([][]tickets.Ticket, error) {
	var ids []string
	for _, idx := range sortedTicketIndexes(epic) {
		t := epic.Tickets[idx]
		if candidates[t.Path] {
			ids = append(ids, t.DisplayNumber())
		}
	}
	if len(ids) == 0 {
		return nil, nil
	}
	scope, err := ralphloop.ResolveRunScope(epic, ids)
	if err != nil {
		return nil, err
	}
	return ralphloop.PlanWaves(epic, scope, maxParallel)
}
