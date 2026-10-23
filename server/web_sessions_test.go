package server_test

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil/agentfake"
)

// startWebDesign starts a web mux with one design session and one plain one.
func startWebDesign(t *testing.T) (h *servertest.Harness, base string, cookie *http.Cookie, ctx context.Context, evs <-chan server.Event, design, plain string) {
	t.Helper()
	store := t.TempDir()
	servertest.WriteTicket(t, store, "proj", "map", "01", "first", "")
	servertest.WriteTicket(t, store, "proj", "plain", "01", "first", "")
	if err := os.WriteFile(filepath.Join(store, "proj", "map", "ticket.md"), []byte("---\nkind: map\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	h = servertest.StartWithStore(t, store, servertest.WithTestHolder(), func(c *server.Config) { c.WebAddr = "127.0.0.1:0" })
	hs, err := h.Client.Handshake(context.Background())
	if err != nil || hs.WebURL == "" {
		t.Fatalf("handshake web_url = %q (err %v)", hs.WebURL, err)
	}
	base, cookie = hs.WebURL, login(t, h.StateDir, hs.WebURL)
	var cancel context.CancelFunc
	ctx, cancel = context.WithTimeout(context.Background(), 20*time.Second)
	t.Cleanup(cancel)
	if evs, err = h.Client.Events(ctx, 0); err != nil {
		t.Fatal(err)
	}
	create := func(ticket string) string {
		res, err := h.Client.CreateSession(ctx, server.CreateSessionRequest{Project: "proj", Dir: t.TempDir(), Harness: string(agentfake.HarnessKind), Ticket: ticket})
		if err != nil || res.Session == nil {
			t.Fatalf("create = %+v, %v", res, err)
		}
		return res.Session.ID
	}
	return h, base, cookie, ctx, evs, create("proj:map"), create("proj:plain")
}

func sessionToken(ctx context.Context, t *testing.T, h *servertest.Harness, id string) string {
	t.Helper()
	snap, err := h.Client.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range snap.Sessions {
		if s.ID == id {
			return server.SessionStateToken(s)
		}
	}
	t.Fatalf("no session %s", id)
	return ""
}

func webSessionWrite(t *testing.T, base string, cookie *http.Cookie, verb, id, state string) (int, string) {
	t.Helper()
	return webPostJSON(t, base+"/v1/sessions/"+verb, fmt.Sprintf(`{"id":%q,"state":%q}`, id, state), cookie, "", jsonPost(base))
}

func TestWebSessions_StopAndRestoreNeedAFreshTokenOnADesignSession(t *testing.T) {
	h, base, cookie, ctx, evs, design, plain := startWebDesign(t)
	waitForHolder(t, filepath.Join(h.SessionsDir, design))
	waitForHolder(t, filepath.Join(h.SessionsDir, plain))
	fresh := sessionToken(ctx, t, h, design)

	for _, tc := range []struct{ name, id, state, want string }{
		{"no token", design, "", server.ReasonStale},
		{"old token", design, design + ":starting:1", server.ReasonStale},
		{"plain session", plain, sessionToken(ctx, t, h, plain), server.ReasonSessionNotDesign},
	} {
		code, body := webSessionWrite(t, base, cookie, "stop", tc.id, tc.state)
		if code != http.StatusOK || reasonOf(t, body) != tc.want {
			t.Errorf("stop with %s = %d %s, want %s", tc.name, code, body, tc.want)
		}
	}
	if code, body := webSessionWrite(t, base, cookie, "resume", plain, sessionToken(ctx, t, h, plain)); code != http.StatusOK || reasonOf(t, body) != server.ReasonSessionNotDesign {
		t.Errorf("restore of a plain session = %d %s, want not-design-session", code, body)
	}

	if code, body := webSessionWrite(t, base, cookie, "stop", design, fresh); code != http.StatusOK || reasonOf(t, body) != "" {
		t.Fatalf("stop with a fresh token = %d %s, want it to succeed", code, body)
	}
	waitForSession(ctx, t, h, evs, design, func(s server.SessionInfo) bool { return s.Status.State == server.SessionExited })

	if code, body := webSessionWrite(t, base, cookie, "resume", design, fresh); code != http.StatusOK || reasonOf(t, body) != server.ReasonStale {
		t.Errorf("restore with the pre-stop token = %d %s, want stale", code, body)
	}
	if code, body := webSessionWrite(t, base, cookie, "resume", design, sessionToken(ctx, t, h, design)); code != http.StatusOK || reasonOf(t, body) != "" {
		t.Errorf("restore with a fresh token = %d %s, want it to succeed", code, body)
	}
}

func TestWebSessions_WriteChecksStillApply(t *testing.T) {
	h, base, cookie, ctx, _, design, _ := startWebDesign(t)
	body := fmt.Sprintf(`{"id":%q,"state":%q}`, design, sessionToken(ctx, t, h, design))
	for _, path := range []string{"/v1/sessions/stop", "/v1/sessions/resume"} {
		if code, _ := webPostJSON(t, base+path, body, cookie, "", map[string]string{"Origin": "http://evil.example", "Content-Type": "application/json"}); code != http.StatusForbidden {
			t.Errorf("%s foreign Origin = %d, want 403", path, code)
		}
		if code, _ := webPostJSON(t, base+path, body, cookie, "", map[string]string{"Origin": base, "Content-Type": "text/plain"}); code != http.StatusForbidden {
			t.Errorf("%s text/plain = %d, want 403", path, code)
		}
		if code, _ := webPostJSON(t, base+path, body, nil, "", jsonPost(base)); code != http.StatusUnauthorized && code != http.StatusForbidden {
			t.Errorf("%s without cookie = %d, want 401/403", path, code)
		}
	}
}

func TestSessions_NonWebStopAndRestoreTakeNoToken(t *testing.T) {
	h, ctx, evs, id, _ := startFakeSession(t)
	waitForHolder(t, filepath.Join(h.SessionsDir, id))
	if res, err := h.Client.StopSession(ctx, id); err != nil || res.Refused {
		t.Fatalf("stop = %+v, %v", res, err)
	}
	waitForSession(ctx, t, h, evs, id, func(s server.SessionInfo) bool { return s.Status.State == server.SessionExited })
	if res, err := h.Client.ResumeSession(ctx, id); err != nil || res.Refused {
		t.Fatalf("restore = %+v, %v", res, err)
	}
}
