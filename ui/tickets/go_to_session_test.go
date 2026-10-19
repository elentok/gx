package tickets

import (
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/ui/contextmenu"
)

func goToSessionModel(t *testing.T, ticketStatus string, sessions ...server.SessionInfo) Model {
	t.Helper()
	m := newServerModel(t).WithServer(fakeServerAPI{})
	m = withState(m, server.Snapshot{
		Tickets: []server.TicketInfo{
			{Address: "gx:map/01", Title: "One", Status: ticketStatus, EpicIsMap: true},
			{Address: "gx:map/02", Title: "Two", Status: "open", EpicIsMap: true},
		},
		Sessions: sessions,
	})
	setCollapsedSection(&m, sectionOpenMaps, false)
	return m
}

func menuLabels(items []contextmenu.Item[ticketTarget]) []string {
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
	return menuLabels(ready.items)
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
		m := goToSessionModel(t, "open", server.SessionInfo{ID: "s1", Ticket: tc.ticket})
		m = openEpicMenuOn(t, m, "gx:map")
		if got := m.ticketMenu.IsOpen && menuViewHas(m, goToSessionLabel); got != tc.want {
			t.Errorf("%s: go to session offered = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func menuViewHas(m Model, label string) bool {
	return strings.Contains(m.ticketMenu.View(), label)
}

func TestGoToSession_TicketMenuMatching(t *testing.T) {
	linked := func(ticket string) server.SessionInfo { return server.SessionInfo{ID: "s1", Ticket: ticket} }
	for _, tc := range []struct {
		name   string
		status string
		s      server.SessionInfo
		want   bool
	}{
		{"ticket-linked", "open", linked("gx:map/01"), true},
		{"epic-linked, open ticket", "open", linked("gx:map"), false},
		{"epic-linked, claimed ticket", "claimed", linked("gx:map"), true},
		{"sibling ticket", "open", linked("gx:map/02"), false},
		{"exited session", "open", server.SessionInfo{ID: "s1", Ticket: "gx:map/01", Status: server.SessionStatus{State: server.SessionExited}}, true},
	} {
		labels := ticketMenuLabels(t, goToSessionModel(t, tc.status, tc.s), "gx:map/01")
		if got := slices.Contains(labels, goToSessionLabel); got != tc.want {
			t.Errorf("%s: offered = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestGoToSession_ListedAboveStartSession(t *testing.T) {
	labels := ticketMenuLabels(t, goToSessionModel(t, "open", server.SessionInfo{ID: "s1", Ticket: "gx:map/01"}), "gx:map/01")
	goTo, start := slices.Index(labels, goToSessionLabel), slices.Index(labels, startSessionLabel)
	if goTo < 0 || start < 0 || goTo > start {
		t.Errorf("labels = %v, want %q above %q", labels, goToSessionLabel, startSessionLabel)
	}
}

func TestGoToSession_MostRecentlyChangedWins(t *testing.T) {
	now := time.Now()
	sessions := []server.SessionInfo{
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
	if got := cmd(); got != (GoToSessionMsg{ID: "new"}) {
		t.Errorf("msg = %+v, want session new", got)
	}
}
