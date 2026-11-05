package server_test

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/elentok/gx/server/wire"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/sessions"
	"github.com/elentok/gx/testutil/agentfake"
)

// startWebDesign starts a web mux with one design session and one plain one.
func startWebDesign(t *testing.T) (h *servertest.Harness, base string, cookie *http.Cookie, ctx context.Context, evs <-chan wire.Event, design, plain string) {
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
	base, cookie = hs.WebURL, servertest.Login(t, h.StateDir, hs.WebURL)
	var cancel context.CancelFunc
	ctx, cancel = context.WithTimeout(context.Background(), 20*time.Second)
	t.Cleanup(cancel)
	if evs, err = h.Client.Events(ctx, 0); err != nil {
		t.Fatal(err)
	}
	create := func(ticket string) string {
		res, err := h.Client.CreateSession(ctx, wire.CreateSessionRequest{Project: "proj", Dir: t.TempDir(), Harness: string(agentfake.HarnessKind), Ticket: ticket})
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
			return sessions.SessionStateToken(s)
		}
	}
	t.Fatalf("no session %s", id)
	return ""
}

func webSessionWrite(t *testing.T, base string, cookie *http.Cookie, verb, id, state string) (int, string) {
	t.Helper()
	return servertest.WebPostJSON(t, base+"/v1/sessions/"+verb, fmt.Sprintf(`{"id":%q,"state":%q}`, id, state), cookie, "", servertest.JSONPost(base))
}

func TestWebSessions_StopAndRestoreNeedAFreshTokenOnADesignSession(t *testing.T) {
	t.Parallel()
	h, base, cookie, ctx, evs, design, plain := startWebDesign(t)
	waitForHolder(t, filepath.Join(h.SessionsDir, design))
	waitForHolder(t, filepath.Join(h.SessionsDir, plain))
	fresh := sessionToken(ctx, t, h, design)

	for _, tc := range []struct{ name, id, state, want string }{
		{"no token", design, "", wire.ReasonStale},
		{"old token", design, design + ":starting:1", wire.ReasonStale},
	} {
		code, body := webSessionWrite(t, base, cookie, "stop", tc.id, tc.state)
		if code != http.StatusOK || servertest.ReasonOf(t, body) != tc.want {
			t.Errorf("stop with %s = %d %s, want %s", tc.name, code, body, tc.want)
		}
	}
	if code, body := webSessionWrite(t, base, cookie, "resume", plain, sessionToken(ctx, t, h, plain)); code != http.StatusOK || servertest.ReasonOf(t, body) != wire.ReasonSessionNotDesign {
		t.Errorf("restore of a plain session = %d %s, want not-design-session", code, body)
	}

	if code, body := webSessionWrite(t, base, cookie, "stop", design, fresh); code != http.StatusOK || servertest.ReasonOf(t, body) != "" {
		t.Fatalf("stop with a fresh token = %d %s, want it to succeed", code, body)
	}
	waitForSession(ctx, t, h, evs, design, func(s wire.SessionInfo) bool { return s.Status.State == wire.SessionExited })

	if code, body := webSessionWrite(t, base, cookie, "resume", design, fresh); code != http.StatusOK || servertest.ReasonOf(t, body) != wire.ReasonStale {
		t.Errorf("restore with the pre-stop token = %d %s, want stale", code, body)
	}
	if code, body := webSessionWrite(t, base, cookie, "resume", design, sessionToken(ctx, t, h, design)); code != http.StatusOK || servertest.ReasonOf(t, body) != "" {
		t.Errorf("restore with a fresh token = %d %s, want it to succeed", code, body)
	}
}

func TestWebSessions_StopThenRemoveAPlainSession(t *testing.T) {
	t.Parallel()
	h, base, cookie, ctx, evs, _, plain := startWebDesign(t)
	waitForHolder(t, filepath.Join(h.SessionsDir, plain))

	live := sessionToken(ctx, t, h, plain)
	if code, body := webSessionWrite(t, base, cookie, "remove", plain, live); code != http.StatusOK || servertest.ReasonOf(t, body) != wire.ReasonSessionLive {
		t.Fatalf("remove of a live session = %d %s, want session-live", code, body)
	}
	if code, body := webSessionWrite(t, base, cookie, "stop", plain, live); code != http.StatusOK || servertest.ReasonOf(t, body) != "" {
		t.Fatalf("stop of a plain session = %d %s, want it to succeed", code, body)
	}
	waitForSession(ctx, t, h, evs, plain, func(s wire.SessionInfo) bool { return s.Status.State == wire.SessionExited })

	if code, body := webSessionWrite(t, base, cookie, "remove", plain, live); code != http.StatusOK || servertest.ReasonOf(t, body) != wire.ReasonStale {
		t.Errorf("remove with the pre-stop token = %d %s, want stale", code, body)
	}
	if code, body := webSessionWrite(t, base, cookie, "remove", plain, sessionToken(ctx, t, h, plain)); code != http.StatusOK || servertest.ReasonOf(t, body) != "" {
		t.Fatalf("remove with a fresh token = %d %s, want it to succeed", code, body)
	}
	snap, err := h.Client.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range snap.Sessions {
		if s.ID == plain {
			t.Errorf("session %s is still listed after remove", plain)
		}
	}
}

func TestWebSessions_WriteChecksStillApply(t *testing.T) {
	t.Parallel()
	h, base, cookie, ctx, _, design, _ := startWebDesign(t)
	body := fmt.Sprintf(`{"id":%q,"state":%q}`, design, sessionToken(ctx, t, h, design))
	for _, path := range []string{"/v1/sessions/stop", "/v1/sessions/resume"} {
		if code, _ := servertest.WebPostJSON(t, base+path, body, cookie, "", map[string]string{"Origin": "http://evil.example", "Content-Type": "application/json"}); code != http.StatusForbidden {
			t.Errorf("%s foreign Origin = %d, want 403", path, code)
		}
		if code, _ := servertest.WebPostJSON(t, base+path, body, cookie, "", map[string]string{"Origin": base, "Content-Type": "text/plain"}); code != http.StatusForbidden {
			t.Errorf("%s text/plain = %d, want 403", path, code)
		}
		if code, _ := servertest.WebPostJSON(t, base+path, body, nil, "", servertest.JSONPost(base)); code != http.StatusUnauthorized && code != http.StatusForbidden {
			t.Errorf("%s without cookie = %d, want 401/403", path, code)
		}
	}
}

func TestSessions_NonWebStopAndRestoreTakeNoToken(t *testing.T) {
	t.Parallel()
	h, ctx, evs, id, _ := startFakeSession(t)
	waitForHolder(t, filepath.Join(h.SessionsDir, id))
	if res, err := h.Client.StopSession(ctx, id); err != nil || res.Refused {
		t.Fatalf("stop = %+v, %v", res, err)
	}
	waitForSession(ctx, t, h, evs, id, func(s wire.SessionInfo) bool { return s.Status.State == wire.SessionExited })
	if res, err := h.Client.ResumeSession(ctx, id); err != nil || res.Refused {
		t.Fatalf("restore = %+v, %v", res, err)
	}
}
