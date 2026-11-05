package prompt_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/elentok/gx/server/wire"

	"github.com/elentok/gx/nativerunner/holder"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil/agentfake"
)

func TestAutoAdvance_RunEndSendsContinueAfterTheCountdown(t *testing.T) {
	t.Parallel()
	for name, text := range map[string]string{
		"run-end line":       runEndText,
		"error then run-end": "Error: the command failed\n" + runEndText,
	} {
		t.Run(name, func(t *testing.T) {
			f := startAdvancing(t, storeWithMapStep(t, "spec"))
			c := waitForHolder(t, filepath.Join(f.H.SessionsDir, f.ID))
			writeAssistantMessage(t, f.Path, text)
			endTurn(t, f)
			got := waitForSession(f.Ctx, t, f.H, f.Evs, f.ID, func(s wire.SessionInfo) bool { return s.AutoAdvanceAt != nil })
			if !got.AutoAdvanceAt.After(time.Now().Add(-time.Second)) || got.Actionable.Reason == "" {
				t.Errorf("session = %+v, want a running countdown over a raised Continue", got)
			}
			if b, s := f.H.Banners.Sent(), f.H.Sounds.Played(); len(b) != 0 || len(s) != 0 {
				t.Errorf("banners = %v, sounds = %v at countdown start, want none", b, s)
			}
			waitForScreen(t, c, "/clear")
			waitForScreen(t, c, "/gx-design map")
			waitForSession(f.Ctx, t, f.H, f.Evs, f.ID, func(s wire.SessionInfo) bool { return s.AutoAdvanceAt == nil && s.Actionable.Reason == "" })
		})
	}
}

func TestAutoAdvance_RunEndWithBackgroundTaskSendsContinue(t *testing.T) {
	t.Parallel()
	f := startAdvancing(t, storeWithMapStep(t, "spec"))
	c := waitForHolder(t, filepath.Join(f.H.SessionsDir, f.ID))
	report(t, f.Conv, agentfake.Control{State: "working"})
	waitForSession(f.Ctx, t, f.H, f.Evs, f.ID, func(s wire.SessionInfo) bool {
		return s.Status.State == wire.SessionWorking && !s.Status.Background
	})
	writeAssistantMessage(t, f.Path, runEndText)
	report(t, f.Conv, agentfake.Control{State: "background"})
	waitForSession(f.Ctx, t, f.H, f.Evs, f.ID, func(s wire.SessionInfo) bool {
		return s.Status.Background && s.AutoAdvanceAt != nil
	})
	waitForScreen(t, c, "/clear")
	waitForScreen(t, c, "/gx-design map")
}

