package ralphloop

import (
	"fmt"
	"strings"

	"github.com/elentok/gx/tickets"
)

// reconcile derives in-flight iteration state from ticket Status: plus live
// herdr state, with no separate state file: it runs unconditionally at the
// start of every Run call. Every ticket left `Status: claimed` (from a prior
// crash, killed terminal, or any other exit) either still has a live iter-NN
// tab in the epic's workspace — returned here so the caller reattaches and
// resumes it — or doesn't, meaning nothing survived and the ticket is
// reverted to open so normal scheduling picks it up fresh.
func reconcile(d Deps, workspaceID string, epic tickets.Epic, report func(string, ...any)) ([]tickets.Ticket, error) {
	tabs, err := d.TabList(workspaceID)
	if err != nil {
		return nil, fmt.Errorf("listing tabs for crash/restart reconciliation: %w", err)
	}
	live := make(map[string]bool, len(tabs))
	for _, tab := range tabs {
		live[tab.Label] = true
	}

	var reattached []tickets.Ticket
	for _, t := range epic.Tickets {
		status := strings.ToLower(strings.TrimSpace(t.Status))
		if status == "needs-attention" {
			if live[iterLabel(t.Number)] {
				report("ticket %02d: reattaching to needs-attention iteration %s\n", t.Number, iterLabel(t.Number))
				reattached = append(reattached, t)
			} else {
				report("ticket %02d still needs attention; no live iteration found\n", t.Number)
			}
			continue
		}
		if status != "claimed" {
			continue
		}
		if !live[iterLabel(t.Number)] {
			if err := SetStatus(t.Path, "open"); err != nil {
				return nil, fmt.Errorf("reverting ticket %d to open: %w", t.Number, err)
			}
			report("ticket %02d: no live iteration found on restart; reverted to open\n", t.Number)
			continue
		}
		report("ticket %02d: reattaching to live iteration %s\n", t.Number, iterLabel(t.Number))
		reattached = append(reattached, t)
	}
	return reattached, nil
}
