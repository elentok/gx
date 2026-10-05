package ralphloop

// Exported seams for `gx tickets land`: the label/branch naming and the
// session recovery stay private to this package so a hand landing can never
// derive them differently from the live loop.

// IterationBranch is the branch an iteration of epic/identifier builds on.
func IterationBranch(epic, identifier string) string { return iterBranch(epic, identifier) }

// LandDepsOf narrows d to what a landing needs.
func LandDepsOf(d Deps) LandDeps { return landDepsFor(d) }

// IterationTabID returns the id of the iteration's herdr tab, or "" when there
// is none. Herdr being unreachable (no workspace, or a failing list) reports
// "": recovery has to keep working when herdr is down, and a tab outlives its
// agent anyway.
func IterationTabID(d Deps, epic, identifier string) string {
	if d.FindWorkspace == nil || d.TabList == nil {
		return ""
	}
	workspaceID, err := d.FindWorkspace(epic)
	if err != nil || workspaceID == "" {
		return ""
	}
	tabs, err := d.TabList(workspaceID)
	if err != nil {
		return ""
	}
	return tabIDForLabel(tabs, iterLabel(epic, identifier))
}

// IterationTabLive reports whether the iteration's herdr tab exists.
func IterationTabLive(d Deps, epic, identifier string) bool {
	return IterationTabID(d, epic, identifier) != ""
}

// IterationWorktreePath is where an iteration of epic/identifier is checked out.
func IterationWorktreePath(worktreeDir, epic, identifier string) string {
	return iterationWorktreePath(worktreeDir, epic, identifier)
}

// StampLanded stamps the trailers (and metrics, when a session is recoverable)
// onto the commit a resolved conflict left at HEAD. It is the stamping half of
// LandTicket, for `land --continue`.
func StampLanded(d LandDeps, lp LandParams) (LandResult, error) { return stampLanded(d, lp) }

// IterationAgentAlive reports whether herdr still knows an agent under the
// iteration's label. A tab with no such agent is stale. Herdr being
// unreachable reports false, for the same reason as IterationTabLive.
func IterationAgentAlive(d Deps, epic, identifier string) bool {
	if d.AgentGet == nil {
		return false
	}
	agent, err := d.AgentGet(iterLabel(epic, identifier))
	return err == nil && agent.PaneID != ""
}

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
