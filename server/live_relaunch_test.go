package server_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/events"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/recovery"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
)

func TestRetryAndCancel_WorkingLeftoverRefuses(t *testing.T) {
	t.Parallel()
	h, fake, _, path := startWithLeftover(t, agentrunner.StateWorking)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	res, err := h.Client.TicketRetry(ctx, "proj:epic-a/01", false)
	if err != nil || !res.Refused || res.Reason != wire.ReasonAgentWorking {
		t.Errorf("retry = %+v, %v; want %s refusal", res, err, wire.ReasonAgentWorking)
	}
	res, err = h.Client.TicketCancel(ctx, "proj:epic-a/01", false)
	if err != nil || !res.Refused || res.Reason != wire.ReasonTicketLive {
		t.Errorf("cancel = %+v, %v; want %s refusal", res, err, wire.ReasonTicketLive)
	}
	if _, ok, _ := fake.Find(holderLabel); !ok {
		t.Error("a refused verb closed the working leftover")
	}
	if data, _ := os.ReadFile(path); strings.Contains(string(data), "status: c") {
		t.Errorf("a refused verb changed the ticket:\n%s", data)
	}
}

func TestCancel_StopClosesALeftoverThroughTheRunner(t *testing.T) {
	t.Parallel()
	h, fake, _, path := startWithLeftover(t, agentrunner.StateWorking)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if res, err := h.Client.TicketCancel(ctx, "proj:epic-a/01", true); err != nil || res.Refused {
		t.Fatalf("cancel --stop = %+v, %v", res, err)
	}
	if _, ok, _ := fake.Find(holderLabel); ok {
		t.Error("the leftover is still live")
	}
	if data, _ := os.ReadFile(path); !strings.Contains(string(data), "status: cancelled") {
		t.Errorf("ticket not cancelled:\n%s", data)
	}
}

func TestRelaunch_RecoveryEndsTheRoundWithSourceGx(t *testing.T) {
	t.Parallel()
	h, _, _, path := startWithLeftover(t, agentrunner.StateDone)
	if _, err := tickets.AppendLog(path, tickets.LogEvent{Type: tickets.EventPauseRepair, Summary: "stuck"}); err != nil {
		t.Fatal(err)
	}

	if _, err := h.Server.RecoveryDo(recovery.Call{Verb: recovery.VerbRelaunch, Address: "proj:epic-a/01"}); err != nil {
		t.Fatal(err)
	}
	rounds := endedRounds(t, path)
	if len(rounds) != 1 || rounds[0].End == nil || rounds[0].End.How != tickets.HowRelaunch || rounds[0].End.Source != tickets.SourceGx {
		t.Errorf("rounds = %+v; want the repair round ended by a gx relaunch", rounds)
	}
}

func TestRelaunch_OnlyAHumanRelaunchIsLogged(t *testing.T) {
	t.Parallel()
	h, _, _, _ := startWithLeftover(t, agentrunner.StateDone)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if _, err := h.Server.RecoveryDo(recovery.Call{Verb: recovery.VerbRelaunch, Address: "proj:epic-a/01"}); err != nil {
		t.Fatal(err)
	}
	if got := ofType(runLog(t, h), events.HumanRelaunch); len(got) != 0 {
		t.Fatalf("recovery relaunch logged %+v", got)
	}
	h.Server.Registry().Delete("proj:epic-a/01")
	if res, err := h.Client.TicketRelaunch(ctx, "proj:epic-a/01", true); err != nil || res.Refused {
		t.Fatalf("relaunch = %+v, %v", res, err)
	}
	if got := ofType(runLog(t, h), events.HumanRelaunch); len(got) != 1 || got[0].Ticket != "01" {
		t.Errorf("human-relaunch events = %+v, want one for 01", got)
	}
}

func TestGuardRail_AHumanRelaunchResetsTheBudget(t *testing.T) {
	t.Parallel()
	applied := func(kind string) ralphloop.Event {
		return ralphloop.Event{Type: string(events.RecoveryApplied), Ticket: "01", Kind: kind, Reason: "relaunch"}
	}
	full := []ralphloop.Event{applied("a"), applied("b"), applied("c")}
	f := recovery.Failure{Address: "proj:epic-a/01", Type: events.Paused, Kind: "d"}
	if why := server.GuardRailStop(full, f); why == "" {
		t.Fatal("three recoveries did not stop recovery")
	}
	human := append(full, ralphloop.Event{Type: string(events.HumanRelaunch), Ticket: "01"})
	if why := server.GuardRailStop(human, f); why != "" {
		t.Errorf("after a human relaunch: %q, want recovery allowed", why)
	}
	if why := server.GuardRailStop(append(human, applied("d")), f); why == "" {
		t.Error("a recovery after the human relaunch was not counted")
	}
}
