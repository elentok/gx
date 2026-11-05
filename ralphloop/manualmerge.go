package ralphloop

import (
	"github.com/elentok/gx/ralphloop/deps"
	"github.com/elentok/gx/ralphloop/runlog"
)

// Exported seams for `gx server tickets merge`: the label/branch naming and the
// session recovery stay private to this package so a hand merging can never
// derive them differently from the live loop.

// MergeDepsOf narrows d to what a merging needs.
func MergeDepsOf(d deps.Deps) MergeDeps { return mergeDepsFor(d) }

// StampMerged stamps the trailers (and metrics, when a session is recoverable)
// onto the commit a resolved conflict left at HEAD. It is the stamping half of
// MergeTicket, for `merge --continue`.
func StampMerged(d MergeDeps, lp MergeParams) (MergeResult, error) { return stampMerged(d, lp) }

// RecoverMergeSession reads the run log for what a merging of epic/identifier
// can recover: the ticket's last iteration session (zero when none was logged)
// and the SHA of a prior merging (empty when none).
func RecoverMergeSession(scratchDir, epic, identifier string) (MergeSession, string) {
	events, ok, err := runlog.ReadEvents(scratchDir, epic)
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
