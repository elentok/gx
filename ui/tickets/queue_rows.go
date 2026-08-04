package tickets

import (
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/tickets"
)

type queueRow struct {
	epic   tickets.Epic
	ticket tickets.Ticket
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
		if _, err := epicWaves(epic, candidates, m.settings.MaxConcurrentTicketsPerEpic()); err != nil {
			planErrs[epic.Name] = err
		}
		for _, idx := range sortedTicketIndexes(epic) {
			t := epic.Tickets[idx]
			if candidates[t.Path] {
				out = append(out, queueRow{epic: epic, ticket: t})
			}
		}
	}
	return out, planErrs
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
