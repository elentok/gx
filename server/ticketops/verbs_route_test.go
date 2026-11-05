package ticketops_test

import (
	"context"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets/schema"
)

const routedLabel = "epic-a-iter-01"

func TestAgentVerbs_ReachARunOnAnotherRunner(t *testing.T) {
	t.Parallel()
	h, own, _ := liveUnderOther(t)
	ctx := context.Background()

	if res, err := h.Client.Tickets.AgentPrompt(ctx, "proj:epic-a/01", "keep going"); err != nil || res.Refused {
		t.Fatalf("prompt = %+v, %v", res, err)
	}
	if got := own.Prompts(routedLabel); !slices.Equal(got, []string{"keep going"}) {
		t.Errorf("run's runner got prompts %v", got)
	}
	// The configured runner has no such session, so reaching it would refuse
	// iteration-not-live; the run's own runner says nothing is blocked.
	if res, err := h.Client.Tickets.AgentAnswer(ctx, "proj:epic-a/01", "allow", ""); err != nil || res.Reason != wire.ReasonNotBlocked {
		t.Errorf("answer = %+v, %v; want refusal %s", res, err, wire.ReasonNotBlocked)
	}
	if res, err := h.Client.Tickets.AgentInterrupt(ctx, "proj:epic-a/01"); err != nil || res.Refused {
		t.Fatalf("interrupt = %+v, %v", res, err)
	}
}

func TestBudget_KillStopsTheSessionThroughTheRunsRunner(t *testing.T) {
	t.Parallel()
	h, own, ticketPath := liveUnderOther(t, func(c *server.Config) {
		c.BudgetHardLimit = 5
		c.BudgetKillGrace = time.Millisecond
		c.BudgetPollInterval = time.Hour
	})
	h.Server.Budget().SetCostOf(func(wire.IterationInfo) (float64, bool) { return 6, true })
	if !servertest.Poll(10*time.Second, func() bool {
		h.Server.Budget().PollBudget()
		tk, err := schema.ParseTicket(ticketPath)
		return err == nil && tk.PausedClass() == schema.PauseClassRepair
	}) {
		body, _ := os.ReadFile(ticketPath)
		t.Fatalf("not budget-killed:\n%s", body)
	}
	if _, live, _ := own.Find(routedLabel); live {
		t.Error("run's runner still hosts the session after the budget kill")
	}
}
