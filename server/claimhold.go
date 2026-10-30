package server

import (
	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/ralphloop"
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
// ticket and explains the wait.
func (s *Server) claimHold(project string, agent ralphloop.AgentKind) (claimHold, bool) {
	if reason, held := s.herdrHolds(s.launchRunner(project, agent)); held {
		return claimHold{Verdict: VerdictHerdrDown, Reason: reason}, true
	}
	if t, held := s.registry.rateLimitHeld(string(agent)); held {
		return claimHold{Verdict: rateLimitVerdict(t.RateLimitReset)}, true
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

// rootAgent is the default agent of root's queue entry, "project:epic".
func (s *Server) rootAgent(root string) ralphloop.AgentKind {
	for _, item := range s.queued.list() {
		if a, err := parseQueuedAddress(item.Address); err == nil && a.Project+":"+a.Epic == root && item.Agent != "" {
			return ralphloop.AgentKind(item.Agent)
		}
	}
	return ralphloop.AgentClaude
}
