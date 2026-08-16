package ralphloop

import (
	"errors"
	"fmt"
	"time"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// StopIterationAndMarkNeedsRepair is the single atomic, fakeable operation a
// budget hard-limit kill (ticket 08) calls into: stop this iteration's agent,
// then mark its ticket needs-repair. It is the only place that writes a
// ticket's terminal status as part of a budget kill — a caller outside
// ralphloop (the Queue tab's registry) triggers it but never writes ticket
// frontmatter itself, avoiding a race with ralphloop's own land-time writes
// to the same file.
//
// paneID/tabID identify the running iteration (see ui/tickets.RunTicketSnapshot,
// populated from ticket 02's IterationStarted plumbing). The graceful stop
// signal (ctrl+c) is sent to paneID, then — after grace elapses — the pane is
// closed unconditionally: a quiet-after-signal pane is not distinguishable
// from a naturally-finished one by any signal available here, so every
// touched iteration is closed uniformly rather than left in an ambiguous
// live state. Both steps run even if the signal send fails, since the point
// is to force the iteration down regardless.
//
// The ticket is marked needs-repair with reason unless it already reached a
// terminal outcome: a nonzero ActualCost read fresh from disk means the
// iteration landed successfully during the grace period, and that done
// status is left alone rather than overwritten.
func StopIterationAndMarkNeedsRepair(d Deps, ticket tickets.Ticket, paneID, tabID string, grace time.Duration, reason string) error {
	sendErr := d.AgentSendKeys(paneID, "ctrl+c")
	d.Sleep(grace)
	closeErr := d.TabClose(tabID)

	current, parseErr := schema.ParseTicket(ticket.Path)
	if parseErr != nil {
		return errors.Join(sendErr, closeErr, fmt.Errorf("reloading ticket %s to check landed status: %w", ticket.Identifier, parseErr))
	}
	if current.ActualCost == 0 {
		if markErr := MarkNeedsRepairWithReason(ticket.Path, reason, schema.NeedsRepairState{}); markErr != nil {
			return errors.Join(sendErr, closeErr, fmt.Errorf("marking ticket %s needs-repair: %w", ticket.Identifier, markErr))
		}
	}
	return errors.Join(sendErr, closeErr)
}
