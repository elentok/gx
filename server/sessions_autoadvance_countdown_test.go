package server_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil/agentfake"
)

const (
	runEndText     = "Done.\n✅ Map: drafted. Next: start a fresh session"
	countdownDelay = 500 * time.Millisecond
)

// endTurn runs a working-then-idle turn on the session.
func endTurn(t *testing.T, f *grillingFixture) {
	t.Helper()
	report(t, f.conv, agentfake.Control{State: "working"})
	waitForSession(f.ctx, t, f.h, f.evs, f.id, func(s server.SessionInfo) bool { return s.Status.State == server.SessionWorking })
	report(t, f.conv, agentfake.Control{State: "idle"})
}

type grillingFixture struct {
	h    *servertest.Harness
	ctx  context.Context
	evs  <-chan server.Event
	id   string
	conv string
	path string
}

func startAdvancing(t *testing.T, store string) *grillingFixture {
	t.Helper()
	h, ctx, evs, id, conv, path := startGrillingSessionIn(t, "proj:map", store, servertest.WithAutoAdvanceDelay(countdownDelay))
	return &grillingFixture{h, ctx, evs, id, conv, path}
}

func sessionSnapshot(t *testing.T, f *grillingFixture) server.SessionInfo {
	t.Helper()
	snap, err := f.h.Client.Sessions(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range snap {
		if s.ID == f.id {
			return s
		}
	}
	t.Fatalf("session %s not in the snapshot", f.id)
	return server.SessionInfo{}
}

func TestAutoAdvance_RunEndSendsContinueAfterTheCountdown(t *testing.T) {
	for name, text := range map[string]string{
		"run-end line":       runEndText,
		"error then run-end": "Error: the command failed\n" + runEndText,
	} {
		t.Run(name, func(t *testing.T) {
			f := startAdvancing(t, storeWithMapStep(t, "spec"))
			c := waitForHolder(t, filepath.Join(f.h.SessionsDir, f.id))
			writeAssistantMessage(t, f.path, text)
			endTurn(t, f)
			got := waitForSession(f.ctx, t, f.h, f.evs, f.id, func(s server.SessionInfo) bool { return s.AutoAdvanceAt != nil })
			if !got.AutoAdvanceAt.After(time.Now().Add(-time.Second)) || got.Actionable.Reason == "" {
				t.Errorf("session = %+v, want a running countdown over a raised Continue", got)
			}
			if b, s := f.h.Banners.Sent(), f.h.Sounds.Played(); len(b) != 0 || len(s) != 0 {
				t.Errorf("banners = %v, sounds = %v at countdown start, want none", b, s)
			}
			waitForScreen(t, c, "/clear")
			waitForScreen(t, c, "/gx-design map")
			waitForSession(f.ctx, t, f.h, f.evs, f.id, func(s server.SessionInfo) bool { return s.AutoAdvanceAt == nil && s.Actionable.Reason == "" })
		})
	}
}

func TestAutoAdvance_EndingsThatNeverAdvance(t *testing.T) {
	for name, tc := range map[string]struct {
		step, text string
	}{
		"grilling":   {step: "spec", text: "❓ **Q1** Which store?\n" + runEndText},
		"branch":     {step: "branch", text: runEndText},
		"done":       {step: "done", text: runEndText},
		"no run-end": {step: "spec", text: "Interrupted mid-step."},
	} {
		t.Run(name, func(t *testing.T) {
			f := startAdvancing(t, storeWithMapStep(t, tc.step))
			c := waitForHolder(t, filepath.Join(f.h.SessionsDir, f.id))
			writeAssistantMessage(t, f.path, tc.text)
			endTurn(t, f)
			waitForSession(f.ctx, t, f.h, f.evs, f.id, func(s server.SessionInfo) bool { return s.Status.State == server.SessionIdle })
			time.Sleep(2 * countdownDelay)
			if screenHas(t, c, "/clear") {
				t.Error("an ending that never advances sent /clear")
			}
			if got := sessionSnapshot(t, f); got.AutoAdvanceAt != nil {
				t.Errorf("countdown = %v, want none", got.AutoAdvanceAt)
			}
		})
	}
}

func TestAutoAdvance_SwitchOffStartsNoCountdown(t *testing.T) {
	f := startAdvancing(t, storeWithMapStep(t, "spec"))
	c := waitForHolder(t, filepath.Join(f.h.SessionsDir, f.id))
	if res, err := f.h.Client.SetAutoAdvance(f.ctx, f.id, false); err != nil || res.Refused {
		t.Fatalf("switch off = %+v, %v", res, err)
	}
	writeAssistantMessage(t, f.path, runEndText)
	endTurn(t, f)
	got := waitForSession(f.ctx, t, f.h, f.evs, f.id, func(s server.SessionInfo) bool { return s.Actionable.Reason != "" })
	time.Sleep(2 * countdownDelay)
	if got.AutoAdvanceAt != nil || screenHas(t, c, "/clear") {
		t.Errorf("session = %+v, want no countdown and nothing sent", got)
	}
}

func TestAutoAdvance_RestartDropsTheCountdownAndNeverStartsOne(t *testing.T) {
	f := startAdvancing(t, storeWithMapStep(t, "spec"))
	c := waitForHolder(t, filepath.Join(f.h.SessionsDir, f.id))
	writeAssistantMessage(t, f.path, runEndText)
	endTurn(t, f)
	waitForSession(f.ctx, t, f.h, f.evs, f.id, func(s server.SessionInfo) bool { return s.AutoAdvanceAt != nil })

	f.h.Restart(t)
	time.Sleep(2 * countdownDelay)
	if s := sessionSnapshot(t, f); s.AutoAdvanceAt != nil || s.Actionable.Reason == "" || len(s.Actionable.Action) == 0 {
		t.Errorf("session after restart = %+v, want no countdown and Continue raised", s)
	}
	if screenHas(t, c, "/clear") {
		t.Error("a restart during the countdown still sent /clear")
	}
}
