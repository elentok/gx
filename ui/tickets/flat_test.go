package tickets

import (
	"testing"

	"github.com/elentok/gx/tickets"
)

// TestSortedTickets_PlanOrderIgnoresStatus guards against re-grouping by
// rendered status: sortedTickets orders purely by ticket number, so a done
// (or superseded, or needs-attention) ticket stays in its plan-order slot
// instead of jumping to the bottom/top — see the function's own doc comment.
func TestSortedTickets_PlanOrderIgnoresStatus(t *testing.T) {
	epic := tickets.Epic{Tickets: []tickets.Ticket{
		{Number: 4, Identifier: "04", Status: "needs-attention"},
		{Number: 1, Identifier: "01", Status: "done"},
		{Number: 3, Identifier: "03", Status: "open"},
		{Number: 2, Identifier: "02", Status: "superseded"},
	}}

	got := sortedTickets(epic)
	want := []string{"01", "02", "03", "04"}
	for i, id := range want {
		if got[i].Identifier != id {
			t.Fatalf("sortedTickets()[%d].Identifier = %q, want %q (got order %v)", i, got[i].Identifier, id, identifiers(got))
		}
	}
}

// TestSortedTickets_LetteredSiblingsFollowOriginalInFilenameOrder covers a
// mid-flight split: 04, 04a, 04b all share Number 4, so they tie-break on
// DisplayNumber — the superseded original sorts before its replacements, in
// the same order their filenames imply.
func TestSortedTickets_LetteredSiblingsFollowOriginalInFilenameOrder(t *testing.T) {
	epic := tickets.Epic{Tickets: []tickets.Ticket{
		{Number: 4, Identifier: "04b", Status: "open"},
		{Number: 5, Identifier: "05", Status: "open"},
		{Number: 4, Identifier: "04", Status: "superseded"},
		{Number: 4, Identifier: "04a", Status: "done"},
	}}

	got := sortedTickets(epic)
	want := []string{"04", "04a", "04b", "05"}
	for i, id := range want {
		if got[i].Identifier != id {
			t.Fatalf("sortedTickets()[%d].Identifier = %q, want %q (got order %v)", i, got[i].Identifier, id, identifiers(got))
		}
	}
}

func identifiers(ts []tickets.Ticket) []string {
	ids := make([]string, len(ts))
	for i, t := range ts {
		ids[i] = t.Identifier
	}
	return ids
}
