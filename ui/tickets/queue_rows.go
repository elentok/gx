package tickets

import (
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
}

func (m QueueModel) rows() []queueRow {
	rows, _ := m.rowsAndPlanErrors()
	return rows
}

// rowsAndPlanErrors lists every candidate ticket per epic in plan order —
// ticket-number order (sortedTicketIndexes), which follows each ticket's
// blocked_by chain, so the topmost open ticket in an epic is always the next
// one ralph-loop would actually claim — rather than batching them into
// synchronized "parallel"/"then" waves (ticket 25: that grouping read as a
// hard concurrency contract the runner didn't actually enforce). A plan
// validation still runs per epic via the same canonical planner
// (ralphloop.PlanWaves over a ralphloop.RunScope) the runner itself claims
// tickets from, so a dependency cycle or a blocker outside the selection that
// will never resolve is still caught — surfaced by name in planErrs for
// queueLines to render as an actionable error instead of a misleading plan.
func (m QueueModel) rowsAndPlanErrors() ([]queueRow, map[string]error) {
	candidates := m.candidates
	if candidates == nil {
		// Before queueEpicsLoadedMsg arrives, m.candidates hasn't been
		// initialized yet, but bubbletea can still call View (and thus this)
		// on the initial render. Fall back to a scratch map rather than
		// writing into the nil m.candidates.
		candidates = make(map[string]bool, len(m.checked))
	}
	for path := range m.checked {
		candidates[path] = true
	}
	var out []queueRow
	planErrs := make(map[string]error)
	for _, epic := range m.epics {
		waves, err := epicWaves(epic, candidates, m.settings.MaxConcurrentTicketsPerEpic())
		if err != nil {
			planErrs[epic.Name] = err
		}
		ordered := epicRowOrder(epic, waves, candidates)
		if m.hideComplete {
			ordered = filterDoneTickets(epic, ordered)
		}
		out = append(out, queueRowsForEpic(epic, ordered, m.queueTree.CollapsedIDs())...)
	}
	return out, planErrs
}

// queueNodeKind distinguishes the six row kinds the Queue tab's tree can
// hold: a per-epic blank separator, its status/context-window header lines,
// an optional plan-error line, a ticket, and a ticket's optional park-reason
// subtext. tree.Model[T] has no notion of a heterogeneous node — this sum
// type plus buildQueueEntries' idFn/childrenFn is what makes one
// tree.Model[queueNode] stand in for buildQueueLines' former hand-spliced
// header/blank/error strings interleaved with queueRowsForEpic's ticket
// rows.
type queueNodeKind int

const (
	nodeEpicSeparator     queueNodeKind = iota // blank line before every epic
	nodeEpicStatus                             // epic name + status icon/text (epicStatusLine)
	nodeEpicContext                            // context-window metrics line
	nodeEpicError                              // present only when planErrs[epic.Name] != nil
	nodeQueueTicket                            // wraps a queueRow
	nodeQueueTicketReason                      // park-reason subtext, sibling right after its nodeQueueTicket (queueTicketReasonLine)
)

// queueNode is ui/tree.Model[queueNode]'s value type. err is set only for
// nodeEpicError, ticket only for nodeQueueTicket.
type queueNode struct {
	kind   queueNodeKind
	epic   tickets.Epic
	err    error
	ticket queueRow
}

// buildQueueEntries flattens m.epics into the Queue tab's full tree.Entry
// list: each epic with at least one candidate ticket contributes a blank
// separator, its status/context-window header lines, an optional plan-error
// line, and its own candidate tickets nested by Parent/Children — mirroring
// buildSidebarEntries (model_data.go) one level down. Reuses
// epicWaves/epicRowOrder/filterDoneTickets/nearestVisibleQueueAncestor
// unchanged (rowsAndPlanErrors' own data-prep); only the final step —
// wrapping ordered tickets into queueNode tree entries instead of []queueRow
// via queueRowsForEpic — is new. Called on every
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
		for _, t := range ordered {
			row := queueRow{epic: epic, ticket: t}
			siblings := []queueNode{{kind: nodeQueueTicket, epic: epic, ticket: row}}
			if parkReason(epic, t, m.icons().Ellipsis) != "" {
				siblings = append(siblings, queueNode{kind: nodeQueueTicketReason, epic: epic, ticket: row})
			}
			if parentPath := nearestVisibleQueueAncestor(t, visible, byIdentifier); parentPath != "" {
				childrenOf[parentPath] = append(childrenOf[parentPath], siblings...)
			} else {
				roots = append(roots, siblings...)
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
		case nodeQueueTicketReason:
			return n.ticket.ticket.Path + ":reason"
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

	return tree.BuildEntriesFromValues(roots, idFn, childrenFn, m.queueTree.CollapsedIDs())
}

// filterDoneTickets drops a done ticket from ordered before nesting
// (queueRowsForEpic) rather than after, so a done parent doesn't strand its
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

// queueRowsForEpic nests ordered (epicRowOrder's flattened plan-order ticket
// list) under each ticket's Parent (ticket 03) via ui/tree's pure
// entry-builder (ticket 02), mirroring the Tickets tab's ticketRows (ticket
// 09) one level down. ordered is also the definition of "visible" here: a
// ticket's parent counts as a nesting target only if it's also present in
// ordered (still checked/planned, and not hideComplete-filtered), preserving
// ordered's own plan order among roots and among each parent's children.
func queueRowsForEpic(epic tickets.Epic, ordered []tickets.Ticket, collapsed map[string]bool) []queueRow {
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

	parentOf := make(map[string]string, len(ordered))
	childrenOf := make(map[string][]tickets.Ticket, len(ordered))
	var roots []tickets.Ticket
	for _, t := range ordered {
		parentPath := nearestVisibleQueueAncestor(t, visible, byIdentifier)
		parentOf[t.Path] = parentPath
		if parentPath == "" {
			roots = append(roots, t)
		} else {
			childrenOf[parentPath] = append(childrenOf[parentPath], t)
		}
	}

	idFn := func(t tickets.Ticket) string { return t.Path }
	childrenFn := func(t tickets.Ticket) []tickets.Ticket { return childrenOf[t.Path] }
	entries := tree.BuildEntriesFromValues(roots, idFn, childrenFn, collapsed)

	rows := make([]queueRow, len(entries))
	for i, e := range entries {
		rows[i] = queueRow{
			epic:        epic,
			ticket:      e.Value,
			depth:       e.Depth,
			hasChildren: e.HasChildren,
			expanded:    e.Expanded,
			parentPath:  parentOf[e.Value.Path],
		}
	}
	return rows
}

// nearestVisibleQueueAncestor walks t's Parent chain (ticket 03's schema
// field) up to the first ancestor present in visible, mirroring the Tickets
// tab's nearestVisibleAncestor (ticket 09, model_data.go) — keyed by
// Ticket.Path/Identifier rather than an epic.Tickets index since queueRow
// doesn't carry one. Returns "" once the chain runs out, hits a Parent token
// with no matching ticket in the epic, or would loop (guarded via seen).
func nearestVisibleQueueAncestor(t tickets.Ticket, visible map[string]bool, byIdentifier map[string]tickets.Ticket) string {
	seen := map[string]bool{t.Path: true}
	cur := t
	for cur.Parent != nil {
		parent, ok := byIdentifier[*cur.Parent]
		if !ok || seen[parent.Path] {
			return ""
		}
		if visible[parent.Path] {
			return parent.Path
		}
		seen[parent.Path] = true
		cur = parent
	}
	return ""
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
// Run uses to claim tickets — see rowsAndPlanErrors.
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
