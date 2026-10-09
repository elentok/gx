package ralphloop

import (
	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/tickets"
)

// resumeReattachable reports whether t's iteration still has a live agent
// session — the same live-ownership test reconcile's startup reattach applies
// to a claimed/needs-repair ticket (see reconcile's reattach closure), reused
// here so a cleared ticket the scheduler is about to reclaim is judged by
// iteration ownership, not by its now-ambiguous "open" status: an "open"
// ticket the scheduler last saw launched looks identical whether its prior
// iteration is still running or long gone.
func resumeReattachable(d Deps, epicName string, agentKind AgentKind, worktreeDir string, t tickets.Ticket) bool {
	_, live := liveAgent(d, epicName, agentKind, worktreeDir, t)
	return live
}

// liveAgent looks up t's iteration session by its iteration label and reports
// the live agentrunner.Status found there, alongside whether a live session
// was actually found (see resumeReattachable's doc for what "live" means here
// — this is that same check, factored out so a caller that also needs the
// agent's current status, not just whether it's live, doesn't have to
// re-derive the lookup).
func liveAgent(d Deps, epicName string, agentKind AgentKind, worktreeDir string, t tickets.Ticket) (status agentrunner.Status, live bool) {
	session, found, err := d.Runner.Find(iterLabel(epicName, t.Identifier))
	if err != nil || !found {
		return agentrunner.Status{}, false
	}
	status, err = d.Runner.Status(session)
	if err != nil {
		return agentrunner.Status{}, false
	}
	if agentKind == AgentCodex {
		cwd := iterationWorktreePath(worktreeDir, epicName, t.Identifier)
		verified, verifyErr := d.VerifyCodexSession(cwd, status.SessionID)
		if verifyErr != nil || !verified {
			return agentrunner.Status{}, false
		}
	}
	return status, true
}
