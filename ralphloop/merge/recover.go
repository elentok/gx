package merge

import (
	"github.com/elentok/gx/events"
	"github.com/elentok/gx/ralphloop/runlog"
)

// RecoverMergeSession reads the run log for what a merging of epic/identifier
// can recover: the ticket's last iteration session (zero when none was logged)
// and the SHA of a prior merging (empty when none).
func RecoverMergeSession(scratchDir, epic, identifier string) (MergeSession, string) {
	evs, ok, err := runlog.ReadEvents(scratchDir, epic)
	if err != nil || !ok {
		return MergeSession{}, ""
	}
	recorded := LatestCherryPickedSHA(evs, identifier)
	all := runlog.IterationSessions(evs, identifier)
	if len(all) == 0 {
		return MergeSession{}, recorded
	}
	return MergeSession{Agent: all[0].Agent, Cwd: all[0].Cwd, ID: all[0].Session}, recorded
}

// LatestCherryPickedSHA returns the SHA recorded on the most recent
// cherry-picked event logged for identifier (a ticket's Identifier, not
// Number, so lettered split siblings sharing a Number aren't
// cross-attributed), or "" if none was ever logged.
func LatestCherryPickedSHA(evs []runlog.Event, identifier string) string {
	sha := ""
	for _, ev := range evs {
		if ev.Type == string(events.CherryPicked) && ev.Ticket == identifier && ev.SHA != "" {
			sha = ev.SHA
		}
	}
	return sha
}

// LatestMergedSHA is the SHA of the ticket's most recent cherry-pick, "" when
// none was logged.
func LatestMergedSHA(scratchDir, epic, ticket string) string {
	evs, _, err := runlog.ReadEvents(scratchDir, epic)
	if err != nil {
		return ""
	}
	return LatestCherryPickedSHA(evs, ticket)
}
