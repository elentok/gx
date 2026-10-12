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
	items := statusActionItems(tickets.StatusNeedsAnswer, tickets.Ticket{}, false)
	want := []string{actionAnswer, actionResumeAnswered, actionInvestigate}
	if got := itemValues(items); !slices.Equal(got, want) {
		t.Errorf("items = %v, want %v", got, want)
	}
}

func TestSuggestedActionItems_HerdrDown_HidesHerdrOnlyItems(t *testing.T) {
	t.Parallel()
	items := suggestedActionItems(tickets.StatusNeedsAnswer, tickets.Ticket{}, true, false, true)
	want := []string{actionAnswer, actionResumeAnswered, actionTranscript}
	if got := itemValues(items); !slices.Equal(got, want) {
		t.Errorf("items = %v, want %v", got, want)
	}
	if ticketHasSuggestedActions(tickets.StatusNeedsRepair, tickets.Ticket{}, true) {
		t.Error("needs-repair carries a badge with herdr down, want none: Investigate is its only item")
	}
}

func TestSuggestedActionItems_WatchOnlyWhileLive(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name                            string
		paneLive, nativeLive, herdrDown bool
		want                            bool
	}{
		{"nothing live", false, false, false, false},
		{"herdr pane live", true, false, false, true},
		{"native agent live", false, true, false, true},
		{"native agent live, herdr down", false, true, true, true},
	}
	for _, status := range []tickets.RenderedStatus{tickets.StatusClaimed, tickets.StatusNeedsAnswer} {
		for _, c := range cases {
			items := itemValues(suggestedActionItems(status, tickets.Ticket{}, c.paneLive, c.nativeLive, c.herdrDown))
			if got := slices.Contains(items, actionWatch); got != c.want {
				t.Errorf("%v, %s: items = %v, want Watch %v", status, c.name, items, c.want)
			}
		}
	}
}

func TestSuggestedActionItems_WatchBeforeTranscript_NoBadge(t *testing.T) {
	t.Parallel()
	items := suggestedActionItems(tickets.StatusNeedsAnswer, tickets.Ticket{}, true, false, false)
	want := []string{actionAnswer, actionResumeAnswered, actionInvestigate, actionWatch, actionTranscript}
	if got := itemValues(items); !slices.Equal(got, want) {
		t.Errorf("items = %v, want %v", got, want)
	}
	if ticketHasSuggestedActions(tickets.StatusClaimed, tickets.Ticket{}, false) {
		t.Error("a claimed ticket carries a badge, want none: Watch alone must not add one")
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
		items := statusActionItems(status, tickets.Ticket{}, false)
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
		items := statusActionItems(status, tickets.Ticket{}, false)
		if len(items) != 0 {
			t.Errorf("status %v: items = %v, want none", status, items)
		}
	}
}

func TestSuggestedActionItems_MutedTicket_AnyStatus_IncludesUnmute(t *testing.T) {
	t.Parallel()
	muted := tickets.Ticket{Mutes: []schema.MuteRecord{{EventType: "notification-storm"}}}

	for _, status := range []tickets.RenderedStatus{tickets.StatusOpen, tickets.StatusNeedsRepair, tickets.StatusClaimed} {
		items := statusActionItems(status, muted, false)
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
	items := statusActionItems(tickets.StatusOpen, tickets.Ticket{}, false)
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
