package tickets

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/ui/tickets/sidebar"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/ui/contextmenu"
	"github.com/elentok/gx/ui/tickets/internal/tickettest"
	"github.com/elentok/gx/ui/tickets/rowactions"
	"github.com/elentok/gx/ui/tickets/tabapi"
)

func goToSessionModel(t *testing.T, ticketStatus string, sessions ...wire.SessionInfo) Model {
	t.Helper()
	m := newServerModel(t).WithServer(tickettest.FakeServerAPI{}).WithTickets(tickettest.FakeServerAPI{}).WithAdmin(tickettest.FakeAdmin{FakeServerAPI: tickettest.FakeServerAPI{}}).WithQueue(tickettest.FakeQueue{FakeServerAPI: tickettest.FakeServerAPI{}}).WithReads(tickettest.FakeReads{FakeServerAPI: tickettest.FakeServerAPI{}})
	m = withState(m, wire.Snapshot{
		Tickets: []wire.TicketInfo{
			{Address: "gx:map/01", Title: "One", Status: ticketStatus, EpicIsMap: true},
			{Address: "gx:map/02", Title: "Two", Status: "open", EpicIsMap: true},
		},
		Sessions: sessions,
	})
	setCollapsedSection(&m, sidebar.SectionOpenMaps, false)
	return m
}

func menuLabels(items []contextmenu.Item[rowactions.TicketTarget]) []string {
	var labels []string
	for _, it := range items {
		labels = append(labels, it.Label)
	}
	return labels
}

func ticketMenuLabels(t *testing.T, m Model, address string) []string {
	t.Helper()
	selectTicketByAddress(t, &m, address)
	_, ready := requestTicketMenu(t, m)
	return menuLabels(ready.Items)
}

func TestGoToSession_EpicMenuMatchesEpicOrAnyTicketSession(t *testing.T) {
	for _, tc := range []struct {
		name, ticket string
		want         bool
	}{
		{"none", "", false},
		{"epic-linked", "gx:map", true},
		{"ticket-linked", "gx:map/02", true},
		{"other epic", "gx:other", false},
		{"epic name prefix only", "gx:map-two/01", false},
	} {
		m := goToSessionModel(t, "open", wire.SessionInfo{ID: "s1", Ticket: tc.ticket})
		m = openEpicMenuOn(t, m, "gx:map")
		if got := m.menus.Menu.IsOpen && menuViewHas(m, rowactions.GoToSessionLabel); got != tc.want {
			t.Errorf("%s: go to session offered = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func menuViewHas(m Model, label string) bool {
	return strings.Contains(m.menus.Menu.View(), label)
}

func TestGoToSession_TicketMenuMatching(t *testing.T) {
	linked := func(ticket string) wire.SessionInfo { return wire.SessionInfo{ID: "s1", Ticket: ticket} }
	for _, tc := range []struct {
		name   string
		status string
		s      wire.SessionInfo
		want   bool
	}{
		{"ticket-linked", "open", linked("gx:map/01"), true},
		{"epic-linked, open ticket", "open", linked("gx:map"), false},
		{"epic-linked, claimed ticket", "claimed", linked("gx:map"), true},
		{"sibling ticket", "open", linked("gx:map/02"), false},
		{"exited session", "open", wire.SessionInfo{ID: "s1", Ticket: "gx:map/01", Status: wire.SessionStatus{State: wire.SessionExited}}, true},
	} {
		labels := ticketMenuLabels(t, goToSessionModel(t, tc.status, tc.s), "gx:map/01")
		if got := slices.Contains(labels, rowactions.GoToSessionLabel); got != tc.want {
			t.Errorf("%s: offered = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestGoToSession_NotOfferedOnNonMapTicket(t *testing.T) {
	m := newServerModel(t).WithServer(tickettest.FakeServerAPI{}).WithTickets(tickettest.FakeServerAPI{}).WithAdmin(tickettest.FakeAdmin{FakeServerAPI: tickettest.FakeServerAPI{}}).WithQueue(tickettest.FakeQueue{FakeServerAPI: tickettest.FakeServerAPI{}}).WithReads(tickettest.FakeReads{FakeServerAPI: tickettest.FakeServerAPI{}})
	m = withState(m, wire.Snapshot{
		Tickets:  []wire.TicketInfo{{Address: "gx:plain/01", Title: "One", Status: "claimed"}},
		Sessions: []wire.SessionInfo{{ID: "s1", Ticket: "gx:plain/01"}},
	})
	setCollapsedSection(&m, sidebar.SectionOpenMaps, false)
	labels := ticketMenuLabels(t, m, "gx:plain/01")
	if slices.Contains(labels, rowactions.GoToSessionLabel) {
		t.Errorf("labels = %v, want no %q on a non-map ticket", labels, rowactions.GoToSessionLabel)
	}
}

func TestGoToSession_ListedAboveStartSession(t *testing.T) {
	labels := ticketMenuLabels(t, goToSessionModel(t, "open", wire.SessionInfo{ID: "s1", Ticket: "gx:map/01"}), "gx:map/01")
	goTo, start := slices.Index(labels, rowactions.GoToSessionLabel), slices.Index(labels, rowactions.StartSessionLabel)
	if goTo < 0 || start < 0 || goTo > start {
		t.Errorf("labels = %v, want %q above %q", labels, rowactions.GoToSessionLabel, rowactions.StartSessionLabel)
	}
}

func TestGoToSession_MostRecentlyChangedWins(t *testing.T) {
	now := time.Now()
	sessions := []wire.SessionInfo{
		{ID: "old", Ticket: "gx:map/01", StatusChangedAt: now.Add(-time.Hour)},
		{ID: "new", Ticket: "gx:map", StatusChangedAt: now},
		{ID: "mid", Ticket: "gx:map/01", StatusChangedAt: now.Add(-time.Minute)},
	}
	m := goToSessionModel(t, "claimed", sessions...)
	selectTicketByAddress(t, &m, "gx:map/01")
	m = pressMenuKey(t, m)
	var updated tea.Model = m
	for _, r := range "Go to design session" {
		updated, _ = updated.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	_, cmd := updated.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("choosing go to session issued no cmd")
	}
	if got := cmd(); got != (tabapi.GoToSessionMsg{ID: "new"}) {
		t.Errorf("msg = %+v, want session new", got)
	}
}
