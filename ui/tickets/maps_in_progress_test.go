package tickets

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/ui/tickets/sidebar"
)

func TestEpicSection_MapsSplitByDoneTicket(t *testing.T) {
	m := withState(newServerModel(t), wire.Snapshot{Tickets: []wire.TicketInfo{
		{Address: "gx:started/01", Title: "A", Status: "done", EpicIsMap: true},
		{Address: "gx:started/02", Title: "B", Status: "open", EpicIsMap: true},
		{Address: "gx:cancelled/01", Title: "A", Status: "cancelled", EpicIsMap: true},
		{Address: "gx:cancelled/02", Title: "B", Status: "open", EpicIsMap: true},
		{Address: "gx:fresh/01", Title: "A", Status: "open", EpicIsMap: true},
	}})
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	want := map[string]sidebar.Section{
		"gx:started":   sidebar.SectionMapsInProgress,
		"gx:cancelled": sidebar.SectionOpenMaps,
		"gx:fresh":     sidebar.SectionOpenMaps,
	}
	for _, e := range m.sidebar.Epics {
		if got := sidebar.EpicSection(e); got != want[e.Path] {
			t.Errorf("%s: section = %v, want %v", e.Path, got, want[e.Path])
		}
	}
	if sidebar.SectionCollapsedByDefault[sidebar.SectionMapsInProgress] {
		t.Error("Maps in progress starts collapsed, want expanded")
	}
}
