package sessions_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/elentok/gx/server/queue"
	"github.com/elentok/gx/server/sessions"
	"github.com/elentok/gx/server/wire"

	"github.com/elentok/gx/server/servertest"
)

func TestWebSessions_StopAndRestoreNeedAFreshTokenOnADesignSession(t *testing.T) {
	t.Parallel()
	h, base, cookie, ctx, evs, design, plain := startWebDesign(t)
	waitForHolder(t, filepath.Join(h.SessionsDir, design))
	waitForHolder(t, filepath.Join(h.SessionsDir, plain))
	fresh := sessionToken(ctx, t, h, design)

	for _, tc := range []struct{ name, id, state, want string }{
		{"no token", design, "", wire.ReasonStale},
		{"old token", design, fresh, wire.ReasonStale},
	} {
		code, res := webTargetWrite(t, base, cookie, "stop", tc.id, tc.state, false)
		if code != http.StatusOK || res.Reason != tc.want {
			t.Errorf("stop with %s = %d %+v, want %s", tc.name, code, res, tc.want)
		}
	}
	if code, body := webSessionWrite(t, base, cookie, "resume", plain, sessionToken(ctx, t, h, plain)); code != http.StatusOK || servertest.ReasonOf(t, body) != wire.ReasonSessionNotDesign {
		t.Errorf("restore of a plain session = %d %s, want not-design-session", code, body)
	}

	if code, res := webTargetWrite(t, base, cookie, "stop", design, rowToken(ctx, t, h, design), false); code != http.StatusOK || res.Refused() {
		t.Fatalf("stop with a fresh token = %d %+v, want it to succeed", code, res)
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

	live := rowToken(ctx, t, h, plain)
	if code, res := webTargetWrite(t, base, cookie, "remove", plain, live, false); code != http.StatusOK || res.Reason != wire.ReasonNeedsYes {
		t.Fatalf("remove of a live session = %d %+v, want needs-yes", code, res)
	}
	if code, res := webTargetWrite(t, base, cookie, "stop", plain, live, false); code != http.StatusOK || res.Refused() {
		t.Fatalf("stop of a plain session = %d %+v, want it to succeed", code, res)
	}
	waitForSession(ctx, t, h, evs, plain, func(s wire.SessionInfo) bool { return s.Status.State == wire.SessionExited })

	if code, res := webTargetWrite(t, base, cookie, "remove", plain, live, false); code != http.StatusOK || res.Reason != wire.ReasonStale || res.Message != wire.StaleRowMessage {
		t.Errorf("remove with the pre-stop token = %d %+v, want stale %q", code, res, wire.StaleRowMessage)
	}
	exited := rowToken(ctx, t, h, plain)
	if code, res := webTargetWrite(t, base, cookie, "stop", plain, exited, false); code != http.StatusOK || res.Reason != wire.ReasonSessionNotLive {
		t.Errorf("stop of an Exited session = %d %+v, want session-not-live", code, res)
	}
	if code, res := webTargetWrite(t, base, cookie, "remove", plain, exited, false); code != http.StatusOK || res.Refused() {
		t.Fatalf("remove with a fresh token = %d %+v, want it to succeed", code, res)
	}
	assertNotListed(ctx, t, h, plain)
}

func assertNotListed(ctx context.Context, t *testing.T, h *servertest.Harness, id string) {
	t.Helper()
	snap, err := h.Client.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range snap.Sessions {
		if s.ID == id {
			t.Errorf("session %s is still listed after remove", id)
		}
	}
}

func TestWebSessions_RemoveWithYesStopsAndRemovesALiveSession(t *testing.T) {
	t.Parallel()
	h, base, cookie, ctx, _, _, plain := startWebDesign(t)
	waitForHolder(t, filepath.Join(h.SessionsDir, plain))

	if code, res := webTargetWrite(t, base, cookie, "remove", plain, rowToken(ctx, t, h, plain), true); code != http.StatusOK || res.Refused() {
		t.Fatalf("remove with yes = %d %+v, want it to succeed", code, res)
	}
	assertNotListed(ctx, t, h, plain)
}

func TestWebSessions_QueueSessionIsRefusedOnTheWebRouteOnly(t *testing.T) {
	t.Parallel()
	h, base, cookie, ctx, _, _, _ := startWebDesign(t)
	id := queue.AgentIDPrefix + "proj-map-impl-01"
	for _, verb := range []string{"stop", "remove"} {
		if code, res := webTargetWrite(t, base, cookie, verb, id, "", false); code != http.StatusOK || res.Reason != wire.ReasonSessionQueue {
			t.Errorf("web %s of a Queue session = %d %+v, want queue-session", verb, code, res)
		}
	}
	res, err := h.Client.Sessions.StopMany(ctx, []string{id})
	if err != nil || len(res.Results) != 1 || res.Results[0].Reason == wire.ReasonSessionQueue {
		t.Errorf("plain stop of a Queue session = %+v, %v, want no queue-session refusal", res, err)
	}
}

func TestSessionRowToken(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 1, 2, 3, 4, 5, 6, time.UTC)
	row := func(state string, run time.Time) wire.SessionInfo {
		return wire.SessionInfo{ID: "s1", RunStartedAt: run, Status: wire.SessionStatus{State: state}}
	}
	working := sessions.SessionRowToken(row(wire.SessionWorking, at))
	if idle := sessions.SessionRowToken(row(wire.SessionIdle, at)); idle != working {
		t.Errorf("Idle token %q differs from Working token %q", idle, working)
	}
	for name, other := range map[string]wire.SessionInfo{
		"exited":      row(wire.SessionExited, at),
		"restored":    row(wire.SessionWorking, at.Add(time.Second)),
		"another row": {ID: "s2", RunStartedAt: at, Status: wire.SessionStatus{State: wire.SessionWorking}},
	} {
		if sessions.SessionRowToken(other) == working {
			t.Errorf("%s keeps the token %q", name, working)
		}
	}
	if old := sessions.SessionStateToken(row(wire.SessionWorking, at)); old == working {
		t.Errorf("the old token format %q equals the row token", old)
	}
}

