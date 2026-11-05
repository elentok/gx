package serverlink

import (
	"slices"
	"testing"

	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/ui/tickets/internal/tickettest"
	"github.com/elentok/gx/ui/tickets/tabapi"
	"github.com/elentok/gx/viewmodel"
)

// stateOf makes snap look like one from a current server, which always sends
// rendered_status: a ticket without one gets its raw status word. A snapshot
// with none at all reads as an older server's.
func stateOf(snap wire.Snapshot) viewmodel.State {
	if snap.Seq == 0 {
		snap.Seq = 1
	}
	snap.Tickets = slices.Clone(snap.Tickets)
	for i := range snap.Tickets {
		if snap.Tickets[i].RenderedStatus == "" {
			snap.Tickets[i].RenderedStatus = snap.Tickets[i].Status
		}
	}
	return viewmodel.State{}.ApplySnapshot(snap)
}

// connected is a binding with a client, as the tab has once WithServer ran.
func connected() *Binding { return &Binding{API: tickettest.FakeServerAPI{}} }

var scopeSnap = wire.Snapshot{Seq: 4, Tickets: []wire.TicketInfo{
	{Address: "gx:alpha/01", Status: "open"},
	{Address: "other:beta/01", Status: "open"},
}}

func TestBinding_SnapshotRendersReducedRows(t *testing.T) {
	b := connected()
	st := stateOf(wire.Snapshot{Seq: 4, Tickets: []wire.TicketInfo{
		{Address: "gx:alpha/01", Title: "First", Status: "open"},
		{Address: "gx:alpha/02", Title: "Fork", Status: "open", Parent: "gx:alpha/01"},
	}})
	if apply, _ := b.Receive(&st, tabapi.ServerLinkUp); !apply {
		t.Fatal("a snapshot over a live link must apply")
	}
	if epics := b.Epics(); len(epics) != 1 || len(epics[0].Tickets) != 2 {
		t.Fatalf("epics = %+v", epics)
	}
}

func TestBinding_ScopesToCwdProjectAndToggles(t *testing.T) {
	b := connected()
	b.Scope.CwdProject = "gx"
	st := stateOf(scopeSnap)
	b.Receive(&st, tabapi.ServerLinkUp)
	if epics := b.Epics(); len(epics) != 1 || epics[0].Name != "gx:alpha" {
		t.Fatalf("scoped epics = %+v", epics)
	}
	if msg, ok := b.ToggleScope(); !ok || msg != "all projects" {
		t.Fatalf("toggle = %q, %v", msg, ok)
	}
	if got := len(b.Epics()); got != 2 {
		t.Fatalf("all-projects epics = %d, want 2", got)
	}
}

func TestBinding_ScopeToggleSurvivesDeliveredState(t *testing.T) {
	b := connected()
	b.Scope.CwdProject = "gx"
	st := stateOf(scopeSnap)
	b.Receive(&st, tabapi.ServerLinkUp)
	b.ToggleScope()

	b.Receive(&st, tabapi.ServerLinkUp)
	if got := len(b.Epics()); got != 2 {
		t.Fatalf("epics after delivery = %d, want 2 (still all projects)", got)
	}
}

// Seam D: the tab reads the store only while the server is down.
func TestBinding_ReadsDiskFollowsConnection(t *testing.T) {
	b := connected()
	if b.ReadsDisk() {
		t.Fatal("connected tab reads the store")
	}

	if _, load := b.Receive(nil, tabapi.ServerLinkDown); !load || !b.ReadsDisk() {
		t.Fatal("going down must read the store and load from disk")
	}
	if _, load := b.Receive(nil, tabapi.ServerLinkDown); load {
		t.Fatal("a repeated down delivery must not reload")
	}
	st := stateOf(scopeSnap)
	if apply, _ := b.Receive(&st, tabapi.ServerLinkDown); apply || len(b.Epics()) != 0 {
		t.Fatal("fallback tab took the shell's stale state")
	}

	// The shell re-snapshots on reconnect and delivers; the tab fetches nothing.
	if _, load := b.Receive(nil, tabapi.ServerLinkUp); load || b.ReadsDisk() {
		t.Fatal("reconnecting must not load from disk")
	}
}

// Seam B: with no client the tab starts, and stays, reading the store.
func TestBinding_NoClientReadsDisk(t *testing.T) {
	b := &Binding{}
	b.Receive(nil, tabapi.ServerLinkDown)
	if !b.ReadsDisk() || b.Older {
		t.Fatal("a tab with no client must read the store and show no older-server notice")
	}
	if _, blocked := b.DisabledToast(); !blocked {
		t.Fatal("server keys are not blocked without a server")
	}
}

func TestBinding_OlderServerShowsNoTickets(t *testing.T) {
	b := connected()
	older := viewmodel.State{}.ApplySnapshot(wire.Snapshot{Seq: 1, Tickets: []wire.TicketInfo{
		{Address: "gx:alpha/01", Title: "First", Status: "open"},
	}})
	b.Receive(&older, tabapi.ServerLinkUp)
	if !b.Older || len(b.Epics()) != 0 {
		t.Fatalf("older = %v, epics = %d; want the notice and no ticket data", b.Older, len(b.Epics()))
	}

	current := stateOf(wire.Snapshot{Seq: 2, Tickets: []wire.TicketInfo{
		{Address: "gx:alpha/01", Title: "First", Status: "open"},
	}})
	b.Receive(&current, tabapi.ServerLinkUp)
	if b.Older || len(b.Epics()) != 1 {
		t.Fatalf("older = %v, epics = %d; a current server's snapshot must clear the notice", b.Older, len(b.Epics()))
	}
}
