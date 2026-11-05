package rigs

import (
	"github.com/elentok/gx/server/servertest"
	"path/filepath"
	"testing"

	"github.com/elentok/gx/events"
	"github.com/elentok/gx/ralphloop/runlog"
	"github.com/elentok/gx/recovery"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/testutil"
)

// StartDeadlock starts a server over epic-a with ticket 02 blocked by 01.
func StartDeadlock(t *testing.T, cat recovery.Catalog) *servertest.Harness {
	t.Helper()
	store := t.TempDir()
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.WriteTicket(t, store, "proj", "epic-a", "02", "second", "01")
	servertest.SetProjectRepo(t, store, "proj", testutil.TempRepo(t))
	return servertest.StartWithStore(t, store, func(c *server.Config) {
		c.Recovery = cat
	})
}

// RunLogOf is the run log's epic-level events of typ. The claim loop may
// launch and pause 01 meanwhile; its own events are not the epic's.
func RunLogOf(t *testing.T, h *servertest.Harness, typ events.Type) []runlog.Event {
	t.Helper()
	log, _, err := runlog.ReadEvents(filepath.Join(h.TicketStore, "proj"), "epic-a")
	if err != nil {
		t.Fatal(err)
	}
	var got []runlog.Event
	for _, ev := range log {
		if events.Type(ev.Type) == typ && ev.Ticket == "" {
			got = append(got, ev)
		}
	}
	return got
}
