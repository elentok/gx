package tickets

import (
	"testing"

	"github.com/elentok/gx/tickets"
)

// TestSortedTicketIndexes_PlanOrderIgnoresStatus mirrors
// TestSortedTickets_PlanOrderIgnoresStatus (flat_test.go) for the sidebar's
// own sort function: it orders purely by ticket number, not rendered-status
// group, so a done/needs-attention ticket stays in its plan-order slot
// instead of jumping to the bottom.
func TestSortedTicketIndexes_PlanOrderIgnoresStatus(t *testing.T) {
	epic := tickets.Epic{Tickets: []tickets.Ticket{
		{Number: 4, Identifier: "04", Status: "needs-attention"},
		{Number: 1, Identifier: "01", Status: "done"},
		{Number: 3, Identifier: "03", Status: "open"},
		{Number: 2, Identifier: "02", Status: "done", Commitless: true},
	}}

	got := sortedTicketIndexes(epic)
	want := []string{"01", "02", "03", "04"}
	for i, id := range want {
		if epic.Tickets[got[i]].Identifier != id {
			t.Fatalf("sortedTicketIndexes()[%d] -> %q, want %q", i, epic.Tickets[got[i]].Identifier, id)
		}
	}
}

// TestSortedTicketIndexes_LetteredSiblingsFollowOriginalInFilenameOrder
// mirrors TestSortedTickets_LetteredSiblingsFollowOriginalInFilenameOrder
// (flat_test.go): 04/04a/04b share Number 4 and tie-break on DisplayNumber.
func TestSortedTicketIndexes_LetteredSiblingsFollowOriginalInFilenameOrder(t *testing.T) {
	epic := tickets.Epic{Tickets: []tickets.Ticket{
		{Number: 4, Identifier: "04b", Status: "open"},
		{Number: 5, Identifier: "05", Status: "open"},
		{Number: 4, Identifier: "04", Status: "done", Commitless: true},
		{Number: 4, Identifier: "04a", Status: "done"},
	}}

	got := sortedTicketIndexes(epic)
	want := []string{"04", "04a", "04b", "05"}
	for i, id := range want {
		if epic.Tickets[got[i]].Identifier != id {
			t.Fatalf("sortedTicketIndexes()[%d] -> %q, want %q", i, epic.Tickets[got[i]].Identifier, id)
		}
	}
}

// TestModel_TicketRows_NestsChildrenAtArbitraryDepthAndRespectsCollapse is
// ticket 09's row-building test seam: a ticket (01) with a child (02) that
// itself has a child (03) nests two levels deep via ui/tree's entry-builder,
// leaving an unrelated ticket (04) at the top level; collapsing the
// grandparent hides both descendants while leaving 04 visible.
func TestModel_TicketRows_NestsChildrenAtArbitraryDepthAndRespectsCollapse(t *testing.T) {
	parent01, parent02 := "01", "02"
	epic := tickets.Epic{Path: "epic", Tickets: []tickets.Ticket{
		{Number: 1, Identifier: "01", Path: "01", Status: "open"},
		{Number: 2, Identifier: "02", Path: "02", Status: "open", Parent: &parent01},
		{Number: 3, Identifier: "03", Path: "03", Status: "open", Parent: &parent02},
		{Number: 4, Identifier: "04", Path: "04", Status: "open"},
	}}
	m := Model{}

	rows := m.ticketRows(0, epic)
	wantIDs := []string{"01", "02", "03", "04"}
	wantDepths := []int{0, 1, 2, 0}
	if len(rows) != len(wantIDs) {
		t.Fatalf("got %d rows, want %d: %+v", len(rows), len(wantIDs), rows)
	}
	for i, r := range rows {
		if got := epic.Tickets[r.ticketIdx].Identifier; got != wantIDs[i] {
			t.Fatalf("row %d ticket = %q, want %q", i, got, wantIDs[i])
		}
		if r.depth != wantDepths[i] {
			t.Fatalf("row %d (%s) depth = %d, want %d", i, wantIDs[i], r.depth, wantDepths[i])
		}
	}
	if !rows[0].hasChildren || !rows[0].expanded {
		t.Fatalf("expected ticket 01 to report hasChildren+expanded, got %+v", rows[0])
	}

	m.collapsedTickets = map[string]bool{"01": true}
	rows = m.ticketRows(0, epic)
	wantAfterCollapse := []string{"01", "04"}
	if len(rows) != len(wantAfterCollapse) {
		t.Fatalf("expected %d rows after collapsing 01, got %d: %+v", len(wantAfterCollapse), len(rows), rows)
	}
	for i, want := range wantAfterCollapse {
		if got := epic.Tickets[rows[i].ticketIdx].Identifier; got != want {
			t.Fatalf("row %d after collapse = %q, want %q", i, got, want)
		}
	}
	if rows[0].expanded {
		t.Fatalf("expected collapsed ticket 01 to report expanded=false")
	}
}
