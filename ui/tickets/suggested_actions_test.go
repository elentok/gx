package tickets

import (
	"slices"
	"testing"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
	"github.com/elentok/gx/ui/components"
)

func TestSuggestedActionItems_NeedsAnswer_NoMutes_ResumeAndInvestigate(t *testing.T) {
	t.Parallel()
	items := suggestedActionItems(tickets.StatusNeedsAnswer, tickets.Ticket{}, false, false, false)
	want := []string{actionAnswer, actionResumeAnswered, actionInvestigate}
	if got := itemValues(items); !slices.Equal(got, want) {
		t.Errorf("items = %v, want %v", got, want)
	}
}

func TestSuggestedActionItems_HerdrDown_HidesHerdrOnlyItems(t *testing.T) {
	t.Parallel()
	items := suggestedActionItems(tickets.StatusNeedsAnswer, tickets.Ticket{}, true, false, true)
	want := []string{actionAnswer, actionResumeAnswered}
	if got := itemValues(items); !slices.Equal(got, want) {
		t.Errorf("items = %v, want %v", got, want)
	}
	if ticketHasSuggestedActions(tickets.StatusNeedsRepair, tickets.Ticket{}, true) {
		t.Error("needs-repair carries a badge with herdr down, want none: Investigate is its only item")
	}
}

func TestSuggestedActionItems_NeedsAnswer_PaneLive_AnswerInPaneFirst(t *testing.T) {
	t.Parallel()
	items := suggestedActionItems(tickets.StatusNeedsAnswer, tickets.Ticket{}, true, false, false)
	want := []string{actionAnswerInPane, actionResumeAnswered, actionInvestigate}
	if got := itemValues(items); !slices.Equal(got, want) {
		t.Errorf("items = %v, want %v", got, want)
	}
}

func TestSuggestedActionItems_NeedsAnswer_Native_WatchReplacesAnswerInPane(t *testing.T) {
	t.Parallel()
	items := suggestedActionItems(tickets.StatusNeedsAnswer, tickets.Ticket{}, true, true, false)
	want := []string{actionWatchAgent, actionAnswer, actionResumeAnswered, actionInvestigate}
	if got := itemValues(items); !slices.Equal(got, want) {
		t.Errorf("items = %v, want %v", got, want)
	}
}

func itemValues(items []components.MenuItem) []string {
	values := make([]string, len(items))
	for i, item := range items {
		values[i] = item.Value
	}
	return values
}

func TestSuggestedActionItems_NeedsRepairOrError_IncludesInvestigate(t *testing.T) {
	t.Parallel()
	for _, status := range []tickets.RenderedStatus{tickets.StatusNeedsRepair, tickets.StatusError} {
		items := suggestedActionItems(status, tickets.Ticket{}, false, false, false)
		if len(items) != 1 || items[0].Value != actionInvestigate {
			t.Errorf("status %v: items = %v, want just %q", status, items, actionInvestigate)
		}
	}
}

func TestSuggestedActionItems_HealthyStatuses_NoInvestigate(t *testing.T) {
	t.Parallel()
	for _, status := range []tickets.RenderedStatus{
		tickets.StatusOpen,
		tickets.StatusClaimed,
		tickets.StatusDone,
		tickets.StatusBlocked,
		tickets.StatusDraft,
		tickets.StatusWaitingForChildren,
	} {
		items := suggestedActionItems(status, tickets.Ticket{}, false, false, false)
		if len(items) != 0 {
			t.Errorf("status %v: items = %v, want none", status, items)
		}
	}
}

func TestSuggestedActionItems_MutedTicket_AnyStatus_IncludesUnmute(t *testing.T) {
	t.Parallel()
	muted := tickets.Ticket{Mutes: []schema.MuteRecord{{EventType: "notification-storm"}}}

	for _, status := range []tickets.RenderedStatus{tickets.StatusOpen, tickets.StatusNeedsRepair, tickets.StatusClaimed} {
		items := suggestedActionItems(status, muted, false, false, false)
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
	items := suggestedActionItems(tickets.StatusOpen, tickets.Ticket{}, false, false, false)
	for _, item := range items {
		if item.Value == actionUnmuteReopen {
			t.Errorf("items = %v, want no %q for a ticket with no Mutes", items, actionUnmuteReopen)
		}
	}
}

func TestTicketHasSuggestedActions_MutedTicket_True(t *testing.T) {
	t.Parallel()
	muted := tickets.Ticket{Mutes: []schema.MuteRecord{{EventType: "notification-storm"}}}
	if !ticketHasSuggestedActions(tickets.StatusOpen, muted, false) {
		t.Error("ticketHasSuggestedActions = false, want true for a muted ticket")
	}
}

func TestTicketHasSuggestedActions_NoMutesNoNeedsAnswer_False(t *testing.T) {
	t.Parallel()
	if ticketHasSuggestedActions(tickets.StatusOpen, tickets.Ticket{}, false) {
		t.Error("ticketHasSuggestedActions = true, want false for an unmuted open ticket")
	}
}
