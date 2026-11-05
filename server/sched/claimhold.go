package sched

import (
	"time"

	harnesschoice "github.com/elentok/gx/server/harness"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
)

// Hold is why a ticket is held from claiming: the explain verdict and
// reason that name the wait.
type Hold struct {
	Verdict string
	Reason  string
}

// ClaimHold asks whether a ticket of agent in project is held from claiming.
// The claim pass and the schedule verdict both ask it, so a rule added here
// holds the ticket and explains the wait. The usage gate comes last, so a
// rate-limit hold's reason wins; a handoff restart (an answered ticket) is
// never gated.
func (x *Explainer) ClaimHold(project string, t tickets.Ticket, rh harnesschoice.ResolvedHarness) (Hold, bool) {
	agent := rh.Harness
	if reason := x.Harness.QueueMissingReason(project, rh); reason != "" {
		return Hold{Verdict: wire.VerdictHarnessMissing, Reason: reason}, true
	}
	if reason, held := x.Herdr.Holds(x.Harness.LaunchRunner(project, agent)); held {
		return Hold{Verdict: wire.VerdictHerdrDown, Reason: reason}, true
	}
	if limited, held := x.Registry.RateLimitHeld(string(agent)); held {
		return Hold{Verdict: RateLimitVerdict(limited.RateLimitReset)}, true
	}
	if AnsweredSession(t) == "" {
		if v, held := x.Limits.UsageGate(agent, time.Now()); held {
			return Hold{Verdict: v}, true
		}
	}
	return Hold{}, false
}
