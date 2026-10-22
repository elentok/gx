package tickets

import (
	"testing"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

func TestPauseActionLabel(t *testing.T) {
	cases := map[schema.PauseReason]string{
		schema.PauseSelfReported:    "Answer",
		schema.PauseAmbiguousBase:   "Choose base",
		schema.PauseMergeConflict:   "Resolve conflict",
		schema.PauseStrandedCommits: "Merge commits",
		schema.PauseManual:          "Resume",
		schema.PauseSpinning:        "Retry",
		schema.PauseZeroCommit:      "Retry",
		schema.PauseBudgetKilled:    "Retry",
	}
	for reason, want := range cases {
		if got := pauseActionLabel(reason); got != want {
			t.Errorf("pauseActionLabel(%q) = %q, want %q", reason, got, want)
		}
	}
}

func TestPausedMenuActionByReason(t *testing.T) {
	for _, reason := range []schema.PauseReason{schema.PauseSelfReported, schema.PauseAmbiguousBase, schema.PauseMergeConflict, schema.PauseSpinning, schema.PauseManual} {
		ticket := tickets.Ticket{Status: string(schema.StatusPaused), PauseReason: reason}
		items := ticketMenuItems(ticketMenuInput{rendered: ticket.RawStatus(), ticket: ticket})
		item := itemByLabel(t, items, pauseActionLabel(reason))
		if item.Disabled != "" && item.Disabled != serverNotRunning {
			t.Errorf("%s: action %q disabled: %q", reason, item.Label, item.Disabled)
		}
	}
	// Resume never labels a pause that needs an answer or a fix.
	for _, reason := range []schema.PauseReason{schema.PauseSelfReported, schema.PauseAmbiguousBase, schema.PauseSpinning} {
		ticket := tickets.Ticket{Status: string(schema.StatusPaused), PauseReason: reason}
		for _, it := range ticketMenuItems(ticketMenuInput{rendered: ticket.RawStatus(), ticket: ticket}) {
			if it.Label == "Resume" {
				t.Errorf("%s: menu offers Resume", reason)
			}
		}
	}
}

func TestPausedStatusText(t *testing.T) {
	got := pausedStatusText(tickets.Ticket{Status: "paused", PauseReason: schema.PauseMergeConflict})
	if got != "paused: Merge conflict" {
		t.Errorf("got %q", got)
	}
}
