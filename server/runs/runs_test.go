package runs

import (
	"testing"
	"time"
)

func TestRunRegistry_WaitGoneTrueOnceRemovedFalseAtTimeout(t *testing.T) {
	t.Parallel()
	r, _ := waitingRegistry(t, time.Minute)
	const addr = "proj:epic-a/01"
	if r.WaitGone(addr, 50*time.Millisecond, 5*time.Millisecond) {
		t.Error("waitGone = true with the run still tracked, want false at timeout")
	}
	removal := time.AfterFunc(30*time.Millisecond, func() { r.Delete(addr) })
	defer removal.Stop()
	if !r.WaitGone(addr, 5*time.Second, 5*time.Millisecond) {
		t.Error("waitGone = false after the run left, want true")
	}
	if !r.WaitGone("proj:epic-a/99", time.Millisecond, time.Millisecond) {
		t.Error("waitGone = false for an address never tracked, want true")
	}
}

func TestRunPauseEvent_RecordsLaunchModelAndEffort(t *testing.T) {
	t.Parallel()
	ev := Run{Agent: "claude", Model: "opus", Effort: "high"}.PauseEvent()
	if ev.Model != "opus" || ev.Effort != "high" {
		t.Fatalf("pauseEvent model/effort = %q/%q, want opus/high", ev.Model, ev.Effort)
	}
}
