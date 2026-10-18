package server

import "testing"

func TestRunParkEvent_RecordsLaunchModelAndEffort(t *testing.T) {
	ev := Run{Agent: "claude", Model: "opus", Effort: "high"}.parkEvent()
	if ev.Model != "opus" || ev.Effort != "high" {
		t.Fatalf("parkEvent model/effort = %q/%q, want opus/high", ev.Model, ev.Effort)
	}
}
