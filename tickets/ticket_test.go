package tickets

import "testing"

func TestTicket_IsDone(t *testing.T) {
	doneValues := []string{"done", "resolved", "wontfix", "closed", "Done", "RESOLVED"}
	for _, v := range doneValues {
		ticket := Ticket{Status: v}
		if !ticket.IsDone() {
			t.Errorf("Status %q should be IsDone", v)
		}
	}

	notDoneValues := []string{"", "open", "claimed", "needs-info", "ready-for-agent", "blocked", "bogus"}
	for _, v := range notDoneValues {
		ticket := Ticket{Status: v}
		if ticket.IsDone() {
			t.Errorf("Status %q should not be IsDone", v)
		}
	}
}
