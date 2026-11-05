package queue

import (
	"context"
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/keys"
	"github.com/elentok/gx/ui/tickets/internal/tickettest"
	"github.com/elentok/gx/ui/tickets/tabapi"
	"github.com/elentok/gx/viewmodel"
)

func newServerQueueModel(t *testing.T, start func(context.Context) error) QueueModel {
	t.Helper()
	m := NewQueueModel(t.TempDir(), ui.Settings{}, keys.New(nil)).
		WithServerLink(tickettest.FakeServerAPI{}, start)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m = next.(QueueModel)
	m.loaded = true
	m.rows.Epics = []tickets.Epic{{Name: "gx:alpha", Tickets: []tickets.Ticket{{Number: 1, Identifier: "01", Title: "First", Path: "gx:alpha/01", Status: "open"}}}}
	m.rows.Checked = map[string]bool{"gx:alpha/01": true}
	m.clampSelected()
	return m
}

func TestQueueServerDown_ClearsRowsAndShowsBanner(t *testing.T) {
	m := newServerQueueModel(t, func(context.Context) error { return nil })
	next, cmd := m.WithServerState(nil, tabapi.ServerLinkDown)
	m = next.(QueueModel)
	if cmd != nil {
		t.Fatal("the tab must not run a reconnect loop of its own")
	}
	if len(m.rows.Epics) != 0 {
		t.Fatalf("rows not cleared: %+v", m.rows.Epics)
	}
	view := ansi.Strip(m.View().Content)
	if !strings.Contains(view, "server not running — s to start") || strings.Contains(view, "First") {
		t.Fatalf("view = %s", view)
	}

	// Coming back clears the banner and brings the rows, in one delivery.
	st := applySnapshot(viewmodel.State{}, wire.Snapshot{Seq: 1, Tickets: []wire.TicketInfo{
		{Address: "gx:alpha/01", Title: "First", Status: "open"},
	}}).SetQueue([]wire.QueueItem{{Address: "gx:alpha/01"}})
	next, _ = m.WithServerState(&st, tabapi.ServerLinkUp)
	if next.(QueueModel).server.Down() {
		t.Fatal("still down after an up delivery")
	}
	if view := ansi.Strip(next.(QueueModel).View().Content); !strings.Contains(view, "First") {
		t.Fatalf("delivered rows not shown after reconnect:\n%s", view)
	}
}

// A state delivered with a down link is not shown; the same state delivered
// with an up link is.
func TestQueueServerDown_StateDeliveredDownShowsOnceUp(t *testing.T) {
	m := newServerQueueModel(t, func(context.Context) error { return nil })

	st := applySnapshot(viewmodel.State{}, wire.Snapshot{Seq: 2, Tickets: []wire.TicketInfo{
		{Address: "gx:alpha/09", Title: "Ninth", Status: "open"},
	}}).SetQueue([]wire.QueueItem{{Address: "gx:alpha/09"}})
	next, _ := m.WithServerState(&st, tabapi.ServerLinkDown)
	m = next.(QueueModel)
	if len(m.rows.Epics) != 0 {
		t.Fatalf("rows shown while down: %+v", m.rows.Epics)
	}

	next, _ = m.WithServerState(&st, tabapi.ServerLinkUp)
	if view := ansi.Strip(next.(QueueModel).View().Content); !strings.Contains(view, "Ninth") {
		t.Fatalf("delivered rows not shown after reconnect:\n%s", view)
	}
}

func TestQueueServerDown_SOpensStartConfirm(t *testing.T) {
	started := false
	m := newServerQueueModel(t, func(context.Context) error { started = true; return nil })

	// Up: "s" does nothing.
	next, _ := m.Update(tea.KeyPressMsg{Code: 's', Text: "s"})
	if next.(QueueModel).confirm.IsOpen {
		t.Fatal("confirm opened while server is up")
	}

	next, _ = m.WithServerState(nil, tabapi.ServerLinkDown)
	next, _ = next.Update(tea.KeyPressMsg{Code: 's', Text: "s"})
	m = next.(QueueModel)
	if !m.confirm.IsOpen || !strings.Contains(m.confirm.View(80), "server is down — start it now?") {
		t.Fatalf("confirm = %q", m.confirm.View(80))
	}

	next, cmd := m.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	if cmd == nil {
		t.Fatal("accept returned no command")
	}
	next, cmd = next.Update(cmd())
	if cmd == nil {
		t.Fatal("start not issued")
	}
	if _, ok := cmd().(ServerStartedMsg); !ok || !started {
		t.Fatalf("started = %v", started)
	}
}

