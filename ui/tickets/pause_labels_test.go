package tickets

import (
	"testing"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

func TestPausedStatusText(t *testing.T) {
	got := pausedStatusText(tickets.Ticket{Status: "paused", PauseReason: schema.PauseMergeConflict})
	if got != "paused: Merge conflict" {
		t.Errorf("got %q", got)
	}
}
