package tickets

import (
	"strings"
	"testing"

	"github.com/elentok/gx/server/wire"

	"github.com/elentok/gx/ui"
)

func sessionMarkerModel(t *testing.T, nerdFont bool, sessions ...wire.SessionInfo) Model {
	t.Helper()
	m := newServerModel(t)
	m.settings.UseNerdFontIcons = nerdFont
	m = withState(m, wire.Snapshot{
		Tickets: []wire.TicketInfo{
			{Address: "gx:map/01", Title: "Open", Status: "open", EpicIsMap: true},
			{Address: "gx:map/02", Title: "Other", Status: "open", EpicIsMap: true},
		},
		Sessions: sessions,
	})
	setCollapsedSection(&m, sectionOpenMaps, false)
	return m
}

func renderedTicketRow(t *testing.T, m Model, address string) string {
	t.Helper()
	for i, e := range m.sidebarTree.Entries() {
		if e.Value.kind != nodeTicket {
			continue
		}
		r, _ := rowFromEntry(e)
		if m.epicAt(r).Tickets[r.ticketIdx].Path == address {
			return strings.Join(m.renderTicketRow(m.epicAt(r), r, i), "\n")
		}
	}
	t.Fatalf("no ticket row %q", address)
	return ""
}

func TestSessionMarker_LiveLinkedSessionMarksItsRow(t *testing.T) {
	for _, nerdFont := range []bool{false, true} {
		marker := ui.Icons(nerdFont).SessionLive
		live := wire.SessionInfo{ID: "a", Ticket: "gx:map/01", Status: wire.SessionStatus{State: wire.SessionIdle}}
		m := sessionMarkerModel(t, nerdFont, live)
		if got := renderedTicketRow(t, m, "gx:map/01"); !strings.Contains(got, marker) {
			t.Errorf("nerdFont=%v: linked row %q lacks marker %q", nerdFont, got, marker)
		}
		if got := renderedTicketRow(t, m, "gx:map/02"); strings.Contains(got, marker) {
			t.Errorf("nerdFont=%v: unlinked row %q shows marker", nerdFont, got)
		}
		if got := m.renderEpicRow(m.epics[0]); strings.Contains(got, marker) {
			t.Errorf("nerdFont=%v: epic row %q shows marker for a ticket's session", nerdFont, got)
		}
	}
}

func TestSessionMarker_LiveSessionOnEpicMarksEpicRow(t *testing.T) {
	marker := ui.Icons(false).SessionLive
	m := sessionMarkerModel(t, false, wire.SessionInfo{ID: "a", Ticket: "gx:map", Status: wire.SessionStatus{State: wire.SessionIdle}})
	if got := m.renderEpicRow(m.epics[0]); !strings.Contains(got, marker) {
		t.Errorf("epic row %q lacks marker %q", got, marker)
	}
}

func TestSessionMarker_FollowsSessionExit(t *testing.T) {
	marker := ui.Icons(false).SessionLive
	live := wire.SessionInfo{ID: "a", Ticket: "gx:map/01", Status: wire.SessionStatus{State: wire.SessionWorking}}
	m := sessionMarkerModel(t, false, live)
	live.Status.State = wire.SessionExited
	m = withState(m, wire.Snapshot{
		Tickets:  []wire.TicketInfo{{Address: "gx:map/01", Title: "Open", Status: "open", EpicIsMap: true}},
		Sessions: []wire.SessionInfo{live},
	})
	if got := renderedTicketRow(t, m, "gx:map/01"); strings.Contains(got, marker) {
		t.Errorf("row %q still shows marker after the session exited", got)
	}
}

func TestSessionMarker_ExitedSessionMarksNothing(t *testing.T) {
	marker := ui.Icons(false).SessionLive
	m := sessionMarkerModel(t, false,
		wire.SessionInfo{ID: "a", Ticket: "gx:map/01", Status: wire.SessionStatus{State: wire.SessionExited}},
		wire.SessionInfo{ID: "b", Ticket: "gx:map", Status: wire.SessionStatus{State: wire.SessionExited}},
	)
	if got := renderedTicketRow(t, m, "gx:map/01"); strings.Contains(got, marker) {
		t.Errorf("row %q shows marker for an exited session", got)
	}
	if got := m.renderEpicRow(m.epics[0]); strings.Contains(got, marker) {
		t.Errorf("epic row %q shows marker for an exited session", got)
	}
}
