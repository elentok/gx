package server_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/events"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil/runnerfake"
)

// budgetEventInterval outlasts a burst of budget polls, which take tens of
// milliseconds each, so the burst test sees one window.
const budgetEventInterval = 300 * time.Millisecond

// budgetEventFixture runs a claimed ticket whose cost the test sets, with the
// background poll off so each poll is the test's own.
type budgetEventFixture struct {
	h    *servertest.Harness
	mu   sync.Mutex
	cost float64
}

func newBudgetEventFixture(t *testing.T) *budgetEventFixture {
	t.Helper()
	store := t.TempDir()
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.SetProjectRepo(t, store, "proj", t.TempDir())
	fake := runnerfake.NewRunner()
	h := servertest.StartWithStore(t, store, servertest.WithRunner(fake), func(c *server.Config) {
		c.BudgetPollInterval = time.Hour
		c.BudgetEventInterval = budgetEventInterval
	})
	f := &budgetEventFixture{h: h}
	h.Server.SetCostOf(func(server.IterationInfo) (float64, bool) {
		f.mu.Lock()
		defer f.mu.Unlock()
		return f.cost, true
	})
	// A nudged pane of a paused ticket is a live, metered iteration.
	if err := h.Server.PauseAs("proj:epic-a/01", events.IterationError, "setup"); err != nil {
		t.Fatal(err)
	}
	if _, err := fake.Start(agentrunner.StartOptions{Label: "epic-a-iter-01", Epic: "epic-a", Cwd: t.TempDir(), Kind: "claude"}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if res, err := h.Client.TicketNudge(ctx, "proj:epic-a/01", "go"); err != nil || res.Refused {
		t.Fatalf("nudge = %+v, %v", res, err)
	}
	return f
}

// spend sets the iteration's total cost and polls the budget once.
func (f *budgetEventFixture) spend(total float64) {
	f.mu.Lock()
	f.cost = total
	f.mu.Unlock()
	f.h.Server.PollBudget()
}

func (f *budgetEventFixture) stream(t *testing.T) <-chan server.Event {
	t.Helper()
	return limitsEventStream(t, f.h)
}

// budgetEvents counts the budget-changed events on ch until it is quiet.
func budgetEvents(ch <-chan server.Event) int {
	n := 0
	for {
		select {
		case ev := <-ch:
			if ev.Type == wire.EventBudgetChanged {
				n++
			}
		case <-time.After(3 * budgetEventInterval):
			return n
		}
	}
}

func wantBudgetEvents(t *testing.T, ch <-chan server.Event, want int) {
	t.Helper()
	if got := budgetEvents(ch); got != want {
		t.Fatalf("budget-changed events = %d, want %d", got, want)
	}
}

func TestBudgetEvent_IncreasePublishes(t *testing.T) {
	f := newBudgetEventFixture(t)
	f.spend(1)
	ch := f.stream(t)
	f.spend(2)
	wantBudgetEvents(t, ch, 1)
}

func TestBudgetEvent_QuietPollPublishesNothing(t *testing.T) {
	f := newBudgetEventFixture(t)
	f.spend(1)
	ch := f.stream(t)
	f.spend(1)
	f.spend(1)
	wantBudgetEvents(t, ch, 0)
}

func TestBudgetEvent_FirstPollPublishesNothing(t *testing.T) {
	f := newBudgetEventFixture(t)
	ch := f.stream(t)
	f.spend(3)
	wantBudgetEvents(t, ch, 0)
}

func TestBudgetEvent_BurstCoalescesAndSummaryShowsFinalTotal(t *testing.T) {
	f := newBudgetEventFixture(t)
	f.spend(1)
	ch := f.stream(t)
	for total := 2.0; total <= 6; total++ {
		f.spend(total)
	}
	if got := budgetEvents(ch); got < 1 || got > 2 {
		t.Fatalf("budget-changed events = %d, want a leading and at most one trailing", got)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	sum, err := f.h.Client.UsageSummary(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if sum.Spend.Today != 6 {
		t.Fatalf("today = %v, want 6", sum.Spend.Today)
	}
}

func TestBudgetEvent_IncreaseAfterIntervalPublishesAgain(t *testing.T) {
	f := newBudgetEventFixture(t)
	f.spend(1)
	ch := f.stream(t)
	f.spend(2)
	wantBudgetEvents(t, ch, 1)
	f.spend(3)
	wantBudgetEvents(t, ch, 1)
}
