package sched

import (
	"testing"

	"github.com/elentok/gx/server/broker"
	"github.com/elentok/gx/server/runs"
	"github.com/elentok/gx/server/wire"
)

func TestSetLaunchStep_PublishesOneTicketChangedPerChange(t *testing.T) {
	t.Parallel()
	const addr = "proj:epic-a/01"
	reg, _, err := runs.Open(t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	b := broker.New(0)
	evs, cancel, err := b.Subscribe(b.CurrentSeq())
	if err != nil {
		t.Fatal(err)
	}
	defer cancel()
	l := NewLauncher(LauncherDeps{Registry: reg, Events: b})

	l.setLaunchStep(addr, runs.StepInstallingDeps) // no run yet
	reg.Put(runs.TrackedRun{Run: runs.Run{Address: addr}, LaunchStep: runs.StepCreatingWorktree})
	steps := []runs.LaunchStep{runs.StepInstallingDeps, runs.StepStartingAgent, runs.StepStartingAgent, runs.StepSendingPrompt}
	for _, s := range steps {
		l.setLaunchStep(addr, s)
	}
	reg.SetLaunchStep(addr, "")

	// Four setter calls, one of them a repeat: three changes, three events.
	for range 3 {
		if ev := <-evs; ev.Type != wire.EventTicketChanged || ev.Address != addr {
			t.Fatalf("event = %+v, want ticket-changed for %s", ev, addr)
		}
	}
	select {
	case ev := <-evs:
		t.Fatalf("extra event %+v for a missing run or a repeated step", ev)
	default:
	}
}