// The web's rowToken embeds run_started_at as the raw JSON string, so it
// matches SessionRowToken only while the server's wire time keeps RFC3339Nano.
// webui/src/design/rowToken.test.ts reads the same fixture.
func TestSessionRowToken_MatchesTheSharedWebFixture(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile(filepath.Join("..", "wire", "testdata", "row_tokens.json"))
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Name         string `json:"name"`
		ID           string `json:"id"`
		RunStartedAt string `json:"run_started_at"`
		State        string `json:"state"`
		Token        string `json:"token"`
	}
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			var info wire.SessionInfo
			body := fmt.Sprintf(`{"id":%q,"run_started_at":%q,"status":{"state":%q}}`, tc.ID, tc.RunStartedAt, tc.State)
			if err := json.Unmarshal([]byte(body), &info); err != nil {
				t.Fatal(err)
			}
			if got := sessions.SessionRowToken(info); got != tc.Token {
				t.Errorf("SessionRowToken = %q, want %q", got, tc.Token)
			}
			out, err := json.Marshal(info.RunStartedAt)
			if err != nil || string(out) != fmt.Sprintf("%q", tc.RunStartedAt) {
				t.Errorf("wire time marshals to %s, %v, want %q", out, err, tc.RunStartedAt)
			}
		})
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
	if res, err := h.Client.Sessions.Stop(ctx, id); err != nil || res.Refused {
		t.Fatalf("stop = %+v, %v", res, err)
	}
	waitForSession(ctx, t, h, evs, id, func(s wire.SessionInfo) bool { return s.Status.State == wire.SessionExited })
	if res, err := h.Client.Sessions.Resume(ctx, id); err != nil || res.Refused {
		t.Fatalf("restore = %+v, %v", res, err)
	}
}
