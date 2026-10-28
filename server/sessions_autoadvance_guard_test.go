package server_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/elentok/gx/server"
)

func autoSends(t *testing.T, f *grillingFixture) int {
	t.Helper()
	n, _ := readMeta(t, f.h, f.id)["auto_sends"].(float64)
	return int(n)
}

// autoSendOnce ends a turn on the run-end line and waits for the countdown to
// send Continue.
func autoSendOnce(t *testing.T, f *grillingFixture) {
	t.Helper()
	endTurn(t, f)
	waitForSession(f.ctx, t, f.h, f.evs, f.id, func(s server.SessionInfo) bool { return s.AutoAdvanceAt != nil })
	waitForSession(f.ctx, t, f.h, f.evs, f.id, func(s server.SessionInfo) bool { return s.AutoAdvanceAt == nil && s.Actionable.Reason == "" })
}

func TestAutoAdvance_StopsAfterFiveSendsWithNoInput(t *testing.T) {
	t.Parallel()
	f := startAdvancing(t, storeWithMapStep(t, "spec"))
	waitForHolder(t, filepath.Join(f.h.SessionsDir, f.id))
	writeAssistantMessage(t, f.path, runEndText)
	for range 5 {
		autoSendOnce(t, f)
	}
	endTurn(t, f)
	waitUntil(t, "the guard's banner", func() bool { return len(f.h.Banners.Sent()) > 0 })
	time.Sleep(2 * countdownDelay) // negative wait: a countdown would start and end within this window

	got := sessionSnapshot(t, f)
	if got.AutoAdvanceAt != nil || got.Actionable.Reason == "" || len(got.Actionable.Action) == 0 {
		t.Errorf("session = %+v, want no countdown and Continue raised", got)
	}
	if n := autoSends(t, f); n != 5 {
		t.Errorf("auto sends = %d, want 5", n)
	}
	if b, s := f.h.Banners.Sent(), f.h.Sounds.Played(); len(b) != 1 || len(s) != 1 {
		t.Errorf("banners = %v, sounds = %v, want one of each", b, s)
	}
}

func TestAutoAdvance_InputAndSwitchFlipResetTheCount(t *testing.T) {
	t.Parallel()
	for name, input := range map[string]func(t *testing.T, f *grillingFixture){
		"pane input":  func(t *testing.T, f *grillingFixture) { clearByInput(t, f.h, f.id) },
		"switch flip": func(t *testing.T, f *grillingFixture) { setSwitch(t, f, false) },
	} {
		t.Run(name, func(t *testing.T) {
			f := startAdvancing(t, storeWithMapStep(t, "spec"))
			waitForHolder(t, filepath.Join(f.h.SessionsDir, f.id))
			writeAssistantMessage(t, f.path, runEndText)
			autoSendOnce(t, f)
			endTurn(t, f)
			waitForSession(f.ctx, t, f.h, f.evs, f.id, func(s server.SessionInfo) bool { return s.AutoAdvanceAt != nil })
			if n := autoSends(t, f); n != 1 {
				t.Fatalf("auto sends = %d before the input, want 1", n)
			}
			input(t, f)
			waitUntil(t, "the count reset", func() bool { return autoSends(t, f) == 0 })
		})
	}
}

func setSwitch(t *testing.T, f *grillingFixture, on bool) {
	t.Helper()
	if res, err := f.h.Client.SetAutoAdvance(f.ctx, f.id, on); err != nil || res.Refused {
		t.Fatalf("switch = %+v, %v", res, err)
	}
}

func TestAutoAdvance_SwitchingOnOverARaisedContinueStartsTheCountdown(t *testing.T) {
	t.Parallel()
	f := startAdvancing(t, storeWithMapStep(t, "spec"))
	c := waitForHolder(t, filepath.Join(f.h.SessionsDir, f.id))
	setSwitch(t, f, false)
	writeAssistantMessage(t, f.path, runEndText)
	endTurn(t, f)
	waitForSession(f.ctx, t, f.h, f.evs, f.id, func(s server.SessionInfo) bool { return s.Actionable.Reason != "" })
	time.Sleep(2 * countdownDelay) // negative wait: a countdown would start and end within this window
	if screenHas(t, c, "/clear") || sessionSnapshot(t, f).AutoAdvanceAt != nil {
		t.Fatal("a countdown ran with the switch off")
	}

	setSwitch(t, f, true)
	waitForSession(f.ctx, t, f.h, f.evs, f.id, func(s server.SessionInfo) bool { return s.AutoAdvanceAt != nil })
	waitForScreen(t, c, "/clear")
	waitForScreen(t, c, "/gx-design map")
}
