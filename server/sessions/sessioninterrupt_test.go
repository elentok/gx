package sessions_test

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/servertest/rigs"
	"github.com/elentok/gx/server/wire"

	"github.com/elentok/gx/testutil/agentfake"
)

func TestSessionInterrupt_SendsCtrlCToAWorkingSession(t *testing.T) {
	t.Parallel()
	f := rigs.StartPromptSession(t, rigs.FirstPromptGrace)
	report(t, f.Conv, agentfake.Control{State: "working"})
	waitForSession(f.Ctx, t, f.H, f.Evs, f.ID, func(s wire.SessionInfo) bool { return s.Status.State == wire.SessionWorking })
	res, err := f.H.Client.Sessions.Interrupt(f.Ctx, f.ID)
	if err != nil || res.Refused {
		t.Fatalf("interrupt = %+v, %v", res, err)
	}
	waitForSubmitted(t, f.Conv, strconv.Quote(agentfake.ProgramInterruptEntry)+"\n")
}

func TestSessionInterrupt_SendsOneEscToACodexSession(t *testing.T) {
	t.Parallel()
	f := rigs.StartCodexPromptSession(t)
	if res := f.Prompt(t, "work"); res.Refused {
		t.Fatalf("prompt = %+v", res)
	}
	waitForSession(f.Ctx, t, f.H, f.Evs, f.ID, func(s wire.SessionInfo) bool { return s.Status.State == wire.SessionWorking })
	res, err := f.H.Client.Sessions.Interrupt(f.Ctx, f.ID)
	if err != nil || res.Refused {
		t.Fatalf("interrupt = %+v, %v", res, err)
	}
	waitForSubmitted(t, f.Conv, "\"work\"\n"+strconv.Quote(agentfake.ProgramEscEntry)+"\n")
	time.Sleep(300 * time.Millisecond)
	if got := submitted(t, f.Conv); strings.Contains(got, agentfake.ProgramInterruptEntry) {
		t.Errorf("codex got a ctrl-c: %q", got)
	}
}

func TestSessionInterrupt_RefusesAnIdleSession(t *testing.T) {
	t.Parallel()
	f := rigs.StartPromptSession(t, rigs.FirstPromptGrace)
	res, err := f.H.Client.Sessions.Interrupt(f.Ctx, f.ID)
	if err != nil || !res.Refused || res.Reason != wire.ReasonSessionNotWorking {
		t.Errorf("interrupt while idle = %+v, %v; want %s", res, err, wire.ReasonSessionNotWorking)
	}
	if got := submitted(t, f.Conv); got != "" {
		t.Errorf("a refused interrupt sent keys: %q", got)
	}
}

func TestWebSessions_InterruptNeedsAFreshTokenOnAnAgentSession(t *testing.T) {
	t.Parallel()
	h, base, cookie, ctx, _, design, plain := startWebDesign(t)
	waitForHolder(t, h.SessionsDir+"/"+design)
	for _, tc := range []struct{ name, id, state, want string }{
		{"no token", design, "", wire.ReasonStale},
		{"plain session", plain, sessionToken(ctx, t, h, plain), wire.ReasonSessionNotWorking},
		{"fresh token, not working", design, sessionToken(ctx, t, h, design), wire.ReasonSessionNotWorking},
	} {
		code, body := webSessionWrite(t, base, cookie, "interrupt", tc.id, tc.state)
		if code != http.StatusOK || servertest.ReasonOf(t, body) != tc.want {
			t.Errorf("interrupt with %s = %d %s, want %s", tc.name, code, body, tc.want)
		}
	}
}

func TestWebSessions_InterruptsAWorkingChatAndDesignSession(t *testing.T) {
	t.Parallel()
	f := rigs.StartPromptSession(t, rigs.FirstPromptGrace)
	res, err := f.H.Client.Sessions.Create(f.Ctx, wire.CreateSessionRequest{Project: "proj", Dir: t.TempDir(), Harness: string(agentfake.HarnessKind)})
	if err != nil || res.Session == nil {
		t.Fatalf("create = %+v, %v", res, err)
	}
	chat := res.Session.ID
	chatConv, _ := readMeta(t, f.H, chat)["conversation_id"].(string)
	waitForHolder(t, f.H.SessionsDir+"/"+chat)
	for _, tc := range []struct{ name, id, conv string }{{"chat", chat, chatConv}, {"design", f.ID, f.Conv}} {
		report(t, tc.conv, agentfake.Control{State: "working"})
		waitForSession(f.Ctx, t, f.H, f.Evs, tc.id, func(s wire.SessionInfo) bool { return s.Status.State == wire.SessionWorking })
		if code, body := webSessionWrite(t, f.Base, f.Ck, "interrupt", tc.id, "stale"); code != http.StatusOK || servertest.ReasonOf(t, body) != wire.ReasonStale {
			t.Errorf("%s: stale token = %d %s, want %s", tc.name, code, body, wire.ReasonStale)
		}
		if got := submitted(t, tc.conv); got != "" {
			t.Errorf("%s: a refused interrupt sent keys: %q", tc.name, got)
		}
		code, body := webSessionWrite(t, f.Base, f.Ck, "interrupt", tc.id, sessionToken(f.Ctx, t, f.H, tc.id))
		if code != http.StatusOK || servertest.ReasonOf(t, body) != "" {
			t.Fatalf("%s: interrupt = %d %s", tc.name, code, body)
		}
		waitForSubmitted(t, tc.conv, strconv.Quote(agentfake.ProgramInterruptEntry)+"\n")
	}
}

func TestWebSessions_RefusesToInterruptAQueueAgent(t *testing.T) {
	t.Parallel()
	h, row, release := rigs.StartQueueChat(t, "hello", func(c *server.Config) { c.WebAddr = "127.0.0.1:0" })
	defer release()
	hs, err := h.Client.Handshake(context.Background())
	if err != nil || hs.WebURL == "" {
		t.Fatalf("handshake web_url = %q (err %v)", hs.WebURL, err)
	}
	ck := servertest.Login(t, h.StateDir, hs.WebURL)
	code, body := webSessionWrite(t, hs.WebURL, ck, "interrupt", row.ID, sessionToken(context.Background(), t, h, row.ID))
	// A Queue agent is not in the sessions dir, so the write routes answer unknown-session
	// before checkAgentWrite can say it is a Queue session.
	if code != http.StatusOK || servertest.ReasonOf(t, body) != wire.ReasonSessionUnknown {
		t.Errorf("interrupt of a Queue agent = %d %s, want %s", code, body, wire.ReasonSessionUnknown)
	}
}
