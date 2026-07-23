package tickets

import "testing"

func TestEpic_RenderedStatus_BaseStates(t *testing.T) {
	cases := []struct {
		status string
		want   RenderedStatus
	}{
		{"", StatusOpen},
		{"open", StatusOpen},
		{"ready-for-agent", StatusOpen},
		{"ready-for-human", StatusOpen},
		{"claimed", StatusClaimed},
		{"needs-info", StatusNeedsInfo},
		{"needs-triage", StatusOpen},
		{"done", StatusDone},
		{"resolved", StatusDone},
		{"wontfix", StatusDone},
		{"closed", StatusDone},
		{"superseded", StatusDone},
		{"CLAIMED", StatusClaimed},
		{"bogus-value", StatusError},
	}

	for _, c := range cases {
		epic := Epic{Tickets: []Ticket{{Number: 1, Status: c.status}}}
		got := epic.RenderedStatus(epic.Tickets[0])
		if got != c.want {
			t.Errorf("RenderedStatus(Status: %q) = %v, want %v", c.status, got, c.want)
		}
	}
}

func TestEpic_RenderedStatus_BlockedOverlaysOpenAndClaimed(t *testing.T) {
	epic := Epic{Tickets: []Ticket{
		{Number: 1, Status: "", BlockedBy: []int{2}},
		{Number: 2, Status: "open"},
	}}
	got := epic.RenderedStatus(epic.Tickets[0])
	if got != StatusBlocked {
		t.Errorf("RenderedStatus = %v, want StatusBlocked", got)
	}

	claimedEpic := Epic{Tickets: []Ticket{
		{Number: 1, Status: "claimed", BlockedBy: []int{2}},
		{Number: 2, Status: "open"},
	}}
	got = claimedEpic.RenderedStatus(claimedEpic.Tickets[0])
	if got != StatusBlocked {
		t.Errorf("RenderedStatus (claimed base) = %v, want StatusBlocked", got)
	}
}

func TestEpic_RenderedStatus_ResolvedBlockerDropsOverlay(t *testing.T) {
	epic := Epic{Tickets: []Ticket{
		{Number: 1, Status: "open", BlockedBy: []int{2}},
		{Number: 2, Status: "done"},
	}}
	got := epic.RenderedStatus(epic.Tickets[0])
	if got != StatusOpen {
		t.Errorf("RenderedStatus = %v, want StatusOpen once blocker is done", got)
	}
}

func TestEpic_RenderedStatus_NeedsInfoNotOverlaidByBlocked(t *testing.T) {
	epic := Epic{Tickets: []Ticket{
		{Number: 1, Status: "needs-info", BlockedBy: []int{2}},
		{Number: 2, Status: "open"},
	}}
	got := epic.RenderedStatus(epic.Tickets[0])
	if got != StatusNeedsInfo {
		t.Errorf("RenderedStatus = %v, want StatusNeedsInfo (blocked overlay only applies to open/claimed)", got)
	}
}

func TestEpic_RenderedStatus_DoneIgnoresBlockedBy(t *testing.T) {
	epic := Epic{Tickets: []Ticket{
		{Number: 1, Status: "done", BlockedBy: []int{2}},
		{Number: 2, Status: "open"},
	}}
	got := epic.RenderedStatus(epic.Tickets[0])
	if got != StatusDone {
		t.Errorf("RenderedStatus = %v, want StatusDone", got)
	}
}

func TestEpic_UnresolvedBlockers(t *testing.T) {
	epic := Epic{Tickets: []Ticket{
		{Number: 1, BlockedBy: []int{2, 3, 4}},
		{Number: 2, Status: "done"},
		{Number: 3, Status: "open"},
		// 4 doesn't exist in the epic: treated as still unresolved.
	}}
	got := epic.UnresolvedBlockers(epic.Tickets[0])
	want := []int{3, 4}
	if len(got) != len(want) {
		t.Fatalf("UnresolvedBlockers = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("UnresolvedBlockers = %v, want %v", got, want)
		}
	}
}

func TestEpic_UnresolvedBlockers_NoneWhenAllResolved(t *testing.T) {
	epic := Epic{Tickets: []Ticket{
		{Number: 1, BlockedBy: []int{2}},
		{Number: 2, Status: "resolved"},
	}}
	got := epic.UnresolvedBlockers(epic.Tickets[0])
	if got != nil {
		t.Errorf("UnresolvedBlockers = %v, want nil", got)
	}
}

func TestEpic_UnresolvedBlockers_NilWhenNoBlockedBy(t *testing.T) {
	epic := Epic{Tickets: []Ticket{{Number: 1}}}
	got := epic.UnresolvedBlockers(epic.Tickets[0])
	if got != nil {
		t.Errorf("UnresolvedBlockers = %v, want nil", got)
	}
}

func TestGroupOrder_ActionableSortsFirstDoneSortsLast(t *testing.T) {
	order := []RenderedStatus{StatusOpen, StatusClaimed, StatusBlocked, StatusNeedsInfo, StatusDone, StatusError}
	for i := 1; i < len(order); i++ {
		if GroupOrder(order[i-1]) > GroupOrder(order[i]) {
			t.Errorf("GroupOrder(%v) > GroupOrder(%v), want non-decreasing order", order[i-1], order[i])
		}
	}
	if GroupOrder(StatusOpen) != GroupOrder(StatusClaimed) {
		t.Errorf("open and claimed should share a group, got %d vs %d", GroupOrder(StatusOpen), GroupOrder(StatusClaimed))
	}
}
