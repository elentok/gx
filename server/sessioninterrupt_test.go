package server_test

import (
	"net/http"
	"strconv"
	"testing"

	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire"

	"github.com/elentok/gx/testutil/agentfake"
)

func TestSessionInterrupt_SendsCtrlCToAWorkingSession(t *testing.T) {
	t.Parallel()
	f := startPromptSession(t)
	report(t, f.conv, agentfake.Control{State: "working"})
	waitForSession(f.ctx, t, f.h, f.evs, f.id, func(s wire.SessionInfo) bool { return s.Status.State == wire.SessionWorking })
	res, err := f.h.Client.InterruptSession(f.ctx, f.id)
	if err != nil || res.Refused {
		t.Fatalf("interrupt = %+v, %v", res, err)
	}
	waitForSubmitted(t, f.conv, strconv.Quote(agentfake.ProgramInterruptEntry)+"\n")
}

func TestSessionInterrupt_RefusesAnIdleSession(t *testing.T) {
	t.Parallel()
	f := startPromptSession(t)
	res, err := f.h.Client.InterruptSession(f.ctx, f.id)
	if err != nil || !res.Refused || res.Reason != wire.ReasonSessionNotWorking {
		t.Errorf("interrupt while idle = %+v, %v; want %s", res, err, wire.ReasonSessionNotWorking)
	}
	if got := submitted(t, f.conv); got != "" {
		t.Errorf("a refused interrupt sent keys: %q", got)
	}
}

func TestWebSessions_InterruptNeedsAFreshTokenOnADesignSession(t *testing.T) {
	t.Parallel()
	h, base, cookie, ctx, _, design, plain := startWebDesign(t)
	waitForHolder(t, h.SessionsDir+"/"+design)
	for _, tc := range []struct{ name, id, state, want string }{
		{"no token", design, "", wire.ReasonStale},
		{"plain session", plain, sessionToken(ctx, t, h, plain), wire.ReasonSessionNotDesign},
		{"fresh token, not working", design, sessionToken(ctx, t, h, design), wire.ReasonSessionNotWorking},
	} {
		code, body := webSessionWrite(t, base, cookie, "interrupt", tc.id, tc.state)
		if code != http.StatusOK || servertest.ReasonOf(t, body) != tc.want {
			t.Errorf("interrupt with %s = %d %s, want %s", tc.name, code, body, tc.want)
		}
	}
}
