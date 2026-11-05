package servertest

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/sessions"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil/agentfake"
)

// StartWebDesign starts a web mux with one design session and one plain one.
func StartWebDesign(t *testing.T) (h *Harness, base string, cookie *http.Cookie, ctx context.Context, evs <-chan wire.Event, design, plain string) {
	t.Helper()
	store := t.TempDir()
	WriteTicket(t, store, "proj", "map", "01", "first", "")
	WriteTicket(t, store, "proj", "plain", "01", "first", "")
	if err := os.WriteFile(filepath.Join(store, "proj", "map", "ticket.md"), []byte("---\nkind: map\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	h = StartWithStore(t, store, WithTestHolder(), func(c *server.Config) { c.WebAddr = "127.0.0.1:0" })
	hs, err := h.Client.Handshake(context.Background())
	if err != nil || hs.WebURL == "" {
		t.Fatalf("handshake web_url = %q (err %v)", hs.WebURL, err)
	}
	base, cookie = hs.WebURL, Login(t, h.StateDir, hs.WebURL)
	var cancel context.CancelFunc
	ctx, cancel = context.WithTimeout(context.Background(), 20*time.Second)
	t.Cleanup(cancel)
	if evs, err = h.Client.Events(ctx, 0); err != nil {
		t.Fatal(err)
	}
	create := func(ticket string) string {
		res, err := h.Client.Sessions.Create(ctx, wire.CreateSessionRequest{Project: "proj", Dir: t.TempDir(), Harness: string(agentfake.HarnessKind), Ticket: ticket})
		if err != nil || res.Session == nil {
			t.Fatalf("create = %+v, %v", res, err)
		}
		return res.Session.ID
	}
	return h, base, cookie, ctx, evs, create("proj:map"), create("proj:plain")
}

// SessionToken is the state token of session id in the current snapshot.
func SessionToken(ctx context.Context, t *testing.T, h *Harness, id string) string {
	t.Helper()
	snap, err := h.Client.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range snap.Sessions {
		if s.ID == id {
			return sessions.SessionStateToken(s)
		}
	}
	t.Fatalf("no session %s", id)
	return ""
}

// WebSessionWrite posts a session write verb with a state token.
func WebSessionWrite(t *testing.T, base string, cookie *http.Cookie, verb, id, state string) (int, string) {
	t.Helper()
	return WebPostJSON(t, base+"/v1/sessions/"+verb, fmt.Sprintf(`{"id":%q,"state":%q}`, id, state), cookie, "", JSONPost(base))
}
