package server_test

import (
	"context"
	"testing"

	"github.com/elentok/gx/server/servertest"
)

func TestSnapshot_TicketsCarryRenderedStatus(t *testing.T) {
	t.Parallel()
	store := t.TempDir()
	servertest.WriteTicketWith(t, store, "proj", "epic", "01", "parent", servertest.TicketOpts{Status: "done"})
	servertest.WriteTicketWith(t, store, "proj", "epic", "02", "fork-child", servertest.TicketOpts{Parent: "01"})
	servertest.WriteTicketWith(t, store, "proj", "epic", "03", "waiting", servertest.TicketOpts{BlockedBy: []string{"04"}})
	servertest.WriteTicketWith(t, store, "proj", "epic", "04", "blocker", servertest.TicketOpts{})
	h := servertest.StartWithStore(t, store)

	snap, err := h.Client.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, ti := range snap.Tickets {
		got[ti.Address] = ti.RenderedStatus
	}
	want := map[string]string{
		"proj:epic/01": "waiting-for-children",
		"proj:epic/02": "open",
		"proj:epic/03": "blocked",
		"proj:epic/04": "open",
	}
	for addr, w := range want {
		if got[addr] != w {
			t.Errorf("%s rendered status = %q, want %q", addr, got[addr], w)
		}
	}
}
