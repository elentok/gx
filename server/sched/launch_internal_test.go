package sched

import (
	"slices"
	"testing"
	"time"

	"github.com/elentok/gx/ralphloop/agent"
	"github.com/elentok/gx/server/broker"
	"github.com/elentok/gx/server/projects"
	"github.com/elentok/gx/server/runs"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
)

func TestAgentEnv_SetsTheRunsAgent(t *testing.T) {
	t.Parallel()
	for _, agent := range []agent.AgentKind{agent.AgentClaude, agent.AgentCodex} {
		env := agentEnv([]string{"A=1"}, tickets.Ticket{}, tickets.Address{}, agent)
		if want := projects.AgentEnv + "=" + string(agent); !slices.Contains(env, want) {
			t.Errorf("agentEnv for %s = %v, want it to hold %q", agent, env, want)
		}
		if !slices.Contains(env, "A=1") {
			t.Errorf("agentEnv for %s = %v, lost the tab env", agent, env)
		}
	}
}

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

// A human Retry and the claim loop reserve through claimAndReserve, so one
// cannot count a slot as free while the other is between its cap check and its
// reservation.
func TestClaimAndReserve_SerialisesCallers(t *testing.T) {
	t.Parallel()
	s := &Scheduler{}
	inside, release, second := make(chan struct{}), make(chan struct{}), make(chan struct{})
	go func() {
		_, _ = s.claimAndReserve(func() (*claimed, error) {
			close(inside)
			<-release
			return nil, nil
		})
	}()
	<-inside
	go func() {
		_, _ = s.claimAndReserve(func() (*claimed, error) {
			close(second)
			return nil, nil
		})
	}()
	select {
	case <-second:
		t.Fatal("a second claim ran while the first held the claim lock")
	case <-time.After(50 * time.Millisecond):
	}
	close(release)
	select {
	case <-second:
	case <-time.After(5 * time.Second):
		t.Fatal("the second claim never ran after the first finished")
	}
}
