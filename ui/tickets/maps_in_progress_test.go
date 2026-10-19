package tickets

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/server"
)

func TestEpicSection_MapsSplitByDoneTicket(t *testing.T) {
	m := withState(newServerModel(t), server.Snapshot{Tickets: []server.TicketInfo{
		{Address: "gx:started/01", Title: "A", Status: "done", EpicIsMap: true},
		{Address: "gx:started/02", Title: "B", Status: "open", EpicIsMap: true},
		{Address: "gx:cancelled/01", Title: "A", Status: "cancelled", EpicIsMap: true},
		{Address: "gx:cancelled/02", Title: "B", Status: "open", EpicIsMap: true},
		{Address: "gx:fresh/01", Title: "A", Status: "open", EpicIsMap: true},
	}})
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	want := map[string]sidebarSection{
		"gx:started":   sectionMapsInProgress,
		"gx:cancelled": sectionOpenMaps,
		"gx:fresh":     sectionOpenMaps,
	}
	for _, e := range m.epics {
		if got := epicSection(e); got != want[e.Path] {
			t.Errorf("%s: section = %v, want %v", e.Path, got, want[e.Path])
		}
	}
	if sectionCollapsedByDefault[sectionMapsInProgress] {
		t.Error("Maps in progress starts collapsed, want expanded")
	}
}