func TestAutoAdvance_EndingsThatNeverAdvance(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		step, text string
		// stopped: a finished design's session is stopped, so its holder is gone.
		stopped bool
	}{
		"grilling":   {step: "spec", text: "❓ **Q1** Which store?\n" + runEndText},
		"branch":     {step: "branch", text: runEndText},
		"done":       {step: "done", text: runEndText, stopped: true},
		"no run-end": {step: "spec", text: "Interrupted mid-step."},
	} {
		t.Run(name, func(t *testing.T) {
			if tc.stopped {
				// The server stops a design only on a rescan that sees the step move to
				// done while the session is live. Seeding done would race the first
				// rescan, which can run before the session exists and then never again.
				f := startAdvancing(t, storeWithMapStep(t, "spec"))
				mapMD := filepath.Join(f.H.TicketStore, "proj", "map", "ticket.md")
				if err := os.WriteFile(mapMD, []byte("---\nkind: map\ndesign_step: "+tc.step+"\n---\n"), 0o644); err != nil {
					t.Fatal(err)
				}
				if err := f.H.Client.Tickets.Changed(f.Ctx, "proj:map"); err != nil {
					t.Fatal(err)
				}
				waitForSession(f.Ctx, t, f.H, f.Evs, f.ID, func(s wire.SessionInfo) bool { return s.Status.State == wire.SessionExited })
				if got := sessionSnapshot(t, f); got.AutoAdvanceAt != nil {
					t.Errorf("countdown = %v, want none", got.AutoAdvanceAt)
				}
				return
			}
			f := startAdvancing(t, storeWithMapStep(t, tc.step))
			c := waitForHolder(t, filepath.Join(f.H.SessionsDir, f.ID))
			writeAssistantMessage(t, f.Path, tc.text)
			endTurn(t, f)
			waitForSession(f.Ctx, t, f.H, f.Evs, f.ID, func(s wire.SessionInfo) bool { return s.Status.State == wire.SessionIdle })
			time.Sleep(2 * countdownDelay) // negative wait: a countdown would start and end within this window
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
	t.Parallel()
	f := startAdvancing(t, storeWithMapStep(t, "spec"))
	c := waitForHolder(t, filepath.Join(f.H.SessionsDir, f.ID))
	if res, err := f.H.Client.Sessions.SetAutoAdvance(f.Ctx, f.ID, false); err != nil || res.Refused {
		t.Fatalf("switch off = %+v, %v", res, err)
	}
	writeAssistantMessage(t, f.Path, runEndText)
	endTurn(t, f)
	got := waitForSession(f.Ctx, t, f.H, f.Evs, f.ID, func(s wire.SessionInfo) bool { return s.Actionable.Reason != "" })
	time.Sleep(2 * countdownDelay) // negative wait: a countdown would start and end within this window
	if got.AutoAdvanceAt != nil || screenHas(t, c, "/clear") {
		t.Errorf("session = %+v, want no countdown and nothing sent", got)
	}
}

// startCountdown ends a run on a session so its countdown runs over a raised Continue.
func startCountdown(t *testing.T) (*grillingFixture, *holder.HolderClient) {
	t.Helper()
	f := startAdvancing(t, storeWithMapStep(t, "spec"))
	c := waitForHolder(t, filepath.Join(f.H.SessionsDir, f.ID))
	writeAssistantMessage(t, f.Path, runEndText)
	endTurn(t, f)
	waitForSession(f.Ctx, t, f.H, f.Evs, f.ID, func(s wire.SessionInfo) bool { return s.AutoAdvanceAt != nil })
	return f, c
}

// wantStopped checks that nothing was sent, the run-end Continue still stands
// and exactly one banner told the person.
func wantStopped(t *testing.T, f *grillingFixture, c *holder.HolderClient) {
	t.Helper()
	time.Sleep(2 * countdownDelay) // negative wait: a countdown still running would end within this window
	if screenHas(t, c, "/clear") {
		t.Error("a stopped countdown still sent /clear")
	}
	got := sessionSnapshot(t, f)
	if got.AutoAdvanceAt != nil || got.Actionable.Reason == "" || len(got.Actionable.Action) == 0 {
		t.Errorf("session = %+v, want no countdown and Continue raised", got)
	}
	if b, s := f.H.Banners.Sent(), f.H.Sounds.Played(); len(b) != 1 || len(s) != 1 {
		t.Errorf("banners = %v, sounds = %v, want one of each", b, s)
	}
}

func TestAutoAdvance_CancelStopsTheSend(t *testing.T) {
	t.Parallel()
	f, c := startCountdown(t)
	if res, err := f.H.Client.Sessions.CancelAutoAdvance(f.Ctx, f.ID); err != nil || res.Refused {
		t.Fatalf("cancel = %+v, %v", res, err)
	}
	wantStopped(t, f, c)
}

func TestAutoAdvance_PaneInputStopsTheSendAndKeepsContinue(t *testing.T) {
	t.Parallel()
	f, c := startCountdown(t)
	clearByInput(t, f.H, f.ID)
	wantStopped(t, f, c)
}

func TestAutoAdvance_PageNoteStopsTheSendAndKeepsContinue(t *testing.T) {
	t.Parallel()
	f, c := startCountdown(t)
	if res, err := f.H.Client.Sessions.Prompt(f.Ctx, wire.SessionPromptRequest{ID: f.ID, Text: "a note"}); err != nil || res.Refused {
		t.Fatalf("prompt = %+v, %v", res, err)
	}
	wantStopped(t, f, c)
}

func TestAutoAdvance_ManualContinueStopsTheCountdown(t *testing.T) {
	t.Parallel()
	f, _ := startCountdown(t)
	if res, err := f.H.Client.Sessions.Prompt(f.Ctx, wire.SessionPromptRequest{ID: f.ID, Continue: true}); err != nil || res.Refused {
		t.Fatalf("continue = %+v, %v", res, err)
	}
	if got := sessionSnapshot(t, f); got.AutoAdvanceAt != nil {
		t.Errorf("countdown = %v after a manual Continue, want none", got.AutoAdvanceAt)
	}
}

func TestAutoAdvance_RunActionStopsTheCountdown(t *testing.T) {
	t.Parallel()
	f, _ := startCountdown(t)
	if res, err := f.H.Client.Sessions.RunAction(f.Ctx, f.ID); err != nil || res.Refused {
		t.Fatalf("run action = %+v, %v", res, err)
	}
	if got := sessionSnapshot(t, f); got.AutoAdvanceAt != nil {
		t.Errorf("countdown = %v after Run Action, want none", got.AutoAdvanceAt)
	}
}

func TestAutoAdvance_SwitchOffDuringTheCountdownStopsTheSend(t *testing.T) {
	t.Parallel()
	f, c := startCountdown(t)
	if res, err := f.H.Client.Sessions.SetAutoAdvance(f.Ctx, f.ID, false); err != nil || res.Refused {
		t.Fatalf("switch off = %+v, %v", res, err)
	}
	wantStopped(t, f, c)
}

func TestAutoAdvance_RunEndingWithTheSwitchOffPlaysOneBanner(t *testing.T) {
	t.Parallel()
	f := startAdvancing(t, storeWithMapStep(t, "spec"))
	if res, err := f.H.Client.Sessions.SetAutoAdvance(f.Ctx, f.ID, false); err != nil || res.Refused {
		t.Fatalf("switch off = %+v, %v", res, err)
	}
	writeAssistantMessage(t, f.Path, runEndText)
	endTurn(t, f)
	waitForSession(f.Ctx, t, f.H, f.Evs, f.ID, func(s wire.SessionInfo) bool { return s.Actionable.Reason != "" })
	servertest.WaitFor(t, 5*time.Second, "the banner", func() bool { return len(f.H.Banners.Sent()) > 0 })
	time.Sleep(countdownDelay) // negative wait: a countdown would end within this window and add a second banner
	if b := f.H.Banners.Sent(); len(b) != 1 {
		t.Errorf("banners = %v, want one", b)
	}
}

func TestAutoAdvance_TurnSeenOnlyMidFlightStartsNoCountdown(t *testing.T) {
	t.Parallel()
	f := startAdvancing(t, storeWithMapStep(t, "spec"))
	c := waitForHolder(t, filepath.Join(f.H.SessionsDir, f.ID))
	writeAssistantMessage(t, f.Path, runEndText)
	report(t, f.Conv, agentfake.Control{State: "working"})
	waitForSession(f.Ctx, t, f.H, f.Evs, f.ID, func(s wire.SessionInfo) bool { return s.Status.State == wire.SessionWorking })

	f.H.Restart(t)
	report(t, f.Conv, agentfake.Control{State: "idle"})
	servertest.WaitFor(t, 10*time.Second, "the session to go idle after the restart", func() bool {
		return sessionSnapshot(t, f).Status.State == wire.SessionIdle
	})
	time.Sleep(2 * countdownDelay) // negative wait: a countdown would start and end within this window
	if s := sessionSnapshot(t, f); s.AutoAdvanceAt != nil {
		t.Errorf("countdown = %v for a turn first seen mid-flight, want none", s.AutoAdvanceAt)
	}
	if screenHas(t, c, "/clear") {
		t.Error("a turn first seen mid-flight still sent /clear")
	}
}

func TestAutoAdvance_RestartDropsTheCountdownAndNeverStartsOne(t *testing.T) {
	t.Parallel()
	f := startAdvancing(t, storeWithMapStep(t, "spec"))
	c := waitForHolder(t, filepath.Join(f.H.SessionsDir, f.ID))
	writeAssistantMessage(t, f.Path, runEndText)
	endTurn(t, f)
	waitForSession(f.Ctx, t, f.H, f.Evs, f.ID, func(s wire.SessionInfo) bool { return s.AutoAdvanceAt != nil })

	f.H.Restart(t)
	time.Sleep(2 * countdownDelay) // negative wait: a revived countdown would end within this window
	if s := sessionSnapshot(t, f); s.AutoAdvanceAt != nil || s.Actionable.Reason == "" || len(s.Actionable.Action) == 0 {
		t.Errorf("session after restart = %+v, want no countdown and Continue raised", s)
	}
	if screenHas(t, c, "/clear") {
		t.Error("a restart during the countdown still sent /clear")
	}
}
