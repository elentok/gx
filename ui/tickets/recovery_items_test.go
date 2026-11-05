package tickets

import (
	"slices"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
	"github.com/elentok/gx/ui/tickets/internal/tickettest"
)

// resumeLabel is the pause-to-verb table's label for resume.
var resumeLabel = schema.VerbResume.Label()

var (
	detailWith   = tickettest.DetailWith
	answerDetail = detailWith(wire.ActionAnswer, wire.ActionResume)
)

func TestServerMode_EnterSkipsWebOnlyActions(t *testing.T) {
	for _, tc := range []struct {
		name    string
		actions []string
		want    []string
	}{
		{"only web verbs", []string{wire.ActionAnswer, wire.ActionChooseBase}, nil},
		{"web verb first", []string{wire.ActionAnswer, wire.ActionResume}, []string{"resume gx:alpha/02"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls []string
			m := newServerModel(t).WithServer(tickettest.FakeServerAPI{Calls: &calls, DetailRes: detailWith(tc.actions...)}).WithTickets(tickettest.FakeServerAPI{Calls: &calls, DetailRes: detailWith(tc.actions...)}).WithAdmin(tickettest.FakeAdmin{FakeServerAPI: tickettest.FakeServerAPI{Calls: &calls, DetailRes: detailWith(tc.actions...)}}).WithQueue(tickettest.FakeQueue{FakeServerAPI: tickettest.FakeServerAPI{Calls: &calls, DetailRes: detailWith(tc.actions...)}}).WithReads(tickettest.FakeReads{FakeServerAPI: tickettest.FakeServerAPI{Calls: &calls, DetailRes: detailWith(tc.actions...)}})
			m = withState(m, wire.Snapshot{Tickets: []wire.TicketInfo{
				{Address: "gx:alpha/02", Title: "Second", Status: string(schema.StatusLegacyRepair), RenderedStatus: tickets.StatusPausedRepair.Word()},
			}})
			updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
			m = selectTicketRow(t, updated.(Model))

			_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
			if cmd != nil {
				cmd()
			}
			if !slices.Equal(calls, tc.want) {
				t.Fatalf("calls = %v, want %v", calls, tc.want)
			}
		})
	}
}

// Seam D: "enter" on a repair pause runs the first verb of detail.actions.
func TestServerMode_EnterRunsTheFirstDetailAction(t *testing.T) {
	var calls []string
	api := tickettest.FakeServerAPI{Calls: &calls, DetailRes: detailWith(wire.ActionRetry, wire.ActionStartOver)}
	m := newServerModel(t).WithServer(api).WithTickets(api).WithAdmin(tickettest.FakeAdmin{FakeServerAPI: api}).WithQueue(tickettest.FakeQueue{FakeServerAPI: api}).WithReads(tickettest.FakeReads{FakeServerAPI: api})
	m = withState(m, wire.Snapshot{Tickets: []wire.TicketInfo{
		{Address: "gx:alpha/02", Title: "Second", Status: string(schema.StatusLegacyRepair), RenderedStatus: tickets.StatusPausedRepair.Word()},
	}})
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = selectTicketRow(t, updated.(Model))

	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("enter on a paused row issued nothing")
	}
	cmd()
	if want := []string{"relaunch gx:alpha/02"}; !slices.Equal(calls, want) {
		t.Fatalf("calls = %v, want %v", calls, want)
	}
}

func TestServerMode_EnterWithNoActionsDoesNothing(t *testing.T) {
	var calls []string
	m := newServerModel(t).WithServer(tickettest.FakeServerAPI{Calls: &calls, DetailRes: detailWith()}).WithTickets(tickettest.FakeServerAPI{Calls: &calls, DetailRes: detailWith()}).WithAdmin(tickettest.FakeAdmin{FakeServerAPI: tickettest.FakeServerAPI{Calls: &calls, DetailRes: detailWith()}}).WithQueue(tickettest.FakeQueue{FakeServerAPI: tickettest.FakeServerAPI{Calls: &calls, DetailRes: detailWith()}}).WithReads(tickettest.FakeReads{FakeServerAPI: tickettest.FakeServerAPI{Calls: &calls, DetailRes: detailWith()}})
	m = withState(m, wire.Snapshot{Tickets: []wire.TicketInfo{
		{Address: "gx:alpha/02", Title: "Second", Status: string(schema.StatusLegacyRepair), RenderedStatus: tickets.StatusPausedRepair.Word()},
	}})
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = selectTicketRow(t, updated.(Model))

	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd != nil {
		if msg := cmd(); msg != nil {
			t.Fatalf("enter with no actions produced %T", msg)
		}
	}
	if len(calls) != 0 {
		t.Fatalf("calls = %v, want none", calls)
	}
}
