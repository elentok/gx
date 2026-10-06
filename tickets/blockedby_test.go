package tickets

import "testing"

func TestFlagDanglingBlockers_RendersErrorWithValidateMessage(t *testing.T) {
	epic := Epic{Tickets: []Ticket{
		{Number: 1, Identifier: "01", Status: "done"},
		{Number: 2, Identifier: "02", Status: "open", BlockedBy: []string{"01"}},
		{Number: 3, Identifier: "03", Status: "open", BlockedBy: []string{"09"}},
	}}

	epic.flagDanglingBlockers()

	dangling := epic.Tickets[2]
	want := epic.CheckBlockedBy(dangling).Error()
	if dangling.BlockedByErr != want {
		t.Errorf("BlockedByErr = %q, want the validate message %q", dangling.BlockedByErr, want)
	}
	if got := epic.RenderedStatus(dangling); got != StatusError {
		t.Errorf("RenderedStatus(03) = %v, want StatusError", got)
	}
	if epic.Tickets[1].BlockedByErr != "" {
		t.Error("ticket 02's resolvable blocker was flagged")
	}
}
