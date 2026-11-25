package web_test

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire"
)

// A blocker change on an epic whose tickets are all done flips no ticket
// status, so only the epic-meta key makes the server publish for it.
func TestEpicBlocker_WebWriteEmitsTicketChanged(t *testing.T) {
	t.Parallel()
	store := t.TempDir()
	servertest.WriteTicketWith(t, store, "proj", "target", "01", "x", servertest.TicketOpts{Status: "done", DoneAt: "2026-01-01T00:00:00Z"})
	servertest.WriteTicketWith(t, store, "proj", "dep", "01", "x", servertest.TicketOpts{})
	for _, epic := range []string{"target", "dep"} {
		if err := os.WriteFile(filepath.Join(store, "proj", epic, "ticket.md"), []byte("---\ntype: epic\n---\n# "+epic+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	h := servertest.StartWithStore(t, store, func(c *server.Config) { c.WebAddr = "127.0.0.1:0" })
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	hs, err := h.Client.Handshake(ctx)
	if err != nil || hs.WebURL == "" {
		t.Fatalf("handshake %+v, %v", hs, err)
	}
	snap, err := h.Client.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	events, err := h.Client.Events(ctx, snap.Seq)
	if err != nil {
		t.Fatal(err)
	}

	base := hs.WebURL
	code, body := servertest.WebPostJSON(t, base+"/v1/epics/blocker", `{"address":"proj:target","blocker":"proj:dep"}`,
		servertest.Login(t, h.StateDir, servertest.WebBase(base)), "", servertest.JSONPost(base))
	if code != http.StatusOK || servertest.ReasonOf(t, body) != "" {
		t.Fatalf("blocker = %d %s, want accepted", code, body)
	}
	for {
		select {
		case ev := <-events:
			if ev.Type == wire.EventTicketChanged && ev.Address == "proj:target" {
				return
			}
		case <-ctx.Done():
			t.Fatal("no ticket-changed for proj:target after the blocker write")
		}
	}
}

func TestEpicBlocker_WebAndCLIAgree(t *testing.T) {
	t.Parallel()
	store := t.TempDir()
	servertest.WriteTicketWith(t, store, "proj", "target", "01", "x", servertest.TicketOpts{})
	servertest.WriteTicketWith(t, store, "proj", "dep", "01", "x", servertest.TicketOpts{})
	for _, epic := range []string{"target", "dep"} {
		if err := os.WriteFile(filepath.Join(store, "proj", epic, "ticket.md"), []byte("---\ntype: epic\n---\n# "+epic+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	h := servertest.StartWithStore(t, store, func(c *server.Config) { c.WebAddr = "127.0.0.1:0" })
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	res, err := h.Client.Tickets.EpicBlocker(ctx, "proj:target", "proj:dep", false)
	if err != nil || res.Refused || len(res.BlockedBy) != 1 || res.BlockedBy[0] != "dep" {
		t.Fatalf("add = %+v, %v", res, err)
	}
	b, err := os.ReadFile(filepath.Join(store, "proj", "target", "ticket.md"))
	if err != nil || !strings.Contains(string(b), "dep") {
		t.Errorf("ticket.md = %q, %v", b, err)
	}
	res, err = h.Client.Tickets.EpicBlocker(ctx, "proj:dep", "proj:target", false)
	if err != nil || !res.Refused || res.Reason != wire.ReasonBlockedByCycle {
		t.Errorf("cycle = %+v, %v", res, err)
	}
}
