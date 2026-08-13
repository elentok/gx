package tickets

import (
	"testing"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

func TestSuggestedActionItems_NeedsAnswer_NoMutes_ResumeAndInvestigate(t *testing.T) {
	t.Parallel()
	items := suggestedActionItems(tickets.StatusNeedsAnswer, tickets.Ticket{})
	if len(items) != 2 || items[0].Value != actionResumeAnswered || items[1].Value != actionInvestigate {
		t.Errorf("items = %v, want %q then %q", items, actionResumeAnswered, actionInvestigate)
	}
}

func TestSuggestedActionItems_MutedTicket_AnyStatus_IncludesUnmute(t *testing.T) {
	t.Parallel()
	muted := tickets.Ticket{Mutes: []schema.MuteRecord{{EventType: "notification-storm"}}}

	for _, status := range []tickets.RenderedStatus{tickets.StatusOpen, tickets.StatusNeedsRepair, tickets.StatusClaimed} {
		items := suggestedActionItems(status, muted)
		found := false
		for _, item := range items {
			if item.Value == actionUnmuteReopen {
				found = true
			}
		}
		if !found {
			t.Errorf("status %q: items = %v, want to include %q", status, items, actionUnmuteReopen)
		}
	}
}

func TestSuggestedActionItems_NoMutes_NoUnmuteAction(t *testing.T) {
	t.Parallel()
	items := suggestedActionItems(tickets.StatusOpen, tickets.Ticket{})
	for _, item := range items {
		if item.Value == actionUnmuteReopen {
			t.Errorf("items = %v, want no %q for a ticket with no Mutes", items, actionUnmuteReopen)
		}
	}
}

func TestTicketHasSuggestedActions_MutedTicket_True(t *testing.T) {
	t.Parallel()
	muted := tickets.Ticket{Mutes: []schema.MuteRecord{{EventType: "notification-storm"}}}
	if !ticketHasSuggestedActions(tickets.StatusOpen, muted) {
		t.Error("ticketHasSuggestedActions = false, want true for a muted ticket")
	}
}

func TestTicketHasSuggestedActions_NoMutesNoNeedsAnswer_False(t *testing.T) {
	t.Parallel()
	if ticketHasSuggestedActions(tickets.StatusOpen, tickets.Ticket{}) {
		t.Error("ticketHasSuggestedActions = true, want false for an unmuted open ticket")
	}
}
