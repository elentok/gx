package server_test

import (
	"net/http"
	"strconv"
	"testing"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/testutil/agentfake"
)

func TestSessionInterrupt_SendsCtrlCToAWorkingSession(t *testing.T) {
	f := startPromptSession(t)
	report(t, f.conv, agentfake.Control{State: "working"})
	waitForSession(f.ctx, t, f.h, f.evs, f.id, func(s server.SessionInfo) bool { return s.Status.State == server.SessionWorking })
	res, err := f.h.Client.InterruptSession(f.ctx, f.id)
	if err != nil || res.Refused {
		t.Fatalf("interrupt = %+v, %v", res, err)
	}
	waitForSubmitted(t, f.conv, strconv.Quote(agentfake.ProgramInterruptEntry)+"\n")
}

func TestSessionInterrupt_RefusesAnIdleSession(t *testing.T) {
	f := startPromptSession(t)
	res, err := f.h.Client.InterruptSession(f.ctx, f.id)
	if err != nil || !res.Refused || res.Reason != server.ReasonSessionNotWorking {
		t.Errorf("interrupt while idle = %+v, %v; want %s", res, err, server.ReasonSessionNotWorking)
	}
	if got := submitted(t, f.conv); got != "" {
		t.Errorf("a refused interrupt sent keys: %q", got)
	}
}

func TestWebSessions_InterruptNeedsAFreshTokenOnADesignSession(t *testing.T) {
	h, base, cookie, ctx, _, design, plain := startWebDesign(t)
	waitForHolder(t, h.SessionsDir+"/"+design)
	for _, tc := range []struct{ name, id, state, want string }{
		{"no token", design, "", server.ReasonStale},
		{"plain session", plain, sessionToken(ctx, t, h, plain), server.ReasonSessionNotDesign},
		{"fresh token, not working", design, sessionToken(ctx, t, h, design), server.ReasonSessionNotWorking},
	} {
		code, body := webSessionWrite(t, base, cookie, "interrupt", tc.id, tc.state)
		if code != http.StatusOK || reasonOf(t, body) != tc.want {
			t.Errorf("interrupt with %s = %d %s, want %s", tc.name, code, body, tc.want)
		}
	}
}
