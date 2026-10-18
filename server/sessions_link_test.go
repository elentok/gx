package server_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil/agentfake"
)

func TestLinkSession_MarksMapTicketsDesign(t *testing.T) {
	store := t.TempDir()
	servertest.WriteTicket(t, store, "proj", "map", "01", "first", "")
	servertest.WriteTicket(t, store, "proj", "plain", "01", "first", "")
	if err := os.WriteFile(filepath.Join(store, "proj", "map", "ticket.md"), []byte("---\nkind: map\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	h := servertest.StartWithStore(t, store, servertest.WithTestHolder())
	ctx := context.Background()
	created, err := h.Client.CreateSession(ctx, server.CreateSessionRequest{Project: "proj", Dir: t.TempDir(), Harness: string(agentfake.HarnessKind), Ticket: "proj:map"})
	if err != nil || created.Session == nil || !created.Session.Design {
		t.Fatalf("create on a map = %+v, %v; want Design", created.Session, err)
	}
	id := created.Session.ID

	for _, tc := range []struct {
		ticket, want string
		design       bool
	}{
		{"plain/01", "proj:plain/01", false},
		{"proj:map/01", "proj:map/01", true},
		{"proj:missing", "proj:missing", false},
	} {
		res, err := h.Client.LinkSession(ctx, server.LinkSessionRequest{ID: id, Ticket: tc.ticket})
		if err != nil || res.Refused || res.Session.Ticket != tc.want || res.Session.Design != tc.design {
			t.Errorf("link %s = %+v, %v; want ticket %s, design %v", tc.ticket, res.Session, err, tc.want, tc.design)
		}
	}
}

func TestLinkSession_Refusals(t *testing.T) {
	h := servertest.StartWithStore(t, t.TempDir(), servertest.WithTestHolder())
	ctx := context.Background()
	for name, tc := range map[string]struct {
		req    server.LinkSessionRequest
		reason string
	}{
		"no ticket":       {server.LinkSessionRequest{ID: "abcdef"}, server.ReasonSessionBadRequest},
		"unknown session": {server.LinkSessionRequest{ID: "abcdef", Ticket: "x"}, server.ReasonSessionUnknown},
	} {
		res, err := h.Client.LinkSession(ctx, tc.req)
		if err != nil || !res.Refused || res.Reason != tc.reason {
			t.Errorf("%s: link = %+v, %v; want refused %s", name, res, err, tc.reason)
		}
	}
}
