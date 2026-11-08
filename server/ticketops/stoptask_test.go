package ticketops_test

import (
	"context"
	"testing"

	"github.com/elentok/gx/events"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets/schema"
)

// A run whose task cannot be tied to one process of a Claude session is
// refused and the pause stays: nothing was stopped, nothing lifted.
func TestStopTask_RefusesWhenNoTaskCanBeIdentified(t *testing.T) {
	t.Parallel()
	h, ticketPath, _ := startWaitingRestartFake(t, 0)
	waitFor(t, func() bool { return len(h.Server.Runs()) == 1 })
	if err := h.Server.PauseAs(answerAddr, events.BackgroundTaskGate, "waited on background task task-1"); err != nil {
		t.Fatal(err)
	}

	res, err := h.Client.Tickets.StopTask(context.Background(), answerAddr)
	if err != nil || !res.Refused {
		t.Fatalf("stop-task = %+v, %v; want a refusal", res, err)
	}
	if res.Reason != wire.ReasonNotStoppable && res.Reason != wire.ReasonTaskUnidentified {
		t.Errorf("reason = %q", res.Reason)
	}
	tk, err := schema.ParseTicket(ticketPath)
	if err != nil || tk.PausedClass() == "" {
		t.Errorf("ticket = %+v, %v; want still paused", tk, err)
	}
}

// Only a Claude task can be stopped: a Codex run's gate is refused as not
// stoppable and the pause stays.
func TestStopTask_RefusesACodexRun(t *testing.T) {
	t.Parallel()
	h, ticketPath, _ := servertest.StartWaitingRestartFakeAs(t, "codex", 0)
	waitFor(t, func() bool { return len(h.Server.Runs()) == 1 })
	if err := h.Server.PauseAs(answerAddr, events.BackgroundTaskGate, "waited on background task task-1"); err != nil {
		t.Fatal(err)
	}

	res, err := h.Client.Tickets.StopTask(context.Background(), answerAddr)
	if err != nil || !res.Refused || res.Reason != wire.ReasonNotStoppable {
		t.Fatalf("stop-task = %+v, %v; want not-stoppable", res, err)
	}
	tk, err := schema.ParseTicket(ticketPath)
	if err != nil || tk.PausedClass() == "" {
		t.Errorf("ticket = %+v, %v; want still paused", tk, err)
	}
}

func TestStopTask_RefusesAPauseThatIsNotABackgroundGate(t *testing.T) {
	t.Parallel()
	h, ticketPath, _ := startWaitingRestartFake(t, 0)
	waitFor(t, func() bool { return len(h.Server.Runs()) == 1 })
	pauseWithQuestion(t, ticketPath, optionsBlock)

	res, err := h.Client.Tickets.StopTask(context.Background(), answerAddr)
	if err != nil || !res.Refused || res.Reason != wire.ReasonNotPaused {
		t.Fatalf("stop-task = %+v, %v; want not-paused", res, err)
	}
}
