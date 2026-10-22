package ralphloop

import "github.com/elentok/gx/agentrunner"

// Exported seams for `gx server tickets merge`: the label/branch naming and the
// session recovery stay private to this package so a hand merging can never
// derive them differently from the live loop.

// IterationBranch is the branch an iteration of epic/identifier builds on.
func IterationBranch(epic, identifier string) string { return iterBranch(epic, identifier) }

// MergeDepsOf narrows d to what a merging needs.
func MergeDepsOf(d Deps) MergeDeps { return mergeDepsFor(d) }

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

// StampMerged stamps the trailers (and metrics, when a session is recoverable)
// onto the commit a resolved conflict left at HEAD. It is the stamping half of
// MergeTicket, for `merge --continue`.
func StampMerged(d MergeDeps, lp MergeParams) (MergeResult, error) { return stampMerged(d, lp) }

// RecoverMergeSession reads the run log for what a merging of epic/identifier
// can recover: the ticket's last iteration session (zero when none was logged)
// and the SHA of a prior merging (empty when none).
func RecoverMergeSession(scratchDir, epic, identifier string) (MergeSession, string) {
	events, ok, err := ReadEvents(scratchDir, epic)
	if err != nil || !ok {
		return MergeSession{}, ""
	}
	recorded := latestCherryPickedSHA(events, identifier)
	id, cwd, agent, ok := lastIterationSession(events, identifier)
	if !ok {
		return MergeSession{}, recorded
	}
	return MergeSession{Agent: agent, Cwd: cwd, ID: id}, recorded
}
