package app

import (
	"reflect"
	"testing"

	"github.com/elentok/gx/ui/nav"
	ticketsui "github.com/elentok/gx/ui/tickets"
)

// ticketsTabChecked reads the Tickets page's private checked set.
func ticketsTabChecked(t *testing.T, m Model) map[string]bool {
	t.Helper()
	page, ok := m.livePageByTab[nav.TabTickets].model.(ticketsui.Model)
	if !ok {
		t.Fatal("no Tickets page")
	}
	got := map[string]bool{}
	for _, k := range reflect.ValueOf(page).FieldByName("checked").MapKeys() {
		got[k.String()] = true
	}
	return got
}

func TestCheckPaths_ReachANeverOpenedTicketsTabAndSurviveTheSwitch(t *testing.T) {
	h := newHarness(t, &fakeServerClient{})
	h.switchTo(nav.TabQueue)

	h.send(ticketsui.CheckPathsMsg{Paths: []string{"gx:alpha/01", "gx:alpha/02"}})
	h.switchTo(nav.TabTickets)

	want := map[string]bool{"gx:alpha/01": true, "gx:alpha/02": true}
	if got := ticketsTabChecked(t, h.m); !reflect.DeepEqual(got, want) {
		t.Errorf("Tickets tab checked = %v, want %v", got, want)
	}
}
