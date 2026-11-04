package ralphloop

import "github.com/elentok/gx/events"

// reconcilePaths bundles the filesystem locations verify needs: the scratch
// dir (to read run-log.jsonl), the feature worktree (to run git checks in) and
// the iteration worktree directory.
type reconcilePaths struct {
	ScratchDir      string
	FeatureWorktree string
	WorktreeDir     string
	RepoDir         string
}

// latestCherryPickedSHA returns the SHA recorded on the most recent
// cherry-picked event logged for identifier (a ticket's Identifier, not
// Number, so lettered split siblings sharing a Number aren't
// cross-attributed), or "" if none was ever logged.
func latestCherryPickedSHA(evs []Event, identifier string) string {
	sha := ""
	for _, ev := range evs {
		if ev.Type == string(events.CherryPicked) && ev.Ticket == identifier && ev.SHA != "" {
			sha = ev.SHA
		}
	}
	return sha
}
