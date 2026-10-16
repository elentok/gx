package tickets

import (
	"slices"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/server"
)

func menuHasStartSession(m actionsMenuModel) bool {
	return m.IsOpen && slices.Contains(itemValues(m.state.Items), actionStartSession)
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
		if got := menuHasStartSession(pressMenuKey(t, m).actionsMenu); got != tc.want {
			t.Errorf("%s: start session offered = %v, want %v", tc.address, got, tc.want)
		}
	}

	selectEpicRow(t, &m, "gx:loop")
	updated, _ := m.Update(tea.KeyPressMsg{Code: 'm', Text: "m"})
	if menuHasStartSession(updated.(Model).actionsMenu) {
		t.Error("start session offered on a loop epic")
	}
	selectEpicRow(t, &m, "gx:map")
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'm', Text: "m"})
	if !menuHasStartSession(updated.(Model).actionsMenu) {
		t.Error("start session not offered on a map epic")
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
		if tc.epicRow {
			selectEpicRow(t, &m, tc.address)
			updated, _ := m.Update(tea.KeyPressMsg{Code: 'm', Text: "m"})
			m = updated.(Model)
		} else {
			selectTicketByAddress(t, &m, tc.address)
			m = pressMenuKey(t, m)
		}
		// Start session… is the first item.
		_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
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
