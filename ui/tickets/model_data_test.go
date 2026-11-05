package tickets

import (
	"testing"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/keys"
	"github.com/elentok/gx/ui/tickets/internal/tickettest"
	"github.com/elentok/gx/ui/tickets/rowfmt"
	"github.com/elentok/gx/ui/tickets/sidebar"
)

// TestNewModel_StartsWithEveryEpicCollapsedAndCursorOnFirstEpic covers the
// startup the user sees: one row per epic, no ticket rows, and the cursor on
// the first epic rather than the section header above it.
func TestNewModel_StartsWithEveryEpicCollapsedAndCursorOnFirstEpic(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	tickettest.WriteTicket(t, root, "alpha", "01-alpha-ticket.md", "Status: open\n\nBody.\n")
	tickettest.WriteTicket(t, root, "beta", "01-beta-ticket.md", "Status: open\n\nBody.\n")

	m := deliverLoadDefaults(t, NewModel(root, ui.Settings{}, keys.New(nil)))

	for _, e := range m.sidebar.Tree.Entries() {
		if e.Value.Kind == sidebar.NodeTicket {
			t.Fatalf("expected no ticket rows at startup, got entry %q", e.ID)
		}
	}
	r, ok := m.sidebar.SelectedRow()
	if !ok || !r.IsEpic() || m.sidebar.Epics[r.EpicIdx].Name != "alpha" {
		t.Fatalf("expected cursor on the first epic row, got row=%+v ok=%v", r, ok)
	}
}

// TestSortedTicketIndexes_PlanOrderIgnoresStatus mirrors
// TestSortedTickets_PlanOrderIgnoresStatus (flat_test.go) for the sidebar's
// own sort function: it orders purely by ticket number, not rendered-status
// group, so a done/paused ticket stays in its plan-order slot
// instead of jumping to the bottom.
func TestSortedTicketIndexes_PlanOrderIgnoresStatus(t *testing.T) {
	t.Parallel()
	epic := tickets.Epic{Tickets: []tickets.Ticket{
		{Number: 4, Identifier: "04", Status: string(schema.StatusPaused), PauseReason: schema.PauseSpinning},
		{Number: 1, Identifier: "01", Status: "done"},
		{Number: 3, Identifier: "03", Status: "open"},
		{Number: 2, Identifier: "02", Status: "done", Commitless: true},
	}}

	got := rowfmt.SortedTicketIndexes(epic)
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

	got := rowfmt.SortedTicketIndexes(epic)
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
func TestModel_TicketRows_NestsInvestigateChildUnderOneOff(t *testing.T) {
	t.Parallel()
	parent := "01"
	epic := tickets.Epic{Path: "epic", Tickets: []tickets.Ticket{
		{Number: 1, Identifier: "01", Path: "01", Status: string(schema.StatusPaused), PauseReason: schema.PauseBlockedPane, Type: "prompt"},
		{Number: 2, Identifier: "02", Path: "02", Status: "open", Type: "investigate", Parent: &parent},
	}}
	m := Model{sidebar: sidebar.Model{Epics: []tickets.Epic{epic}}}

	var rows []sidebar.Row
	for _, e := range m.sidebar.BuildSidebarEntries(sidebarEnvFor(m)) {
		if e.Value.Kind != sidebar.NodeTicket {
			continue
		}
		if r, ok := sidebar.RowFromEntry(e); ok {
			rows = append(rows, r)
		}
	}
	if len(rows) != 2 || rows[0].Depth != 0 || !rows[0].HasChildren || rows[1].Depth != 1 {
		t.Fatalf("expected investigate child nested under its one-off, got %+v", rows)
	}
}

func TestModel_TicketRows_NestsChildrenAtArbitraryDepthAndRespectsCollapse(t *testing.T) {
	t.Parallel()
	parent01, parent02 := "01", "02"
	epic := tickets.Epic{Path: "epic", Tickets: []tickets.Ticket{
		{Number: 1, Identifier: "01", Path: "01", Status: "open"},
		{Number: 2, Identifier: "02", Path: "02", Status: "open", Parent: &parent01},
		{Number: 3, Identifier: "03", Path: "03", Status: "open", Parent: &parent02},
		{Number: 4, Identifier: "04", Path: "04", Status: "open"},
	}}
	m := Model{sidebar: sidebar.Model{Epics: []tickets.Epic{epic}}}

	ticketRows := func() []sidebar.Row {
		var rows []sidebar.Row
		for _, e := range m.sidebar.BuildSidebarEntries(sidebarEnvFor(m)) {
			if e.Value.Kind != sidebar.NodeTicket || e.Value.EpicIdx != 0 {
				continue
			}
			r, ok := sidebar.RowFromEntry(e)
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
		if got := epic.Tickets[r.TicketIdx].Identifier; got != wantIDs[i] {
			t.Fatalf("row %d ticket = %q, want %q", i, got, wantIDs[i])
		}
		if r.Depth != wantDepths[i] {
			t.Fatalf("row %d (%s) depth = %d, want %d", i, wantIDs[i], r.Depth, wantDepths[i])
		}
	}
	if !rows[0].HasChildren || !rows[0].Expanded {
		t.Fatalf("expected ticket 01 to report hasChildren+expanded, got %+v", rows[0])
	}

	m.sidebar.Tree.SetCollapsedIDs(map[string]bool{"01": true})
	rows = ticketRows()
	wantAfterCollapse := []string{"01", "04"}
	if len(rows) != len(wantAfterCollapse) {
		t.Fatalf("expected %d rows after collapsing 01, got %d: %+v", len(wantAfterCollapse), len(rows), rows)
	}
	for i, want := range wantAfterCollapse {
		if got := epic.Tickets[rows[i].TicketIdx].Identifier; got != want {
			t.Fatalf("row %d after collapse = %q, want %q", i, got, want)
		}
	}
	if rows[0].Expanded {
		t.Fatalf("expected collapsed ticket 01 to report expanded=false")
	}
}

// TestModel_EpicAt_ResolvesActiveRowAgainstEpics covers ticket 03's accessor
// for the common case: a row with archived=false (the default, matching
// every row buildSidebarEntries produces today) resolves against m.sidebar.epics.
func TestModel_EpicAt_ResolvesActiveRowAgainstEpics(t *testing.T) {
	t.Parallel()
	m := Model{sidebar: sidebar.Model{Epics: []tickets.Epic{{Name: "active-epic"}}}}

	got := m.sidebar.EpicAt(sidebar.Row{EpicIdx: 0})

	if got.Name != "active-epic" {
		t.Fatalf("epicAt() = %+v, want epic named active-epic", got)
	}
}

// TestModel_EpicAt_ResolvesArchivedRowAgainstArchivedEpics covers the other
// half: a row with archived=true resolves against m.archivedEpics instead of
// m.sidebar.epics, even when both slices are populated and share the same index —
// the origin flag, not epicIdx alone, decides which slice wins.
func TestModel_EpicAt_ResolvesArchivedRowAgainstArchivedEpics(t *testing.T) {
	t.Parallel()
	m := Model{
		sidebar: sidebar.Model{
			Epics:         []tickets.Epic{{Name: "active-epic"}},
			ArchivedEpics: []tickets.Epic{{Name: "archived-epic"}},
		},
	}

	got := m.sidebar.EpicAt(sidebar.Row{EpicIdx: 0, Archived: true})

	if got.Name != "archived-epic" {
		t.Fatalf("epicAt() = %+v, want epic named archived-epic", got)
	}
}