// In server mode the Queue tab shows the server's queue.
func TestQueueServerMode_LoadsRowsAndQueuedSetFromServer(t *testing.T) {
	m, _ := deliverQueue(t, wire.Snapshot{Seq: 1, Tickets: []wire.TicketInfo{
		{Address: "gx:alpha/01", Title: "First", Status: "open"},
		{Address: "gx:alpha/02", Title: "Second", Status: "open"},
	}}, []wire.QueueItem{{Address: "gx:alpha/02"}, {Address: "gx:alpha/01"}})

	if want := map[string]bool{"gx:alpha/01": true, "gx:alpha/02": true}; !reflect.DeepEqual(m.rows.Checked, want) {
		t.Errorf("checked = %v, want the server queue %v", m.rows.Checked, want)
	}
	if m.rows.CheckOrder["gx:alpha/02"] >= m.rows.CheckOrder["gx:alpha/01"] {
		t.Errorf("checkOrder = %v, want the server's order (02 before 01)", m.rows.CheckOrder)
	}
	if len(m.rows.Epics) != 1 || m.rows.Epics[0].Name != "gx:alpha" || len(m.rows.Epics[0].Tickets) != 2 {
		t.Errorf("epics = %+v, want gx:alpha with both server tickets", m.rows.Epics)
	}
}

// Seam D: a snapshot with two projects names each project in its epic header
// only — a row prefix misaligned idle titles against live ones — and "tp"
// filters them by project.
func TestQueueServerMode_ProjectInEpicHeaderAndFiltersByProject(t *testing.T) {
	m, _ := deliverQueue(t, wire.Snapshot{Seq: 1, Tickets: []wire.TicketInfo{
		{Address: "gx:alpha/01", Title: "First", Status: "open"},
		{Address: "blog:beta/01", Title: "Second", Status: "open"},
	}}, []wire.QueueItem{{Address: "gx:alpha/01"}, {Address: "blog:beta/01"}})

	view := ansi.Strip(m.View().Content)
	for _, want := range []string{"gx:alpha", "blog:beta", " 01 First", " 01 Second"} {
		if !strings.Contains(view, want) {
			t.Errorf("view missing %q:\n%s", want, view)
		}
	}
	for _, unwanted := range []string{"gx: 01", "blog: 01"} {
		if strings.Contains(view, unwanted) {
			t.Errorf("view has project-prefixed row %q:\n%s", unwanted, view)
		}
	}

	m.rows.ProjectFilter = NextProject(m.rows.Epics, "")
	view = ansi.Strip(m.View().Content)
	if !strings.Contains(view, "01 Second") || strings.Contains(view, "01 First") {
		t.Errorf("filter %q should show only blog:\n%s", m.rows.ProjectFilter, view)
	}
	if got := NextProject(m.rows.Epics, "gx"); got != "" {
		t.Errorf("after the last project the filter should return to all, got %q", got)
	}
}

func TestQueueFocusEpic_SelectsTheEpicHeaderOnceItsRowsArrive(t *testing.T) {
	m := newServerQueueModel(t, func(context.Context) error { return nil }).WithFocusEpic("gx:beta")
	tickets := []wire.TicketInfo{
		{Address: "gx:alpha/01", Title: "First", Status: "open", RenderedStatus: "open"},
		{Address: "gx:beta/01", Title: "Second", Status: "open", RenderedStatus: "open"},
	}
	st := applySnapshot(viewmodel.State{}, wire.Snapshot{
		Seq:     1,
		Tickets: tickets,
		Epics:   fixtureSummaries(tabapi.EpicsFromViewModel(viewmodel.State{Tickets: tickets}, viewmodel.Scope{}), nil),
	}).SetQueue([]wire.QueueItem{{Address: "gx:alpha/01"}, {Address: "gx:beta/01"}})
	next, _ := m.WithServerState(&st, tabapi.ServerLinkUp)
	m = next.(QueueModel)

	entry := m.rows.Tree.Entries()[m.rows.Tree.SelectedIndex()].Value
	if entry.Kind != NodeEpicName || entry.Epic.Path != "gx:beta" {
		t.Fatalf("selected = %+v, want the gx:beta header", entry)
	}
}
