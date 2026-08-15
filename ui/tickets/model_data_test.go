package tickets

import (
	"testing"

	"github.com/elentok/gx/tickets"
)

// TestDeriveCollapsedSidebar_ClosedEpicDefaultsCollapsedOpenEpicDoesNot covers
// ticket 02's core policy: a closed epic's ID gets a derived default of
// collapsed with no explicit state at all, while an open epic gets none (so
// it stays expanded).
func TestDeriveCollapsedSidebar_ClosedEpicDefaultsCollapsedOpenEpicDoesNot(t *testing.T) {
	t.Parallel()
	epics := []tickets.Epic{
		{Path: "closed-epic", Tickets: []tickets.Ticket{{Status: "done"}}},
		{Path: "open-epic", Tickets: []tickets.Ticket{{Status: "open"}}},
	}

	got := deriveCollapsedSidebar(nil, epics, nil, "")

	if !got["closed-epic"] {
		t.Fatalf("expected closed epic to default to collapsed, got %v", got["closed-epic"])
	}
	if got["open-epic"] {
		t.Fatalf("expected open epic to have no collapse default, got collapsed=%v", got["open-epic"])
	}
}

// TestDeriveCollapsedSidebar_ReopenedEpicLosesCollapsedDefault covers the
// no-stickiness requirement: an epic that closes then reopens must lose its
// collapsed default on the very next derivation, not stay stuck collapsed.
func TestDeriveCollapsedSidebar_ReopenedEpicLosesCollapsedDefault(t *testing.T) {
	t.Parallel()
	closed := []tickets.Epic{{Path: "epic", Tickets: []tickets.Ticket{{Status: "done"}}}}
	reopened := []tickets.Epic{{Path: "epic", Tickets: []tickets.Ticket{{Status: "open"}}}}

	if got := deriveCollapsedSidebar(nil, closed, nil, ""); !got["epic"] {
		t.Fatalf("expected closed epic to default to collapsed, got %v", got["epic"])
	}
	if got := deriveCollapsedSidebar(nil, reopened, nil, ""); got["epic"] {
		t.Fatalf("expected reopened epic to lose its collapsed default, got collapsed=%v", got["epic"])
	}
}

// TestDeriveCollapsedSidebar_ExplicitToggleBeatsClosedDefault covers both
// directions of an explicit user toggle on a closed epic overriding the
// declared default.
func TestDeriveCollapsedSidebar_ExplicitToggleBeatsClosedDefault(t *testing.T) {
	t.Parallel()
	epics := []tickets.Epic{{Path: "epic", Tickets: []tickets.Ticket{{Status: "done"}}}}

	expanded := deriveCollapsedSidebar(map[string]bool{"epic": false}, epics, nil, "")
	if expanded["epic"] {
		t.Fatalf("expected explicit expand to beat closed-epic default, got collapsed=%v", expanded["epic"])
	}

	collapsed := deriveCollapsedSidebar(map[string]bool{"epic": true}, epics, nil, "")
	if !collapsed["epic"] {
		t.Fatalf("expected explicit collapse to be honored, got collapsed=%v", collapsed["epic"])
	}
}

// TestDeriveCollapsedSidebar_SearchOverrideIsTransient covers search's
// auto-expand: it applies only while the epic still matches the live query,
// disappears once the query no longer matches (including an empty query),
// and never leaves a trace in the explicit map that was handed in.
func TestDeriveCollapsedSidebar_SearchOverrideIsTransient(t *testing.T) {
	t.Parallel()
	epics := []tickets.Epic{{Path: "closed-epic", Tickets: []tickets.Ticket{{Title: "widget", Status: "done"}}}}
	explicit := map[string]bool{}

	matched := deriveCollapsedSidebar(explicit, epics, nil, "widget")
	if matched["closed-epic"] {
		t.Fatalf("expected search match to expand closed epic, got collapsed=%v", matched["closed-epic"])
	}
	if _, ok := explicit["closed-epic"]; ok {
		t.Fatalf("expected search override to leave no trace in the explicit map, got %v", explicit)
	}

	noQuery := deriveCollapsedSidebar(explicit, epics, nil, "")
	if !noQuery["closed-epic"] {
		t.Fatalf("expected closed-epic default to reassert once search ends, got collapsed=%v", noQuery["closed-epic"])
	}

	noMatch := deriveCollapsedSidebar(explicit, epics, nil, "gadget")
	if !noMatch["closed-epic"] {
		t.Fatalf("expected closed-epic default to reassert once it no longer matches, got collapsed=%v", noMatch["closed-epic"])
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
