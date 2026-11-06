package runs

import (
	"os"
	"strings"
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

func TestRunRegistry_LaunchStep(t *testing.T) {
	t.Parallel()
	const addr = "proj:epic-a/01"
	r, _, err := Open(t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if r.SetLaunchStep(addr, StepInstallingDeps) {
		t.Error("SetLaunchStep on a missing run reported a change")
	}
	if _, ok := r.Tracked(addr); ok {
		t.Fatal("SetLaunchStep created a run")
	}

	r.Put(TrackedRun{Run: Run{Address: addr}})
	if !r.SetLaunchStep(addr, StepInstallingDeps) || r.SetLaunchStep(addr, StepInstallingDeps) {
		t.Error("SetLaunchStep must report a change once, not for the same step again")
	}
	r.Put(TrackedRun{Run: Run{Address: addr}, State: Launching})
	if got, _ := r.Tracked(addr); got.LaunchStep != StepInstallingDeps {
		t.Errorf("step after Put = %q, want it kept", got.LaunchStep)
	}
	data, err := os.ReadFile(r.path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "launch_step") || strings.Contains(string(data), string(StepInstallingDeps)) {
		t.Errorf("runs.json saved the step: %s", data)
	}
	r.SetLaunchStep(addr, "")
	if got, _ := r.Tracked(addr); got.LaunchStep != "" {
		t.Errorf("step after clear = %q", got.LaunchStep)
	}
}

func TestRunPauseEvent_RecordsLaunchModelAndEffort(t *testing.T) {
	t.Parallel()
	ev := Run{Agent: "claude", Model: "opus", Effort: "high"}.PauseEvent()
	if ev.Model != "opus" || ev.Effort != "high" {
		t.Fatalf("pauseEvent model/effort = %q/%q, want opus/high", ev.Model, ev.Effort)
	}
}
