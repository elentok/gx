package prompt_test

import (
	"net/http"
	"testing"

	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/servertest/rigs"
	"github.com/elentok/gx/server/wire"

	"github.com/elentok/gx/testutil/agentfake"
)

func TestSessionAutoQueue_FlipsTheSwitchAndKeepsItAcrossARestart(t *testing.T) {
	t.Parallel()
	f := rigs.StartPromptSession(t, rigs.FirstPromptGrace)
	snapshotSwitch := func() wire.SessionInfo {
		snap, err := f.H.Client.Snapshot(f.Ctx)
		if err != nil {
			t.Fatal(err)
		}
		for _, s := range snap.Sessions {
			if s.ID == f.ID {
				return s
			}
		}
		t.Fatalf("no session %s", f.ID)
		return wire.SessionInfo{}
	}
	if snapshotSwitch().AutoQueue {
		t.Error("a new session has auto-queue on")
	}
	for _, on := range []bool{true, false, true} {
		res, err := f.H.Client.Sessions.SetAutoQueue(f.Ctx, f.ID, on)
		if err != nil || res.Refused || res.Session == nil || res.Session.AutoQueue != on {
			t.Fatalf("set auto-queue %v = %+v, %v", on, res, err)
		}
		if got := snapshotSwitch(); got.AutoQueue != on || got.AutoQueueAt != nil {
			t.Errorf("snapshot auto-queue = %v at %v, want %v with no deadline", got.AutoQueue, got.AutoQueueAt, on)
		}
	}
	f.H.Restart(t)
	if !snapshotSwitch().AutoQueue {
		t.Error("auto-queue is off after a restart")
	}
}

func TestSessionAutoQueue_RefusesAPlainAndAnUnknownSession(t *testing.T) {
	t.Parallel()
	f := rigs.StartPromptSession(t, rigs.FirstPromptGrace)
	plain, err := f.H.Client.Sessions.Create(f.Ctx, wire.CreateSessionRequest{Project: "proj", Dir: t.TempDir(), Harness: string(agentfake.HarnessKind)})
	if err != nil || plain.Session == nil {
		t.Fatalf("create = %+v, %v", plain, err)
	}
	for _, tc := range []struct{ name, id, want string }{
		{"plain", plain.Session.ID, wire.ReasonSessionNotDesign},
		{"unknown", "nope", wire.ReasonSessionUnknown},
	} {
		res, err := f.H.Client.Sessions.SetAutoQueue(f.Ctx, tc.id, true)
		if err != nil || !res.Refused || res.Reason != tc.want {
			t.Errorf("%s session = %+v, %v; want %s", tc.name, res, err, tc.want)
		}
	}
}

func TestWebSessions_AutoQueueNeedsAFreshTokenOnADesignSession(t *testing.T) {
	t.Parallel()
	h, base, cookie, ctx, _, design, plain := startWebDesign(t)
	waitForHolder(t, h.SessionsDir+"/"+design)
	for _, tc := range []struct{ name, id, state, want string }{
		{"no token", design, "", wire.ReasonStale},
		{"plain session", plain, sessionToken(ctx, t, h, plain), wire.ReasonSessionNotDesign},
	} {
		code, body := webSessionWrite(t, base, cookie, "auto-queue", tc.id, tc.state)
		if code != http.StatusOK || servertest.ReasonOf(t, body) != tc.want {
			t.Errorf("auto-queue with %s = %d %s, want %s", tc.name, code, body, tc.want)
		}
	}
	code, body := webSessionWrite(t, base, cookie, "auto-queue", design, sessionToken(ctx, t, h, design))
	if code != http.StatusOK || servertest.ReasonOf(t, body) != "" {
		t.Errorf("auto-queue with a fresh token = %d %s, want accepted", code, body)
	}
}
