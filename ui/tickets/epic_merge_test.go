package tickets

import (
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/ui/notify"
)

// openEpicMenuOn selects the epic row name and presses "m".
func openEpicMenuOn(t *testing.T, m Model, name string) Model {
	t.Helper()
	selectEpicRow(t, &m, name)
	updated, _ := m.Update(tea.KeyPressMsg{Code: 'm', Text: "m"})
	m = updated.(Model)
	if !m.ticketMenu.IsOpen {
		t.Fatalf("m on epic %s opened no menu", name)
	}
	return m
}

func TestEpicMerge_TicketsTabMergesCompleteUnmergedEpicByAddress(t *testing.T) {
	var calls []string
	api := fakeServerAPI{calls: &calls, mergeRes: server.QueueResult{MergeTicket: "gx:alpha/02"}}
	m := withState(newServerModel(t).WithServer(api), server.Snapshot{Tickets: []server.TicketInfo{
		{Address: "gx:alpha/01", Title: "Done", Status: "done"},
		{Address: "gx:beta/01", Title: "Open", Status: "open"},
	}})
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)
	setCollapsedSection(&m, sectionComplete, false)

	open := openEpicMenuOn(t, m, "gx:beta")
	if _, cmd := chooseTicketMenuItem(t, open, "Merge"); cmd != nil {
		t.Error("merge ran on an open epic")
	}

	_, cmd := chooseTicketMenuItem(t, openEpicMenuOn(t, m, "gx:alpha"), "Merge")
	if cmd == nil {
		t.Fatal("choosing merge issued no cmd")
	}
	toast, ok := cmd().(notify.NotifyMsg)
	if !ok {
		t.Fatalf("merge cmd = %T, want a toast", toast)
	}
	if !slices.Equal(calls, []string{"merge gx:alpha"}) {
		t.Errorf("calls = %v, want [merge gx:alpha]", calls)
	}
	if !strings.Contains(toast.Message, "gx:alpha/02") {
		t.Errorf("toast = %q, want the forked merge ticket", toast.Message)
	}
}

func TestEpicMerge_ServerModeMergedEpicSitsUnderMergedWithMergeDim(t *testing.T) {
	m := withState(newServerModel(t).WithServer(fakeServerAPI{calls: &[]string{}}), server.Snapshot{Tickets: []server.TicketInfo{
		{Address: "gx:alpha/01", Title: "Done", Status: "done", EpicMergedAt: time.Now()},
	}})
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)
	if got := epicSection(m.epics[0]); got != sectionMerged {
		t.Fatalf("section = %v, want sectionMerged", got)
	}
	setCollapsedSection(&m, sectionMerged, false)

	m = openEpicMenuOn(t, m, "gx:alpha")
	if view := m.ticketMenu.View(); !strings.Contains(view, alreadyMerged) {
		t.Errorf("view lacks %q:\n%s", alreadyMerged, view)
	}
}
