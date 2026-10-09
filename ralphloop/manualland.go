package ralphloop

import "github.com/elentok/gx/agentrunner"

// Exported seams for `gx server tickets land`: the label/branch naming and the
// session recovery stay private to this package so a hand landing can never
// derive them differently from the live loop.

// IterationBranch is the branch an iteration of epic/identifier builds on.
func IterationBranch(epic, identifier string) string { return iterBranch(epic, identifier) }

// LandDepsOf narrows d to what a landing needs.
func LandDepsOf(d Deps) LandDeps { return landDepsFor(d) }

// FindIterationSession returns the iteration's live session, if the runner
// knows one. An unreachable runner reports none: recovery has to keep working
// when the host is down.
func FindIterationSession(d Deps, epic, identifier string) (agentrunner.Session, bool) {
	if d.Runner == nil {
		return agentrunner.Session{}, false
	}
	s, ok, err := d.Runner.Find(iterLabel(epic, identifier))
	if err != nil || !ok {
		return agentrunner.Session{}, false
	}
	return s, true
}

// IterationWorktreePath is where an iteration of epic/identifier is checked out.
func IterationWorktreePath(worktreeDir, epic, identifier string) string {
	return iterationWorktreePath(worktreeDir, epic, identifier)
}

// StampLanded stamps the trailers (and metrics, when a session is recoverable)
// onto the commit a resolved conflict left at HEAD. It is the stamping half of
// LandTicket, for `land --continue`.
func StampLanded(d LandDeps, lp LandParams) (LandResult, error) { return stampLanded(d, lp) }

// RecoverLandSession reads the run log for what a landing of epic/identifier
// can recover: the ticket's last iteration session (zero when none was logged)
// and the SHA of a prior landing (empty when none).
func RecoverLandSession(scratchDir, epic, identifier string) (LandSession, string) {
	events, ok, err := ReadEvents(scratchDir, epic)
	if err != nil || !ok {
		return LandSession{}, ""
	}
	recorded := latestCherryPickedSHA(events, identifier)
	id, cwd, agent, ok := lastIterationSession(events, identifier)
	if !ok {
		return LandSession{}, recorded
	}
	return LandSession{Agent: agent, Cwd: cwd, ID: id}, recorded
}
