package server

import "testing"

func TestRunPauseEvent_RecordsLaunchModelAndEffort(t *testing.T) {
	t.Parallel()
	ev := Run{Agent: "claude", Model: "opus", Effort: "high"}.pauseEvent()
	if ev.Model != "opus" || ev.Effort != "high" {
		t.Fatalf("pauseEvent model/effort = %q/%q, want opus/high", ev.Model, ev.Effort)
	}
}
