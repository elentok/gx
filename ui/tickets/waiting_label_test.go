package tickets

import (
	"strings"
	"testing"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/ui/tickets/queue"
)

func TestQueueRow_WaitingLabel(t *testing.T) {
	t.Parallel()
	epic := tickets.Epic{Name: "epic", Tickets: []tickets.Ticket{
		{Identifier: "01", Path: "gx:epic/01", Title: "Waiter", Status: "claimed"},
		{Identifier: "02", Path: "gx:epic/02", Title: "Other", Status: "claimed"},
	}}
	const label = "waiting for the check lock, held by 7"
	var m QueueModel
	m.server.waitingLabels = map[string]string{"gx:epic/01": label}

	if got := m.rows.RenderTicketRow(m.rowsEnv(), queue.Row{Epic: epic, Ticket: epic.Tickets[0]}, 0); !strings.Contains(got, label) {
		t.Errorf("waiting row = %q, want label %q", got, label)
	}
	if got := m.rows.RenderTicketRow(m.rowsEnv(), queue.Row{Epic: epic, Ticket: epic.Tickets[1]}, 1); strings.Contains(got, "waiting for") {
		t.Errorf("non-waiting row = %q, want no label", got)
	}
}
