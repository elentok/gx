package tickets

import "testing"

func TestTicket_IsDone(t *testing.T) {
	doneValues := []string{"done", "Done", "DONE", " done "}
	for _, v := range doneValues {
		ticket := Ticket{Status: v}
		if !ticket.IsDone() {
			t.Errorf("Status %q should be IsDone", v)
		}
	}

	// The retired "done family" aliases are not done, and not anything else
	// either: the contracted enum has exactly one spelling per state.
	notDoneValues := []string{
		"", "open", "claimed", "needs-info", "needs-attention", "draft", "blocked", "bogus",
		"resolved", "wontfix", "closed", "implemented",
	}
	for _, v := range notDoneValues {
		ticket := Ticket{Status: v}
		if ticket.IsDone() {
			t.Errorf("Status %q should not be IsDone", v)
		}
	}
}
