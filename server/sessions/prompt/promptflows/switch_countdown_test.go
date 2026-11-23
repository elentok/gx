package promptflows_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/elentok/gx/server/servertest/rigs"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil/agentfake"
)

func switchModel(t *testing.T, f *rigs.GrillingFixture, model string) wire.SessionResult {
	t.Helper()
	res, err := f.H.Client.Sessions.Switch(f.Ctx, wire.SessionSwitchRequest{ID: f.ID, Model: model})
	if err != nil {
		t.Fatalf("switch = %+v, %v", res, err)
	}
	return res
}

func TestSwitch_AnAcceptedSwitchStopsTheAutoAdvanceCountdownWithOneNotice(t *testing.T) {
	t.Parallel()
	f := startCountdown(t)

	if res := switchModel(t, f, "opus"); res.Refused || res.Session == nil || res.Session.SwitchingModel != "opus" {
		t.Fatalf("switch = %+v, want accepted", res)
	}
	time.Sleep(2 * countdownDelay) // negative wait: a countdown still running would end within this window
	got := rigs.SessionSnapshot(t, f)
	if got.AutoAdvanceAt != nil {
		t.Errorf("auto_advance_at = %v, want the countdown stopped", got.AutoAdvanceAt)
	}
	if n := rigs.AutoSends(t, f); n != 0 {
		t.Errorf("auto sends = %d, want no Continue", n)
	}
	if b := f.H.Banners.Sent(); len(b) != 1 {
		t.Errorf("banners = %+v, want one notice", b)
	}
}

func TestSwitch_AnAcceptedSwitchStopsTheAutoQueueCountdown(t *testing.T) {
	t.Parallel()
	f, _ := startReadyMap(t, mapCase{step: "branch", implStatus: "open", base: "other"})
	setAutoQueue(t, f, true)
	waitForHolder(t, filepath.Join(f.H.SessionsDir, f.ID))
	report(t, f.Conv, agentfake.Control{State: "idle"})
	waitForSession(f.Ctx, t, f.H, f.Evs, f.ID, func(s wire.SessionInfo) bool { return s.Status.State == wire.SessionIdle && s.AutoQueueAt != nil })

	if res := switchModel(t, f, "opus"); res.Refused {
		t.Fatalf("switch = %+v, want accepted", res)
	}
	time.Sleep(2 * rigs.CountdownDelay) // negative wait: a countdown still running would end within this window
	if got := rigs.SessionSnapshot(t, f); got.AutoQueueAt != nil {
		t.Errorf("auto_queue_at = %v, want the countdown stopped", got.AutoQueueAt)
	}
	if queued(f) {
		t.Error("epic queued by a cancelled countdown")
	}
}

func TestSwitch_ARefusedSwitchLeavesTheCountdownRunning(t *testing.T) {
	t.Parallel()
	f := startCountdown(t)

	if res := switchModel(t, f, ""); !res.Refused {
		t.Fatalf("switch without a model = %+v, want refused", res)
	}
	if got := rigs.SessionSnapshot(t, f); got.AutoAdvanceAt == nil {
		t.Errorf("session = %+v, want the countdown still running", got)
	}
	if b := f.H.Banners.Sent(); len(b) != 0 {
		t.Errorf("banners = %+v, want none", b)
	}
}
