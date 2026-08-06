package ralphloop

import (
	"fmt"
	"sort"

	"github.com/elentok/gx/tickets"
)

// PlanWaves computes scope's execution order as batches of at most
// maxParallel tickets each — the canonical planner both Run's scheduler and
// any UI preview of a plan (e.g. Queue) share, so a preview can never label a
// ticket runnable when Run would actually reject it for an unresolved
// dependency. It simulates Run's claim-then-complete loop in memory: each
// wave takes the scope's currently-unblocked tickets (tickets.Epic.
// RenderedStatus, same policy scope.Frontier uses, which resolves blockers
// epic-wide regardless of whether the blocker itself is in scope), caps it at
// maxParallel, then marks that wave done before computing the next one. If a
// pass finds no unblocked ticket left while scope tickets remain pending, the
// remainder can never run — a dependency cycle or a blocker outside the scope
// that will never resolve — and PlanWaves reports that instead of silently
// dropping or misordering them.
func PlanWaves(epic tickets.Epic, scope RunScope, maxParallel int) ([][]tickets.Ticket, error) {
	if maxParallel <= 0 {
		maxParallel = defaultMaxParallel
	}

	sim := epic
	sim.Tickets = append([]tickets.Ticket(nil), epic.Tickets...)
	indexByID := make(map[string]int, len(sim.Tickets))
	for i, t := range sim.Tickets {
		indexByID[t.DisplayNumber()] = i
	}

	var remaining []tickets.Ticket
	for _, t := range epic.Tickets {
		if scope.Contains(t, epic) {
			remaining = append(remaining, t)
		}
	}

	var waves [][]tickets.Ticket
	for len(remaining) > 0 {
		var ready, blocked []tickets.Ticket
		for _, t := range remaining {
			if sim.RenderedStatus(t) == tickets.StatusBlocked {
				blocked = append(blocked, t)
			} else {
				ready = append(ready, t)
			}
		}
		if len(ready) == 0 {
			return nil, stuckPlanError(epic.Name, blocked)
		}
		sort.Slice(ready, func(i, j int) bool { return ready[i].Number < ready[j].Number })

		wave := ready
		if len(wave) > maxParallel {
			overflow := append([]tickets.Ticket(nil), wave[maxParallel:]...)
			wave = wave[:maxParallel]
			remaining = append(overflow, blocked...)
		} else {
			remaining = blocked
		}

		waves = append(waves, wave)
		for _, t := range wave {
			sim.Tickets[indexByID[t.DisplayNumber()]].Status = "done"
		}
	}
	return waves, nil
}

func stuckPlanError(epicName string, blocked []tickets.Ticket) error {
	ids := make([]string, 0, len(blocked))
	for _, t := range blocked {
		ids = append(ids, t.DisplayNumber())
	}
	return fmt.Errorf(
		"epic %q has no unblocked tickets left in the selected scope but isn't all done; "+
			"check for a dependency cycle or a blocker outside the scope among: %v", epicName, ids,
	)
}
