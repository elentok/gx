package server

import (
	"testing"
	"time"
)

func TestRunRegistry_WaitGoneTrueOnceRemovedFalseAtTimeout(t *testing.T) {
	t.Parallel()
	r, _ := waitingRegistry(t, time.Minute)
	const addr = "proj:epic-a/01"
	if r.waitGone(addr, 50*time.Millisecond, 5*time.Millisecond) {
		t.Error("waitGone = true with the run still tracked, want false at timeout")
	}
	go func() {
		time.Sleep(30 * time.Millisecond)
		r.delete(addr)
	}()
	if !r.waitGone(addr, 5*time.Second, 5*time.Millisecond) {
		t.Error("waitGone = false after the run left, want true")
	}
	if !r.waitGone("proj:epic-a/99", time.Millisecond, time.Millisecond) {
		t.Error("waitGone = false for an address never tracked, want true")
	}
}

func TestRunPauseEvent_RecordsLaunchModelAndEffort(t *testing.T) {
	t.Parallel()
	ev := Run{Agent: "claude", Model: "opus", Effort: "high"}.pauseEvent()
	if ev.Model != "opus" || ev.Effort != "high" {
		t.Fatalf("pauseEvent model/effort = %q/%q, want opus/high", ev.Model, ev.Effort)
	}
}
