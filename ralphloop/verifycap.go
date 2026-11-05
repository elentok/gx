package ralphloop

import (
	"fmt"

	"github.com/elentok/gx/events"
	"github.com/elentok/gx/ralphloop/agent"
	"github.com/elentok/gx/ralphloop/deps"
	"github.com/elentok/gx/ralphloop/supervise"
)

// verifyCapMargin is how far below the smart zone a verify session is stopped,
// leaving room for the cap-stop step to write its results.
const verifyCapMargin = 20_000

// verifyCap stops a verify session once, when its context occupancy first
// passes the smart zone minus verifyCapMargin. It sends a finish-up prompt and
// never compacts: the session wraps up by gx-verify-epic's cap-stop step.
type verifyCap struct {
	limit int
	state *supervise.VerifyCapState
}

// newVerifyCap returns nil when p's pane gets no cap: a pane the launch did not
// opt in by giving it a state (only the main iteration launch does; the
// conflict-resolution pane shares the ticket's params but is not the verify
// session), a non-verify ticket, or a smart zone of verifyCapMargin or less.
func newVerifyCap(p supervise.LaunchParams, smartZone int) *verifyCap {
	if p.VerifyCap == nil || !p.TicketData.IsVerify() || smartZone <= verifyCapMargin {
		return nil
	}
	return &verifyCap{limit: smartZone - verifyCapMargin, state: p.VerifyCap}
}

// tick, the first time the occupancy of the session sessionID passes the cap,
// interrupts the pane and sends the finish-up prompt. stopped reports whether
// it did, so the caller can reset its elapsed-poll-time counter. A nil receiver
// never stops anything.
func (c *verifyCap) tick(d deps.Deps, p supervise.LaunchParams, sessionID string) (stopped bool, err error) {
	if c == nil || c.state.Fired {
		return false, nil
	}
	occupancy, _, decidable, occErr := smartZoneOccupancy(d, p.Agent, p.SessionCwd, sessionID)
	if occErr != nil || !decidable || occupancy <= c.limit {
		return false, nil
	}
	if retry, err := interruptOrRetry(d, p, "verify cap"); retry || err != nil {
		return false, err
	}
	// Set before the prompt is sent on purpose: a failed send is logged, not retried.
	c.state.Fired = true

	reason := fmt.Sprintf("context occupancy %d exceeds the verify cap %d", occupancy, c.limit)
	p.LogAgentEvent(string(events.VerifyCapStopped), sessionID, reason)
	p.EventSink().VerifyCapStopped(p.Ticket)

	prompt := fmt.Sprintf(
		"I stopped you because you exceeded %d tokens in the context window, the verify token cap. "+
			"Follow the cap-stop step in the `%s` skill for your role (part, collect or lone) and finish up now.",
		c.limit, agent.VerifySkill,
	)
	if err := d.Runner.Prompt(p.SessionOrDefault(), prompt); err != nil {
		p.LogAgentEvent(string(events.VerifyCapStopped), sessionID, fmt.Sprintf("sending the verify cap prompt to %s: %v", p.Label, err))
	}
	return true, nil
}
