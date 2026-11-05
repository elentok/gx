package prompt_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/elentok/gx/server/wire"
)

// autoSendOnce ends a turn on the run-end line and waits for the countdown to
// send Continue.
func autoSendOnce(t *testing.T, f *grillingFixture) {
	t.Helper()
	endTurn(t, f)
	waitForSession(f.Ctx, t, f.H, f.Evs, f.ID, func(s wire.SessionInfo) bool { return s.AutoAdvanceAt != nil })
	waitForSession(f.Ctx, t, f.H, f.Evs, f.ID, func(s wire.SessionInfo) bool { return s.AutoAdvanceAt == nil && s.Actionable.Reason == "" })
}

func TestAutoAdvance_StopsAfterFiveSendsWithNoInput(t *testing.T) {
	t.Parallel()
	f := startAdvancing(t, storeWithMapStep(t, "spec"))
	waitForHolder(t, filepath.Join(f.H.SessionsDir, f.ID))
	writeAssistantMessage(t, f.Path, runEndText)
	for range 5 {
		autoSendOnce(t, f)
	}
	endTurn(t, f)
	waitUntil(t, "the guard's banner", func() bool { return len(f.H.Banners.Sent()) > 0 })
	time.Sleep(2 * countdownDelay) // negative wait: a countdown would start and end within this window

	got := sessionSnapshot(t, f)
	if got.AutoAdvanceAt != nil || got.Actionable.Reason == "" || len(got.Actionable.Action) == 0 {
		t.Errorf("session = %+v, want no countdown and Continue raised", got)
	}
	if n := autoSends(t, f); n != 5 {
		t.Errorf("auto sends = %d, want 5", n)
	}
	if b, s := f.H.Banners.Sent(), f.H.Sounds.Played(); len(b) != 1 || len(s) != 1 {
		t.Errorf("banners = %v, sounds = %v, want one of each", b, s)
	}
}

func TestAutoAdvance_InputAndSwitchFlipResetTheCount(t *testing.T) {
	t.Parallel()
	for name, input := range map[string]func(t *testing.T, f *grillingFixture){
		"pane input":  func(t *testing.T, f *grillingFixture) { clearByInput(t, f.H, f.ID) },
		"switch flip": func(t *testing.T, f *grillingFixture) { setSwitch(t, f, false) },
	} {
		t.Run(name, func(t *testing.T) {
			f := startAdvancing(t, storeWithMapStep(t, "spec"))
			waitForHolder(t, filepath.Join(f.H.SessionsDir, f.ID))
			writeAssistantMessage(t, f.Path, runEndText)
			autoSendOnce(t, f)
			endTurn(t, f)
			waitForSession(f.Ctx, t, f.H, f.Evs, f.ID, func(s wire.SessionInfo) bool { return s.AutoAdvanceAt != nil })
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
	if res, err := f.H.Client.SetAutoAdvance(f.Ctx, f.ID, on); err != nil || res.Refused {
		t.Fatalf("switch = %+v, %v", res, err)
	}
}

func TestAutoAdvance_SwitchingOnOverARaisedContinueStartsTheCountdown(t *testing.T) {
	t.Parallel()
	f := startAdvancing(t, storeWithMapStep(t, "spec"))
	c := waitForHolder(t, filepath.Join(f.H.SessionsDir, f.ID))
	setSwitch(t, f, false)
	writeAssistantMessage(t, f.Path, runEndText)
	endTurn(t, f)
	waitForSession(f.Ctx, t, f.H, f.Evs, f.ID, func(s wire.SessionInfo) bool { return s.Actionable.Reason != "" })
	time.Sleep(2 * countdownDelay) // negative wait: a countdown would start and end within this window
	if screenHas(t, c, "/clear") || sessionSnapshot(t, f).AutoAdvanceAt != nil {
		t.Fatal("a countdown ran with the switch off")
	}

	setSwitch(t, f, true)
	waitForSession(f.Ctx, t, f.H, f.Evs, f.ID, func(s wire.SessionInfo) bool { return s.AutoAdvanceAt != nil })
	waitForScreen(t, c, "/clear")
	waitForScreen(t, c, "/gx-design map")
}
