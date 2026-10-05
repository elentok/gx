package ralphloop

// Exported seams for `gx tickets land`: the label/branch naming and the
// session recovery stay private to this package so a hand landing can never
// derive them differently from the live loop.

// IterationBranch is the branch an iteration of epic/identifier builds on.
func IterationBranch(epic, identifier string) string { return iterBranch(epic, identifier) }

// LandDepsOf narrows d to what a landing needs.
func LandDepsOf(d Deps) LandDeps { return landDepsFor(d) }

// IterationTabLive reports whether the iteration's herdr tab exists. Herdr
// being unreachable (no workspace, or a failing list) reports false: recovery
// has to keep working when herdr is down, and a tab outlives its agent anyway.
func IterationTabLive(d Deps, epic, identifier string) bool {
	if d.FindWorkspace == nil || d.TabList == nil {
		return false
	}
	workspaceID, err := d.FindWorkspace(epic)
	if err != nil || workspaceID == "" {
		return false
	}
	tabs, err := d.TabList(workspaceID)
	if err != nil {
		return false
	}
	label := iterLabel(epic, identifier)
	for _, tab := range tabs {
		if tab.Label == label {
			return true
		}
	}
	return false
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
