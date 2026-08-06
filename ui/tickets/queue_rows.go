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
		out = append(out, queueRowsForEpic(epic, ordered, m.collapsedQueueTickets)...)
	}
	return out, planErrs
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
