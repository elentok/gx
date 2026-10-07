package tickets

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

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
