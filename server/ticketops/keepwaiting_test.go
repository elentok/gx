package ticketops_test

import (
	"context"
	"testing"

	"github.com/elentok/gx/events"
	"github.com/elentok/gx/ralphloop/runlog"
	"github.com/elentok/gx/tickets/schema"
)

func TestKeepWaiting_LiftsThePauseOnTheSameAgentAndLogsAFreshHeld(t *testing.T) {
	t.Parallel()
	h, ticketPath, fake := startWaitingRestartFake(t, 0)
	waitFor(t, func() bool { return len(h.Server.Runs()) == 1 })
	if err := h.Server.PauseAs(answerAddr, events.BackgroundTaskGate, "waited on background task task-1"); err != nil {
		t.Fatal(err)
	}
	sess, ok, err := fake.Find("epic-a-iter-01")
	if err != nil || !ok {
		t.Fatalf("Find = %v, %v", ok, err)
	}

	res, err := h.Client.Tickets.KeepWaiting(context.Background(), answerAddr)
	if err != nil || res.Refused {
		t.Fatalf("keep-waiting = %+v, %v", res, err)
	}

	tk, err := schema.ParseTicket(ticketPath)
	if err != nil || tk.Status != schema.StatusClaimed {
		t.Errorf("ticket = %+v, %v; want claimed", tk, err)
	}
	if again, ok, err := fake.Find("epic-a-iter-01"); err != nil || !ok || again.ID != sess.ID {
		t.Errorf("session = %+v, %v, %v; want the same live agent", again, ok, err)
	}
	if n := len(fake.Prompts("epic-a-iter-01")); n != 0 {
		t.Errorf("prompts = %d, want none", n)
	}
	log, _, err := runlog.ReadEvents(h.TicketStore+"/proj", "epic-a")
	if err != nil {
		t.Fatal(err)
	}
	var held int
	for _, ev := range log {
		if events.Type(ev.Type) == events.BackgroundTaskGateHeld {
			held++
		}
	}
	if held != 1 {
		t.Errorf("held events = %d, want 1", held)
	}
}

func TestKeepWaiting_RefusesAPauseThatIsNotABackgroundGate(t *testing.T) {
	t.Parallel()
	h, ticketPath, _ := startWaitingRestartFake(t, 0)
	waitFor(t, func() bool { return len(h.Server.Runs()) == 1 })
	pauseWithQuestion(t, ticketPath, optionsBlock)

	res, err := h.Client.Tickets.KeepWaiting(context.Background(), answerAddr)
	if err != nil || !res.Refused {
		t.Fatalf("keep-waiting = %+v, %v; want a refusal", res, err)
	}
}
