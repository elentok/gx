package server_test

import (
	"context"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/nativerunner"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/tickets/schema"
)

const routedLabel = "epic-a-iter-01"

// liveUnderOther restarts a server configured with runner "b" over a live,
// working run that runner "a" started. It returns the harness, a's fake and
// the ticket path.
func liveUnderOther(t *testing.T, opts ...func(*server.Config)) (*servertest.Harness, renamedFake, string) {
	t.Helper()
	return liveUnder(t, newRenamedFake("a", nativerunner.VerdictLive), opts...)
}

// liveUnder is liveUnderOther with the run's runner given as own.
func liveUnder(t *testing.T, own renamedFake, opts ...func(*server.Config)) (*servertest.Harness, renamedFake, string) {
	t.Helper()
	own.IDs = func(string) (string, string) { return "p1", "sess" }
	own.PromptState = agentrunner.StateWorking
	if _, err := own.Start(agentrunner.StartOptions{Label: routedLabel, Epic: "epic-a", Cwd: t.TempDir(), Kind: "claude"}); err != nil {
		t.Fatal(err)
	}
	own.SetState(routedLabel, agentrunner.StateWorking, "")
	h, ticketPath := restartUnder(t, own.name, newRenamedFake("b", nativerunner.VerdictLive), map[string]agentrunner.Runner{own.name: own}, opts...)
	deadline := time.Now().Add(10 * time.Second)
	for !slices.ContainsFunc(h.Server.Runs(), func(r server.Run) bool { return r.Runner == own.name }) {
		if time.Now().After(deadline) {
			t.Fatalf("run not reclaimed under %s: %+v", own.name, h.Server.Runs())
		}
		time.Sleep(20 * time.Millisecond)
	}
	return h, own, ticketPath
}

func TestAgentVerbs_ReachARunOnAnotherRunner(t *testing.T) {
	h, own, _ := liveUnderOther(t)
	ctx := context.Background()

	if res, err := h.Client.AgentPrompt(ctx, "proj:epic-a/01", "keep going"); err != nil || res.Refused {
		t.Fatalf("prompt = %+v, %v", res, err)
	}
	if got := own.Prompts(routedLabel); !slices.Equal(got, []string{"keep going"}) {
		t.Errorf("run's runner got prompts %v", got)
	}
	// The configured runner has no such session, so reaching it would refuse
	// iteration-not-live; the run's own runner says nothing is blocked.
	if res, err := h.Client.AgentAnswer(ctx, "proj:epic-a/01", "allow", ""); err != nil || res.Reason != server.ReasonNotBlocked {
		t.Errorf("answer = %+v, %v; want refusal %s", res, err, server.ReasonNotBlocked)
	}
	if res, err := h.Client.AgentInterrupt(ctx, "proj:epic-a/01"); err != nil || res.Refused {
		t.Fatalf("interrupt = %+v, %v", res, err)
	}
}

func TestBudget_KillStopsTheSessionThroughTheRunsRunner(t *testing.T) {
	h, own, ticketPath := liveUnderOther(t, func(c *server.Config) {
		c.BudgetHardLimit = 5
		c.BudgetKillGrace = time.Millisecond
		c.BudgetPollInterval = time.Hour
	})
	h.Server.SetCostOf(func(server.IterationInfo) (float64, bool) { return 6, true })
	deadline := time.Now().Add(10 * time.Second)
	for {
		h.Server.PollBudget()
		if tk, err := schema.ParseTicket(ticketPath); err == nil && tk.PausedClass() == schema.PauseClassRepair {
			break
		}
		if time.Now().After(deadline) {
			body, _ := os.ReadFile(ticketPath)
			t.Fatalf("not budget-killed:\n%s", body)
		}
		time.Sleep(50 * time.Millisecond)
	}
	if _, live, _ := own.Find(routedLabel); live {
		t.Error("run's runner still hosts the session after the budget kill")
	}
}
