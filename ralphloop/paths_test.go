package ralphloop

import (
	"testing"

	eventsc "github.com/elentok/gx/events"

	"github.com/elentok/gx/ralphloop/runlog"
)

// A mid-flight split keeps the same Number across every letter (04 ->
// 04a/04b), so iterLabel/iterBranch must key off Identifier, not Number, or
// lettered siblings would collide on the same worktree/branch.
func TestIterLabelIterBranch_DistinctForLetteredSiblingsSharingNumber(t *testing.T) {
	t.Parallel()
	if iterLabel("epic", "04a") == iterLabel("epic", "04b") {
		t.Errorf("iterLabel(04a) == iterLabel(04b) = %q, want distinct labels", iterLabel("epic", "04a"))
	}
	if iterBranch("epic", "04a") == iterBranch("epic", "04b") {
		t.Errorf("iterBranch(04a) == iterBranch(04b) = %q, want distinct branches", iterBranch("epic", "04a"))
	}
}

// latestCherryPickedSHA attributes a run-log cherry-picked event to the
// sibling that logged it (by Identifier), not whichever sibling shares its
// Number; and it returns the latest SHA when a ticket was picked twice.
func TestLatestCherryPickedSHA_KeyedByIdentifierAndLatestWins(t *testing.T) {
	t.Parallel()
	evs := []runlog.Event{
		{Type: string(eventsc.CherryPicked), Ticket: "04a", SHA: "sha-04a-old"},
		{Type: string(eventsc.CherryPicked), Ticket: "04b", SHA: "sha-04b"},
		{Type: string(eventsc.CherryPicked), Ticket: "04a", SHA: "sha-04a"},
		{Type: string(eventsc.CherryPicked), Ticket: "04a"},
	}
	if got := latestCherryPickedSHA(evs, "04a"); got != "sha-04a" {
		t.Errorf("latestCherryPickedSHA(04a) = %q, want sha-04a", got)
	}
	if got := latestCherryPickedSHA(evs, "04b"); got != "sha-04b" {
		t.Errorf("latestCherryPickedSHA(04b) = %q, want sha-04b", got)
	}
	if got := latestCherryPickedSHA(evs, "04c"); got != "" {
		t.Errorf("latestCherryPickedSHA(04c) = %q, want empty", got)
	}
}
