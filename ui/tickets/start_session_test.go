package tickets

import (
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/ui/contextmenu"
)

func menuHasStartSession(m Model) bool {
	return m.ticketMenu.IsOpen && strings.Contains(m.ticketMenu.View(), startSessionLabel)
}

func selectTicketByAddress(t *testing.T, m *Model, address string) {
	t.Helper()
	for i, e := range m.sidebarTree.Entries() {
		r := e.Value
		if r.kind == nodeTicket && !r.archived && m.epics[r.epicIdx].Tickets[r.ticketIdx].Path == address {
			m.sidebarTree.SetSelectedIndex(i)
			return
		}
	}
	t.Fatalf("no ticket row %q", address)
}

func startSessionModel(t *testing.T) Model {
	t.Helper()
	m := withState(newServerModel(t), server.Snapshot{Tickets: []server.TicketInfo{
		{Address: "gx:map/01", Title: "Open", Status: "open", EpicIsMap: true},
		{Address: "gx:map/02", Title: "Done", Status: "done", EpicIsMap: true},
		{Address: "gx:loop/01", Title: "Loop", Status: "open"},
	}})
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)
	setCollapsedSection(&m, sectionOpenMaps, false)
	setCollapsedSection(&m, sectionOpen, false)
	return m
}

func TestStartSession_OfferedOnMapEpicAndItsOpenTicketsOnly(t *testing.T) {
	m := startSessionModel(t)
	for _, tc := range []struct {
		address string
		want    bool
	}{{"gx:map/01", true}, {"gx:map/02", false}, {"gx:loop/01", false}} {
		selectTicketByAddress(t, &m, tc.address)
		_, ready := requestTicketMenu(t, m)
		got := slices.ContainsFunc(ready.items, func(it contextmenu.Item[ticketTarget]) bool {
			return it.Label == startSessionLabel
		})
		if got != tc.want {
			t.Errorf("%s: start session offered = %v, want %v", tc.address, got, tc.want)
		}
	}

	if menuHasStartSession(openEpicMenuOn(t, m, "gx:loop")) {
		t.Error("start session offered on a loop epic")
	}
	if !menuHasStartSession(openEpicMenuOn(t, m, "gx:map")) {
		t.Error("start session not offered on a map epic")
	}
}

func TestStartSession_CompletedMapEpicSuggestsStartAndMerge(t *testing.T) {
	m := withState(newServerModel(t), server.Snapshot{Tickets: []server.TicketInfo{
		{Address: "gx:done-map/01", Title: "Done", Status: "done", EpicIsMap: true},
	}})
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)
	setCollapsedSection(&m, sectionCompleteMaps, false)
	selectEpicRow(t, &m, "gx:done-map")
	r, _ := m.selectedRow()
	items := epicMenuItems(epicMenuInput{epic: m.epicAt(r)})
	var suggested []string
	for _, it := range items {
		if it.Group == contextmenu.Suggested {
			suggested = append(suggested, it.Label)
		}
	}
	if want := []string{startSessionLabel, "Merge"}; !slices.Equal(suggested, want) {
		t.Errorf("suggested = %v, want %v", suggested, want)
	}
}

func TestStartSession_RunsWithoutAServer(t *testing.T) {
	m := startSessionModel(t)
	m.serverAPI = nil
	m = openEpicMenuOn(t, m, "gx:map")
	if _, cmd := chooseTicketMenuItem(t, m, "Start session"); cmd == nil {
		t.Error("start session issued nothing on a map epic with no server")
	}
}

func TestStartSession_ChoosingItRequestsThePrefilledForm(t *testing.T) {
	for _, tc := range []struct {
		address string
		epicRow bool
		want    StartSessionMsg
	}{
		{"gx:map", true, StartSessionMsg{Project: "gx", Name: "map", Prompt: "/gx-design gx:map", Ticket: "gx:map"}},
		{"gx:map/01", false, StartSessionMsg{Project: "gx", Name: "map/01", Prompt: "/gx-design gx:map/01", Ticket: "gx:map/01"}},
	} {
		var calls []string
		m := startSessionModel(t).WithServer(fakeServerAPI{calls: &calls})
		var cmd tea.Cmd
		if tc.epicRow {
			_, cmd = chooseTicketMenuItem(t, openEpicMenuOn(t, m, tc.address), "Start session")
		} else {
			selectTicketByAddress(t, &m, tc.address)
			_, cmd = chooseTicketMenuItem(t, pressMenuKey(t, m), "Start session")
		}
		if cmd == nil {
			t.Fatalf("%s: choosing start session issued no cmd", tc.address)
		}
		if got := cmd(); got != tc.want {
			t.Errorf("%s: msg = %+v, want %+v", tc.address, got, tc.want)
		}
		if len(calls) != 0 {
			t.Errorf("%s: server calls = %v, want none (no ticket status written)", tc.address, calls)
		}
	}
}
