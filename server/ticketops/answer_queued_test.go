package ticketops_test

import (
	"io"
	"log/slog"
	"sync"
	"testing"

	"github.com/elentok/gx/ralphloop/deps"
	"github.com/elentok/gx/server/broker"
	"github.com/elentok/gx/server/index"
	"github.com/elentok/gx/server/projects"
	"github.com/elentok/gx/server/runs"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/settings"
	"github.com/elentok/gx/server/ticketops"
	"github.com/elentok/gx/server/ticketwrite"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// launchFake is a ticketops.Launch that records what the operations ask of the
// run loop and answers the budget check as told.
type launchFake struct {
	mu      sync.Mutex
	budget  *wire.QueueResult
	settled []string
}

func (l *launchFake) BudgetRefusal() (wire.QueueResult, bool) {
	if l.budget == nil {
		return wire.QueueResult{}, false
	}
	return *l.budget, true
}

func (l *launchFake) HostingDeps(tickets.Address) deps.Deps { return deps.Deps{} }

func (l *launchFake) CompleteMergedRoot(tickets.Address) {}

func (l *launchFake) ResettleWaiting(t runs.TrackedRun) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.settled = append(l.settled, t.Address)
	return nil
}

// An answer to a ticket whose run is not tracked is queued: the ticket opens
// again and nothing is settled, even
// with the budget spent.
func TestAnswer_QueuedPathUsesTheLaunchFake(t *testing.T) {
	t.Parallel()
	store := t.TempDir()
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.SetProjectRepo(t, store, "proj", t.TempDir())
	ticketPath := store + "/proj/epic-a/issues/01-first.md"
	state := pauseWithQuestion(t, ticketPath, optionsBlock)

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	set := &settings.Holder{}
	set.Store(&settings.Settings{})
	reg, _, err := runs.Open(t.TempDir(), log)
	if err != nil {
		t.Fatal(err)
	}
	idx := index.New(store, broker.New(8), log)
	projs := projects.NewRegistry(func() projects.Env { return projects.Env{TicketStore: store} }, log, idx, set, nil, nil)
	launch := &launchFake{budget: &wire.QueueResult{Refused: true, Reason: wire.ReasonBudgetLimit}}
	ops := ticketops.New(ticketops.Deps{
		Writer:   ticketwrite.New(ticketwrite.Deps{Log: log, Index: idx, Projects: projs, Registry: reg}),
		Registry: reg,
		Index:    idx,
		Launch:   launch,
	})

	res, err := ops.Answer(wire.QueueRequest{Address: answerAddr, State: state, Choice: "a"})
	if err != nil || res.Refused || res.Outcome != wire.OutcomeQueued {
		t.Fatalf("answer = %+v, %v; want queued", res, err)
	}
	if n := answerCount(t, ticketPath); n != 1 {
		t.Errorf("pause.answer lines = %d, want 1", n)
	}
	if tk, err := schema.ParseTicket(ticketPath); err != nil || tk.Status != schema.StatusOpen {
		t.Errorf("ticket = %+v, %v; want open", tk, err)
	}
	if len(launch.settled) != 0 {
		t.Errorf("settled = %v; want no settle", launch.settled)
	}
}
