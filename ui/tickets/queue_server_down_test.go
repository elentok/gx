package tickets

import (
	"context"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/keys"
)

func newServerQueueModel(t *testing.T, start func(context.Context) error) QueueModel {
	t.Helper()
	m := NewQueueModelWithStore(t.TempDir(), ui.Settings{}, keys.New(nil), loadQueueStoreAt(filepath.Join(t.TempDir(), "queue.json"))).
		WithServerLink(fakeServerAPI{}, start)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m = next.(QueueModel)
	m.loaded = true
	m.epics = []tickets.Epic{{Name: "gx:alpha", Tickets: []tickets.Ticket{{Number: 1, Identifier: "01", Title: "First", Path: "gx:alpha/01", Status: "open"}}}}
	m.checked = map[string]bool{"gx:alpha/01": true}
	m.clampSelected()
	return m
}

func TestQueueServerDown_ClearsRowsAndShowsBanner(t *testing.T) {
	m := newServerQueueModel(t, func(context.Context) error { return nil })
	next, cmd := m.Update(ServerDownMsg{})
	m = next.(QueueModel)
	if cmd == nil {
		t.Fatal("no background reconnect scheduled")
	}
	if len(m.epics) != 0 {
		t.Fatalf("rows not cleared: %+v", m.epics)
	}
	view := ansi.Strip(m.View().Content)
	if !strings.Contains(view, "server down — s to start") || strings.Contains(view, "First") {
		t.Fatalf("view = %s", view)
	}

	// Coming back clears the banner and reloads.
	next, cmd = m.Update(ServerUpMsg{})
	m = next.(QueueModel)
	if m.serverDown || cmd == nil {
		t.Fatalf("up: down=%v cmd=%v", m.serverDown, cmd)
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

	next, _ = m.Update(ServerDownMsg{})
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
	if _, ok := cmd().(queueServerStartedMsg); !ok || !started {
		t.Fatalf("started = %v", started)
	}
}

// In server mode the Queue tab shows the server's queue, never the local
// queue store a pre-server run left behind.
func TestQueueServerMode_LoadsRowsAndQueuedSetFromServer(t *testing.T) {
	store := loadQueueStoreAt(filepath.Join(t.TempDir(), "queue.json"))
	if err := store.SetChecked([]string{"/old/epic/issues/01-stale.md"}, true); err != nil {
		t.Fatal(err)
	}
	api := fakeServerAPI{
		snap: server.Snapshot{Seq: 1, Tickets: []server.TicketInfo{
			{Address: "gx:alpha/01", Title: "First", Status: "open"},
			{Address: "gx:alpha/02", Title: "Second", Status: "open"},
		}},
		queue: []server.QueueItem{{Address: "gx:alpha/02"}, {Address: "gx:alpha/01"}},
	}
	m := NewQueueModelWithStore(t.TempDir(), ui.Settings{}, keys.New(nil), store).WithServerLink(api, nil)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m = next.(QueueModel)

	next, _ = m.Update(m.cmdLoadQueue()())
	m = next.(QueueModel)

	if want := map[string]bool{"gx:alpha/01": true, "gx:alpha/02": true}; !reflect.DeepEqual(m.checked, want) {
		t.Errorf("checked = %v, want the server queue %v", m.checked, want)
	}
	if m.checkOrder["gx:alpha/02"] >= m.checkOrder["gx:alpha/01"] {
		t.Errorf("checkOrder = %v, want the server's order (02 before 01)", m.checkOrder)
	}
	if len(m.epics) != 1 || m.epics[0].Name != "gx:alpha" || len(m.epics[0].Tickets) != 2 {
		t.Errorf("epics = %+v, want gx:alpha with both server tickets", m.epics)
	}
	if !store.IsChecked("/old/epic/issues/01-stale.md") {
		t.Error("server-mode load rewrote the local queue store")
	}
}

// Seam D: a snapshot with two projects renders project-prefixed rows, and "tp"
// filters them by project.
func TestQueueServerMode_PrefixesRowsAndFiltersByProject(t *testing.T) {
	api := fakeServerAPI{snap: server.Snapshot{Seq: 1, Tickets: []server.TicketInfo{
		{Address: "gx:alpha/01", Title: "First", Status: "open"},
		{Address: "blog:beta/01", Title: "Second", Status: "open"},
	}}, queue: []server.QueueItem{{Address: "gx:alpha/01"}, {Address: "blog:beta/01"}}}
	m := NewQueueModelWithStore(t.TempDir(), ui.Settings{}, keys.New(nil), loadQueueStoreAt(filepath.Join(t.TempDir(), "queue.json"))).WithServerLink(api, nil)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	next, _ = next.(QueueModel).Update(next.(QueueModel).cmdLoadQueue()())
	m = next.(QueueModel)

	view := ansi.Strip(m.View().Content)
	for _, want := range []string{"gx: 01 First", "blog: 01 Second"} {
		if !strings.Contains(view, want) {
			t.Errorf("view missing %q:\n%s", want, view)
		}
	}

	m.projectFilter = nextProject(m.epics, "")
	view = ansi.Strip(m.View().Content)
	if !strings.Contains(view, "blog: 01 Second") || strings.Contains(view, "gx: 01 First") {
		t.Errorf("filter %q should show only blog:\n%s", m.projectFilter, view)
	}
	if got := nextProject(m.epics, "gx"); got != "" {
		t.Errorf("after the last project the filter should return to all, got %q", got)
	}
}
