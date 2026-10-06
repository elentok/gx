package tickets

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/keys"
)

type fakeServerAPI struct{}

func (fakeServerAPI) Snapshot(context.Context) (server.Snapshot, error) {
	return server.Snapshot{}, nil
}
func (fakeServerAPI) Events(context.Context, uint64) (<-chan server.Event, error) { return nil, nil }
func (fakeServerAPI) QueueItems(context.Context) ([]server.QueueItem, error)      { return nil, nil }

func newServerModel(t *testing.T) Model {
	t.Helper()
	return NewModelWithStore(t.TempDir(), ui.Settings{}, keys.New(nil), loadQueueStoreAt(filepath.Join(t.TempDir(), "queue.json"))).WithServer(fakeServerAPI{})
}

func TestServerMode_SnapshotRendersReducedRows(t *testing.T) {
	m := newServerModel(t)
	snap := server.Snapshot{Seq: 4, Tickets: []server.TicketInfo{
		{Address: "gx:alpha/01", Title: "First", Status: "open"},
		{Address: "gx:alpha/02", Title: "Fork", Status: "open", Parent: "gx:alpha/01"},
	}}
	next, cmd, ok := m.updateServer(serverSnapshotMsg{snap: snap})
	if !ok || cmd == nil {
		t.Fatalf("snapshot not handled: ok=%v cmd=%v", ok, cmd)
	}
	if len(next.epics) != 1 || len(next.epics[0].Tickets) != 2 {
		t.Fatalf("epics = %+v", next.epics)
	}

	// An event the reducer applies changes the rows without a disk read.
	next, _, _ = next.updateServer(serverEventMsg{ev: server.Event{Seq: 5, Type: server.EventTicketDone, Address: "gx:alpha/01"}})
	if got := next.epics[0].Tickets[0].Status; got != "done" {
		t.Fatalf("status after event = %q", got)
	}
}

func TestServerMode_GapAndReconnectResnapshot(t *testing.T) {
	m := newServerModel(t)
	m, _, _ = m.updateServer(serverSnapshotMsg{snap: server.Snapshot{Seq: 4}})

	// A seq gap asks for a re-snapshot.
	_, cmd, _ := m.updateServer(serverEventMsg{ev: server.Event{Seq: 9, Type: server.EventQueueChanged}})
	if cmd == nil {
		t.Fatal("gap produced no command")
	}

	// The stream ending (reconnect) re-snapshots.
	_, cmd, ok := m.updateServer(serverStreamEndedMsg{})
	if !ok || cmd == nil {
		t.Fatal("stream end produced no command")
	}
	if _, isSnap := cmd().(serverSnapshotMsg); !isSnap {
		t.Fatalf("stream end cmd did not fetch a snapshot")
	}
}

func TestServerMode_QuitNotGuarded(t *testing.T) {
	if !newServerModel(t).CanQuit() {
		t.Fatal("server mode must not guard quit")
	}
}
