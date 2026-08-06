package tickets

import (
	"testing"

	"github.com/elentok/gx/tickets"
)

// TestQueueRowsForEpic_NestsChildrenAtArbitraryDepthAndRespectsCollapse is
// ticket 10's row-building test seam, mirroring ticket 09's
// TestModel_TicketRows_NestsChildrenAtArbitraryDepthAndRespectsCollapse for
// the Queue tab's own row model: a ticket (01) with a child (02) that itself
// has a child (03) nests two levels deep via ui/tree's entry-builder,
// leaving an unrelated ticket (04) at the top level; collapsing the
// grandparent hides both descendants while leaving 04 visible, and the
// plan-order given in ordered (mirroring epicRowOrder's blockers-before-
// dependents output) is preserved among sibling rows.
func TestQueueRowsForEpic_NestsChildrenAtArbitraryDepthAndRespectsCollapse(t *testing.T) {
	parent01, parent02 := "01", "02"
	epic := tickets.Epic{Path: "epic", Tickets: []tickets.Ticket{
		{Number: 1, Identifier: "01", Path: "01", Status: "open"},
		{Number: 2, Identifier: "02", Path: "02", Status: "open", Parent: &parent01},
		{Number: 3, Identifier: "03", Path: "03", Status: "open", Parent: &parent02},
		{Number: 4, Identifier: "04", Path: "04", Status: "open"},
	}}
	// ordered mirrors epicRowOrder's plan-order output: blockers before
	// dependents, here just the epic's four tickets in declaration order.
	ordered := epic.Tickets

	rows := queueRowsForEpic(epic, ordered, nil)
	wantIDs := []string{"01", "02", "03", "04"}
	wantDepths := []int{0, 1, 2, 0}
	if len(rows) != len(wantIDs) {
		t.Fatalf("got %d rows, want %d: %+v", len(rows), len(wantIDs), rows)
	}
	for i, r := range rows {
		if r.ticket.Identifier != wantIDs[i] {
			t.Fatalf("row %d ticket = %q, want %q", i, r.ticket.Identifier, wantIDs[i])
		}
		if r.depth != wantDepths[i] {
			t.Fatalf("row %d (%s) depth = %d, want %d", i, wantIDs[i], r.depth, wantDepths[i])
		}
	}
	if !rows[0].hasChildren || !rows[0].expanded {
		t.Fatalf("expected ticket 01 to report hasChildren+expanded, got %+v", rows[0])
	}

	collapsed := map[string]bool{"01": true}
	rows = queueRowsForEpic(epic, ordered, collapsed)
	wantAfterCollapse := []string{"01", "04"}
	if len(rows) != len(wantAfterCollapse) {
		t.Fatalf("expected %d rows after collapsing 01, got %d: %+v", len(wantAfterCollapse), len(rows), rows)
	}
	for i, want := range wantAfterCollapse {
		if rows[i].ticket.Identifier != want {
			t.Fatalf("row %d after collapse = %q, want %q", i, rows[i].ticket.Identifier, want)
		}
	}
	if rows[0].expanded {
		t.Fatalf("expected collapsed ticket 01 to report expanded=false")
	}
}

// TestQueueRowsForEpic_FilteredParentReattachesChildrenToNearestAncestor
// covers filterDoneTickets dropping a done parent before nesting: its child
// reattaches to the epic's top level instead of being stranded, mirroring
// the Tickets tab's nearestVisibleAncestor (ticket 09) hideDone behavior.
func TestQueueRowsForEpic_FilteredParentReattachesChildrenToNearestAncestor(t *testing.T) {
	parent01 := "01"
	epic := tickets.Epic{Path: "epic", Tickets: []tickets.Ticket{
		{Number: 1, Identifier: "01", Path: "01", Status: "done"},
		{Number: 2, Identifier: "02", Path: "02", Status: "open", Parent: &parent01},
	}}
	ordered := filterDoneTickets(epic, epic.Tickets)

	rows := queueRowsForEpic(epic, ordered, nil)
	if len(rows) != 1 || rows[0].ticket.Identifier != "02" {
		t.Fatalf("expected only ticket 02 reattached to top level, got %+v", rows)
	}
	if rows[0].depth != 0 || rows[0].parentPath != "" {
		t.Fatalf("expected ticket 02 at depth 0 with no parent, got depth=%d parentPath=%q", rows[0].depth, rows[0].parentPath)
	}
}
