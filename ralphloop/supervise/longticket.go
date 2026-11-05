package supervise

import (
	"github.com/elentok/gx/ralphloop/deps"

	"fmt"
	"time"
)

// eventLongTicket is the run-log type of the alert.
const eventLongTicket = "long-ticket"

// LongTicketGuard counts one implement launch's assistant turns and calls the
// alert once when they reach the threshold. The session keeps running.
type LongTicketGuard struct {
	threshold int
	since     time.Time
	fired     bool
}

// NewLongTicketGuard returns nil when the launch gets no alert: not an
// implement ticket, no reader, no callback, or a threshold of zero.
func NewLongTicketGuard(d deps.Deps, p LaunchParams, since time.Time) *LongTicketGuard {
	if d.ReadTurns == nil || p.OnLongTicket == nil || p.LongTicketTurns <= 0 || !IsImplementTicket(p) {
		return nil
	}
	return &LongTicketGuard{threshold: p.LongTicketTurns, since: since}
}

// Tick reads the turn count and fires the alert the first time it reaches the
// threshold. A nil receiver does nothing; a failed or empty read tries again.
func (g *LongTicketGuard) Tick(d deps.Deps, p LaunchParams, sessionID string) {
	if g == nil || g.fired || sessionID == "" {
		return
	}
	turns, ok, err := d.ReadTurns(p.SessionCwd, sessionID, g.since)
	if err != nil || !ok || turns.Count < g.threshold {
		return
	}
	g.fired = true
	p.LogAgentEvent(eventLongTicket, sessionID, fmt.Sprintf("%d assistant turns, threshold %d", turns.Count, g.threshold))
	p.OnLongTicket(turns.Count)
}
