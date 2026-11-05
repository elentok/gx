package merge

import (
	"testing"

	eventsc "github.com/elentok/gx/events"
	"github.com/elentok/gx/ralphloop/runlog"
)

// LatestCherryPickedSHA attributes a run-log cherry-picked event to the
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
	if got := LatestCherryPickedSHA(evs, "04a"); got != "sha-04a" {
		t.Errorf("LatestCherryPickedSHA(04a) = %q, want sha-04a", got)
	}
	if got := LatestCherryPickedSHA(evs, "04b"); got != "sha-04b" {
		t.Errorf("LatestCherryPickedSHA(04b) = %q, want sha-04b", got)
	}
	if got := LatestCherryPickedSHA(evs, "04c"); got != "" {
		t.Errorf("LatestCherryPickedSHA(04c) = %q, want empty", got)
	}
}
