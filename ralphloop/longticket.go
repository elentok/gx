package ralphloop

import (
	"fmt"
	"time"
)

// eventLongTicket is the run-log type of the alert.
const eventLongTicket = "long-ticket"

// longTicketGuard counts one implement launch's assistant turns and calls the
// alert once when they reach the threshold. The session keeps running.
type longTicketGuard struct {
	threshold int
	since     time.Time
	fired     bool
}

// newLongTicketGuard returns nil when the launch gets no alert: not an
// implement ticket, no reader, no callback, or a threshold of zero.
func newLongTicketGuard(d Deps, p launchAndPromptParams, since time.Time) *longTicketGuard {
	if d.ReadTurns == nil || p.OnLongTicket == nil || p.LongTicketTurns <= 0 || !isImplementTicket(p) {
		return nil
	}
	return &longTicketGuard{threshold: p.LongTicketTurns, since: since}
}

// tick reads the turn count and fires the alert the first time it reaches the
// threshold. A nil receiver does nothing; a failed or empty read tries again.
func (g *longTicketGuard) tick(d Deps, p launchAndPromptParams, sessionID string) {
	if g == nil || g.fired || sessionID == "" {
		return
	}
	turns, ok, err := d.ReadTurns(p.SessionCwd, sessionID, g.since)
	if err != nil || !ok || turns.Count < g.threshold {
		return
	}
	g.fired = true
	p.logAgentEvent(eventLongTicket, sessionID, fmt.Sprintf("%d assistant turns, threshold %d", turns.Count, g.threshold))
	p.OnLongTicket(turns.Count)
}
