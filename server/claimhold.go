package server

import (
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/tickets"
)

// VerdictHerdrDown is the explain verdict for a queued ticket that launches on
// herdr while herdr is down or not installed.
const VerdictHerdrDown = "herdr down"

// claimHold is why a ticket is held from claiming: the explain verdict and
// reason that name the wait.
type claimHold struct {
	Verdict string
	Reason  string
}

// claimHold asks whether a ticket of agent in project is held from claiming.
// claimRoot and scheduleVerdict both ask it, so a rule added here holds the
// ticket and explains the wait. The usage gate comes last, so a rate-limit
// hold's reason wins; a handoff restart (an answered ticket) is never gated.
func (s *Server) claimHold(project string, t tickets.Ticket, agent ralphloop.AgentKind) (claimHold, bool) {
	if reason, held := s.herdrHolds(s.launchRunner(project, agent)); held {
		return claimHold{Verdict: VerdictHerdrDown, Reason: reason}, true
	}
	if limited, held := s.registry.rateLimitHeld(string(agent)); held {
		return claimHold{Verdict: rateLimitVerdict(limited.RateLimitReset)}, true
	}
	if answeredSession(t) == "" {
		if v, held := s.usageGate(agent, time.Now()); held {
			return claimHold{Verdict: v}, true
		}
	}
	return claimHold{}, false
}

// herdrHolds reports why r cannot take work now: r is herdr and herdr is down
// or not installed. Any other runner is never held by herdr.
func (s *Server) herdrHolds(r agentrunner.Runner) (string, bool) {
	if r == nil || r.Name() != runnerHerdr {
		return "", false
	}
	return s.herdr.holdReason()
}

// queuedAgent is the default agent of the queue entry for address's root (a
// ticket address or "project:epic"), the project's Default harness when none
// names one.
func (s *Server) queuedAgent(address string) ralphloop.AgentKind {
	want, err := parseQueuedAddress(address)
	if err != nil {
		return s.defaultHarness(projectOfAddress(address))
	}
	for _, item := range s.queued.list() {
		if a, err := parseQueuedAddress(item.Address); err == nil && rootOf(a) == rootOf(want) && item.Agent != "" {
			return ralphloop.AgentKind(item.Agent)
		}
	}
	return s.defaultHarness(want.Project)
}
