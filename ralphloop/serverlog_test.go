package ralphloop

import (
	"testing"

	"github.com/elentok/gx/events"
)

func TestAppendServerEvent_BudgetEventLandsInServerLogNotEpicLog(t *testing.T) {
	store := t.TempDir()

	if err := AppendEvent(store, "epic-a", Event{Type: eventResumed, Ticket: "01"}); err != nil {
		t.Fatal(err)
	}
	if err := AppendServerEvent(store, Event{Type: string(events.BudgetSoftLimitPaused)}); err != nil {
		t.Fatal(err)
	}

	server, ok, err := ReadServerEvents(store)
	if err != nil || !ok {
		t.Fatalf("ReadServerEvents ok=%v err=%v", ok, err)
	}
	if len(server) != 1 || server[0].Type != string(events.BudgetSoftLimitPaused) {
		t.Fatalf("server log = %+v, want one budget event", server)
	}

	epic, _, err := ReadEvents(store, "epic-a")
	if err != nil {
		t.Fatal(err)
	}
	if len(epic) != 1 || epic[0].Type != eventResumed {
		t.Fatalf("epic log = %+v, want only the resumed event", epic)
	}
}

func TestAppendServerEvent_RejectsAddressedEvent(t *testing.T) {
	store := t.TempDir()
	if err := AppendServerEvent(store, Event{Type: eventResumed, Ticket: "01"}); err == nil {
		t.Fatal("want error for an event that names a ticket")
	}
	if _, ok, _ := ReadServerEvents(store); ok {
		t.Fatal("rejected event must not create the server log")
	}
}
