package tickets

import (
	"testing"

	"github.com/elentok/gx/tickets"
)

func TestBlockedBySuffix_FewBlockersListsIds(t *testing.T) {
	t.Parallel()
	epic := tickets.Epic{Name: "epic", Tickets: []tickets.Ticket{
		{Identifier: "01", Status: "open"},
		{Identifier: "02", Status: "open"},
		{Identifier: "03", Status: "blocked", BlockedBy: []string{"01", "02"}},
	}}
	ticket := epic.Tickets[2]

	got := blockedBySuffix(epic, ticket, tickets.StatusBlocked)

	want := "(blocked by 01, 02)"
	if got != want {
		t.Fatalf("blockedBySuffix() = %q, want %q", got, want)
	}
}

func TestBlockedBySuffix_ManyBlockersCollapsesToCount(t *testing.T) {
	t.Parallel()
	epic := tickets.Epic{Name: "epic", Tickets: []tickets.Ticket{
		{Identifier: "01", Status: "open"},
		{Identifier: "02", Status: "open"},
		{Identifier: "03", Status: "open"},
		{Identifier: "04", Status: "open"},
		{Identifier: "05", Status: "blocked", BlockedBy: []string{"01", "02", "03", "04"}},
	}}
	ticket := epic.Tickets[4]

	got := blockedBySuffix(epic, ticket, tickets.StatusBlocked)

	want := "(blocked by 4 tickets)"
	if got != want {
		t.Fatalf("blockedBySuffix() = %q, want %q", got, want)
	}
}
