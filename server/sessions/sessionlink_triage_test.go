package sessions_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil/agentfake"
)

func TestLinkSession_TriageSetsFlowAndKey(t *testing.T) {
	t.Parallel()
	store := t.TempDir()
	servertest.WriteTicket(t, store, "proj", "map", "01", "first", "")
	if err := os.WriteFile(filepath.Join(store, "proj", "map", "ticket.md"), []byte("---\nkind: map\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	h := servertest.StartWithStore(t, store, servertest.WithTestHolder())
	ctx := context.Background()
	newSession := func(ticket string) string {
		t.Helper()
		created, err := h.Client.Sessions.Create(ctx, wire.CreateSessionRequest{Project: "proj", Dir: t.TempDir(), Harness: string(agentfake.HarnessKind), Ticket: ticket})
		if err != nil || created.Session == nil {
			t.Fatalf("create = %+v, %v", created, err)
		}
		return created.Session.ID
	}

	t.Run("plain session", func(t *testing.T) {
		res, err := h.Client.Sessions.Link(ctx, wire.LinkSessionRequest{ID: newSession(""), Flow: "triage", FlowKey: "proj"})
		if err != nil || res.Refused || res.Session == nil {
			t.Fatalf("link = %+v, %v", res, err)
		}
		if s := res.Session; s.Flow != "triage" || s.FlowKey != "proj" || s.Ticket != "" || s.Design {
			t.Errorf("session = %+v; want flow triage, key proj (unqualified), no ticket, not design", s)
		}
	})

	t.Run("unknown project", func(t *testing.T) {
		res, err := h.Client.Sessions.Link(ctx, wire.LinkSessionRequest{ID: newSession(""), Flow: "triage", FlowKey: "nope"})
		if err != nil || !res.Refused || res.Reason != wire.ReasonSessionBadRequest {
			t.Errorf("link = %+v, %v; want refused %s", res, err, wire.ReasonSessionBadRequest)
		}
	})

	t.Run("design session", func(t *testing.T) {
		res, err := h.Client.Sessions.Link(ctx, wire.LinkSessionRequest{ID: newSession("proj:map"), Flow: "triage", FlowKey: "proj"})
		if err != nil || !res.Refused {
			t.Errorf("link = %+v, %v; want refused on a Design session", res, err)
		}
	})
}
