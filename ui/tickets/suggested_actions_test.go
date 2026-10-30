package tickets

import (
	"slices"
	"testing"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
	"github.com/elentok/gx/ui/contextmenu"
)

func TestTicketMenuItems_SuggestableItemsBuiltOnce(t *testing.T) {
	t.Parallel()
	type want struct{ investigate, unmute contextmenu.Group }
	home := want{contextmenu.Agent, contextmenu.Server}
	for _, tt := range []struct {
		name string
		in   ticketMenuInput
		want want
	}{
		{"open", ticketMenuInput{rendered: tickets.StatusOpen}, home},
		{"claimed", ticketMenuInput{rendered: tickets.StatusClaimed}, home},
		{"paused (answer)", ticketMenuInput{rendered: tickets.StatusPausedAnswer, ticket: tickets.Ticket{Status: string(schema.StatusPaused)}},
			want{contextmenu.Suggested, contextmenu.Server}},
		{"paused (repair)", ticketMenuInput{rendered: tickets.StatusPausedRepair, ticket: tickets.Ticket{Status: string(schema.StatusPaused), PauseReason: schema.PauseManual}},
			want{contextmenu.Suggested, contextmenu.Server}},
		{"error", ticketMenuInput{rendered: tickets.StatusError}, want{contextmenu.Suggested, contextmenu.Server}},
		{"muted open", ticketMenuInput{rendered: tickets.StatusOpen, ticket: tickets.Ticket{Mutes: muted}},
			want{contextmenu.Agent, contextmenu.Suggested}},
	} {
		items := ticketMenuItems(tt.in)
		for label, group := range map[string]contextmenu.Group{
			"Investigate": tt.want.investigate, "Unmute & Reopen": tt.want.unmute,
		} {
			var groups []contextmenu.Group
			for _, it := range items {
				if it.Label == label {
					groups = append(groups, it.Group)
				}
			}
			if !slices.Equal(groups, []contextmenu.Group{group}) {
				t.Errorf("%s: %s groups = %v, want [%v]", tt.name, label, groups, group)
			}
		}
		if hasItem(items, "Answer…") {
			t.Errorf("%s: menu offers Answer…", tt.name)
		}
	}
}

func TestTicketHasSuggestedActions_AgreesWithMenu(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name string
		in   ticketMenuInput
		want bool
	}{
		{"open", ticketMenuInput{rendered: tickets.StatusOpen}, false},
		{"claimed", ticketMenuInput{rendered: tickets.StatusClaimed}, false},
		{"done", ticketMenuInput{rendered: tickets.StatusDone}, false},
		{"blocked", ticketMenuInput{rendered: tickets.StatusBlocked}, false},
		{"paused (answer)", ticketMenuInput{rendered: tickets.StatusPausedAnswer}, true},
		// The row has no detail; the menu opens with the answer verbs Suggested.
		{"paused (answer) herdr down", ticketMenuInput{rendered: tickets.StatusPausedAnswer, herdrDown: true, detail: &answerDetail}, true},
		{"paused (repair)", ticketMenuInput{rendered: tickets.StatusPausedRepair}, true},
		{"paused (repair) herdr down", ticketMenuInput{rendered: tickets.StatusPausedRepair, herdrDown: true}, true},
		{"error herdr down", ticketMenuInput{rendered: tickets.StatusError, herdrDown: true}, true},
		{"muted open", ticketMenuInput{rendered: tickets.StatusOpen, ticket: tickets.Ticket{Mutes: muted}}, true},
		{"muted open server down", ticketMenuInput{rendered: tickets.StatusOpen, ticket: tickets.Ticket{Mutes: muted}, serverReason: serverNotRunning}, false},
		{"open on a map", ticketMenuInput{rendered: tickets.StatusOpen, isMap: true}, true},
		{"archived paused (repair)", ticketMenuInput{rendered: tickets.StatusPausedRepair, archived: true}, false},
	} {
		menu := slices.ContainsFunc(ticketMenuItems(tt.in), func(it contextmenu.Item[ticketTarget]) bool {
			return it.Group == contextmenu.Suggested && it.Disabled == ""
		})
		if got := ticketHasSuggestedActions(tt.in); got != tt.want || menu != tt.want {
			t.Errorf("%s: badge = %v, menu has enabled Suggested = %v, want %v", tt.name, got, menu, tt.want)
		}
	}
}
