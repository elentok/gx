package tabapi

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/elentok/gx/server/wire"
	gxtickets "github.com/elentok/gx/tickets"
	"github.com/elentok/gx/viewmodel"
)

// stateOf applies snap as a current server sends it, which always carries
// rendered_status: a ticket without one gets its raw status word.
func stateOf(snap wire.Snapshot) viewmodel.State {
	snap.Seq = 1
	for i := range snap.Tickets {
		snap.Tickets[i].RenderedStatus = snap.Tickets[i].Status
	}
	return viewmodel.State{}.ApplySnapshot(snap)
}

func TestEpicsFromViewModel_KeepsTheDoneMetrics(t *testing.T) {
	vm := stateOf(wire.Snapshot{Tickets: []wire.TicketInfo{
		{Address: "proj:epic-a/01", Status: "done", ActualContextWindow: 66395, ElapsedTime: 316, Compactions: 2},
	}})
	epics := EpicsFromViewModel(vm, viewmodel.Scope{})
	if len(epics) != 1 || len(epics[0].Tickets) != 1 {
		t.Fatalf("epics = %+v", epics)
	}
	got := epics[0].Tickets[0]
	if got.ActualContextWindow != 66395 || got.ElapsedTime != 316 || got.Compactions != 2 {
		t.Errorf("ticket = %+v, want the done metrics so the summary can add them up", got)
	}
}

func TestEpicsFromViewModel_MultiLevelForkKeepsItsNumber(t *testing.T) {
	vm := stateOf(wire.Snapshot{Tickets: []wire.TicketInfo{
		{Address: "proj:epic-a/27", Status: "done"},
		{Address: "proj:epic-a/27a", Status: "done", Parent: "proj:epic-a/27"},
		{Address: "proj:epic-a/27a1", Status: "done", Parent: "proj:epic-a/27a"},
		{Address: "proj:epic-a/27a3", Status: "claimed", Parent: "proj:epic-a/27a1"},
	}})
	epics := EpicsFromViewModel(vm, viewmodel.Scope{})
	if len(epics) != 1 || len(epics[0].Tickets) != 4 {
		t.Fatalf("epics = %+v", epics)
	}
	epic := epics[0]
	for _, tk := range epic.Tickets {
		if tk.Number != 27 {
			t.Errorf("%s: Number = %d, want 27", tk.Identifier, tk.Number)
		}
	}
	if got := epic.RenderedStatus(epic.Tickets[3]); got != gxtickets.StatusClaimed {
		t.Errorf("27a3 RenderedStatus = %v, want claimed: its parent 27a1 is done", got)
	}
}

func TestWatchStore_FiresOnStoreChange(t *testing.T) {
	dir := t.TempDir()
	events, stop, err := WatchStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	// Under heavy disk load (the full parallel suite) FSEvents can delay a
	// write by seconds or drop it, so keep writing until one gets through.
	write := time.NewTicker(500 * time.Millisecond)
	defer write.Stop()
	deadline := time.After(20 * time.Second)
	for i := 0; ; i++ {
		if err := os.WriteFile(filepath.Join(dir, "x.md"), []byte{byte(i)}, 0o644); err != nil {
			t.Fatal(err)
		}
		select {
		case <-events:
			return
		case <-write.C:
		case <-deadline:
			t.Fatal("no change signal after store writes")
		}
	}
}
