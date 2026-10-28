package server_test

import (
	"net/http"
	"testing"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/testutil/agentfake"
)

func TestSessionAutoAdvance_FlipsTheSwitchOnADesignSession(t *testing.T) {
	t.Parallel()
	f := startPromptSession(t)
	snapshotSwitch := func() bool {
		snap, err := f.h.Client.Snapshot(f.ctx)
		if err != nil {
			t.Fatal(err)
		}
		for _, s := range snap.Sessions {
			if s.ID == f.id {
				return s.AutoAdvance
			}
		}
		t.Fatalf("no session %s", f.id)
		return false
	}
	if !snapshotSwitch() {
		t.Error("a new session has auto-advance off")
	}
	if _, ok := readMeta(t, f.h, f.id)["auto_advance_off"]; ok {
		t.Error("a new session's meta records the switch; an old one has no such field")
	}
	for _, on := range []bool{false, true} {
		res, err := f.h.Client.SetAutoAdvance(f.ctx, f.id, on)
		if err != nil || res.Refused || res.Session == nil || res.Session.AutoAdvance != on {
			t.Fatalf("set auto-advance %v = %+v, %v", on, res, err)
		}
		if got := snapshotSwitch(); got != on {
			t.Errorf("snapshot auto-advance = %v, want %v", got, on)
		}
	}
}

func TestSessionAutoAdvance_RefusesAPlainAndAnUnknownSession(t *testing.T) {
	t.Parallel()
	f := startPromptSession(t)
	plain, err := f.h.Client.CreateSession(f.ctx, server.CreateSessionRequest{Project: "proj", Dir: t.TempDir(), Harness: string(agentfake.HarnessKind)})
	if err != nil || plain.Session == nil {
		t.Fatalf("create = %+v, %v", plain, err)
	}
	for _, tc := range []struct{ name, id, want string }{
		{"plain", plain.Session.ID, server.ReasonSessionNotDesign},
		{"unknown", "nope", server.ReasonSessionUnknown},
	} {
		res, err := f.h.Client.SetAutoAdvance(f.ctx, tc.id, false)
		if err != nil || !res.Refused || res.Reason != tc.want {
			t.Errorf("%s session = %+v, %v; want %s", tc.name, res, err, tc.want)
		}
	}
}

func TestWebSessions_AutoAdvanceNeedsAFreshTokenOnADesignSession(t *testing.T) {
	t.Parallel()
	h, base, cookie, ctx, _, design, plain := startWebDesign(t)
	waitForHolder(t, h.SessionsDir+"/"+design)
	for _, tc := range []struct{ name, id, state, want string }{
		{"no token", design, "", server.ReasonStale},
		{"plain session", plain, sessionToken(ctx, t, h, plain), server.ReasonSessionNotDesign},
	} {
		code, body := webSessionWrite(t, base, cookie, "auto-advance", tc.id, tc.state)
		if code != http.StatusOK || reasonOf(t, body) != tc.want {
			t.Errorf("auto-advance with %s = %d %s, want %s", tc.name, code, body, tc.want)
		}
	}
	code, body := webSessionWrite(t, base, cookie, "auto-advance", design, sessionToken(ctx, t, h, design))
	if code != http.StatusOK || reasonOf(t, body) != "" {
		t.Errorf("auto-advance with a fresh token = %d %s, want accepted", code, body)
	}
}
