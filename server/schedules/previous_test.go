package schedules

import (
	"testing"
	"time"

	"github.com/elentok/gx/tickets"
)

func TestDecide(t *testing.T) {
	t.Parallel()
	const addr = "proj/one-offs/01"
	claimed := time.Now()
	tests := []struct {
		name string
		prev *tickets.Ticket
		want Decision
	}{
		{"none", nil, Decision{Action: ActionStart}},
		{"done", &tickets.Ticket{Status: "done"}, Decision{Action: ActionStart}},
		{"cancelled", &tickets.Ticket{Status: "cancelled"}, Decision{Action: ActionStart}},
		{"queued never claimed", &tickets.Ticket{Status: "open"}, Decision{Action: ActionReplace, Address: addr}},
		{"queued after a claim", &tickets.Ticket{Status: "open", ClaimedAt: claimed}, Decision{Action: ActionSkip, Reason: ReasonPreviousRunStarted, Address: addr}},
		{"running", &tickets.Ticket{Status: "claimed", ClaimedAt: claimed}, Decision{Action: ActionSkip, Reason: ReasonPreviousRunStarted, Address: addr}},
		{"paused", &tickets.Ticket{Status: "paused"}, Decision{Action: ActionSkip, Reason: ReasonPreviousRunNeedsAttention, Address: addr}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := Decide(tc.prev, addr); got != tc.want {
				t.Errorf("Decide = %+v, want %+v", got, tc.want)
			}
		})
	}
}
