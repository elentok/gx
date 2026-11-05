package tickets

import (
	"slices"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/ui/tickets/internal/tickettest"
	"github.com/elentok/gx/ui/tickets/queue"
)

func TestServerMode_FinishAndPauseKeyIsFinishAndPause(t *testing.T) {
	var calls []string
	m := newServerModel(t).WithServer(tickettest.FakeServerAPI{Calls: &calls}).WithTickets(tickettest.FakeServerAPI{Calls: &calls}).WithAdmin(tickettest.FakeAdmin{FakeServerAPI: tickettest.FakeServerAPI{Calls: &calls}}).WithQueue(tickettest.FakeQueue{FakeServerAPI: tickettest.FakeServerAPI{Calls: &calls}}).WithReads(tickettest.FakeReads{FakeServerAPI: tickettest.FakeServerAPI{Calls: &calls}})
	next, _ := m.handleServerFinishAndPauseKey()
	m = next.(Model)
	if !m.confirm.IsOpen {
		t.Fatal("confirm not open")
	}
	_, cmd, _ := m.confirm.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	if cmd == nil {
		t.Fatal("accept produced no command")
	}
	if _, ok := cmd().(queue.ServerDoneMsg); !ok || !slices.Equal(calls, []string{"pause"}) {
		t.Fatalf("calls = %v", calls)
	}
}
