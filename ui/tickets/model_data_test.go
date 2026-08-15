package tickets

import (
	"testing"

	"github.com/elentok/gx/tickets"
)

// TestDefaultCollapsedSidebar_HonorsExplicitFalseOverAllDoneDefault covers
// the seam that caused a manually-expanded done epic to snap shut on the
// next auto-refresh poll: an explicit `false` in existing (user expanded it)
// must survive reload, since an epic has no default collapse of its own
// (that moved to the "section:closed" root as a single toggle).
func TestDefaultCollapsedSidebar_HonorsExplicitFalseOverAllDoneDefault(t *testing.T) {
	t.Parallel()
	existing := map[string]bool{"done-epic": false}

	got := defaultCollapsedSidebar(existing)

	if got["done-epic"] {
		t.Fatalf("expected explicitly-expanded done epic to stay expanded, got collapsed=%v", got["done-epic"])
	}

	// The result must carry the explicit-false entry forward, not just honor
	// it once: feeding got back in as the next reload's existing map (as
	// model.go's epicsLoadedMsg handler does every auto-refresh tick) must
	// still keep the epic expanded, not silently drop back to "unseen".
	again := defaultCollapsedSidebar(got)
	if again["done-epic"] {
		t.Fatalf("expected explicit-expand to survive a second reload, got collapsed=%v", again["done-epic"])
	}
}

// TestDefaultCollapsedSidebar_ClosedSectionDefaultsCollapsed covers 03's
// replacement for the per-epic AllDone default: "section:closed" defaults to
// collapsed when absent from existing, and an explicit override survives a
// second reload the same way a per-epic override does.
func TestDefaultCollapsedSidebar_ClosedSectionDefaultsCollapsed(t *testing.T) {
	got := defaultCollapsedSidebar(nil)
	if !got["section:closed"] {
		t.Fatalf("expected section:closed to default to collapsed, got %v", got["section:closed"])
	}

	existing := map[string]bool{"section:closed": false}
	got = defaultCollapsedSidebar(existing)
	if got["section:closed"] {
		t.Fatalf("expected explicit-expand of section:closed to be honored, got collapsed=%v", got["section:closed"])
	}

	again := defaultCollapsedSidebar(got)
	if again["section:closed"] {
		t.Fatalf("expected section:closed explicit-expand to survive a second reload, got collapsed=%v", again["section:closed"])
	}
}

// TestSortedTicketIndexes_PlanOrderIgnoresStatus mirrors
// TestSortedTickets_PlanOrderIgnoresStatus (flat_test.go) for the sidebar's
// own sort function: it orders purely by ticket number, not rendered-status
// group, so a done/needs-repair ticket stays in its plan-order slot
// instead of jumping to the bottom.
func TestSortedTicketIndexes_PlanOrderIgnoresStatus(t *testing.T) {
	t.Parallel()
	epic := tickets.Epic{Tickets: []tickets.Ticket{
		{Number: 4, Identifier: "04", Status: "needs-repair"},
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
	t.Parallel()
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
	t.Parallel()
	parent01, parent02 := "01", "02"
	epic := tickets.Epic{Path: "epic", Tickets: []tickets.Ticket{
		{Number: 1, Identifier: "01", Path: "01", Status: "open"},
		{Number: 2, Identifier: "02", Path: "02", Status: "open", Parent: &parent01},
		{Number: 3, Identifier: "03", Path: "03", Status: "open", Parent: &parent02},
		{Number: 4, Identifier: "04", Path: "04", Status: "open"},
	}}
	m := Model{epics: []tickets.Epic{epic}}

	ticketRows := func() []row {
		var rows []row
		for _, e := range m.buildSidebarEntries() {
			if e.Value.kind != nodeTicket || e.Value.epicIdx != 0 {
				continue
			}
			r, ok := rowFromEntry(e)
			if !ok {
				continue
			}
			rows = append(rows, r)
		}
		return rows
	}

	rows := ticketRows()
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

	m.sidebarTree.SetCollapsedIDs(map[string]bool{"01": true})
	rows = ticketRows()
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

// TestModel_EpicAt_ResolvesActiveRowAgainstEpics covers ticket 03's accessor
// for the common case: a row with archived=false (the default, matching
// every row buildSidebarEntries produces today) resolves against m.epics.
func TestModel_EpicAt_ResolvesActiveRowAgainstEpics(t *testing.T) {
	t.Parallel()
	m := Model{epics: []tickets.Epic{{Name: "active-epic"}}}

	got := m.epicAt(row{epicIdx: 0})

	if got.Name != "active-epic" {
		t.Fatalf("epicAt() = %+v, want epic named active-epic", got)
	}
}

// TestModel_EpicAt_ResolvesArchivedRowAgainstArchivedEpics covers the other
// half: a row with archived=true resolves against m.archivedEpics instead of
// m.epics, even when both slices are populated and share the same index —
// the origin flag, not epicIdx alone, decides which slice wins.
func TestModel_EpicAt_ResolvesArchivedRowAgainstArchivedEpics(t *testing.T) {
	t.Parallel()
	m := Model{
		epics:         []tickets.Epic{{Name: "active-epic"}},
		archivedEpics: []tickets.Epic{{Name: "archived-epic"}},
	}

	got := m.epicAt(row{epicIdx: 0, archived: true})

	if got.Name != "archived-epic" {
		t.Fatalf("epicAt() = %+v, want epic named archived-epic", got)
	}
}
