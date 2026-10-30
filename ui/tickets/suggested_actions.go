package tickets

import (
	"slices"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/ui/contextmenu"
)

// actionResume is the server's resume.
const actionResume = serverVerbResume

// actionInvestigate starts the server's Fix agent for the ticket (see
// investigateCmd), for any status signaling a problem (see statusActionItems).
const actionInvestigate = "investigate"

// actionUnmuteReopen is offered on any ticket carrying a non-empty Mutes
// field, independent of status — a mute that tripped with no pausing-a-ticket
// callback (see ralphloop.muteSource) never moves the ticket to
// paused (repair), so gating on status alone would leave that mute invisible
// and unclearable from the Tickets tab.
const actionUnmuteReopen = serverVerbUnmute

// resumeSuggested, investigateSuggested and unmuteSuggested decide which
// ticket Context menu items move to Suggested.
func resumeSuggested(status tickets.RenderedStatus) bool {
	return status == tickets.StatusPausedAnswer
}

// investigateSuggested whitelists the statuses that signal an actual problem
// rather than excluding the healthy ones: Blocked, Draft and
// WaitingForChildren are healthy too, with no live or stalled session worth
// investigating, and a status added to the enum later fails closed.
func investigateSuggested(status tickets.RenderedStatus) bool {
	return status == tickets.StatusPausedAnswer || status == tickets.StatusPausedRepair || status == tickets.StatusError
}

func unmuteSuggested(ticket tickets.Ticket) bool {
	return len(ticket.Mutes) > 0
}

// ticketHasSuggestedActions reports whether a row carries the "m" badge
// (ui.IconSet.SuggestedAction): its Context menu has an enabled Suggested
// item.
func ticketHasSuggestedActions(in ticketMenuInput) bool {
	// The row has no detail yet, so an answer pause's recovery items (suggested
	// once the menu opens) are counted here.
	if resumeSuggested(in.rendered) && in.serverReason == "" && !in.archived {
		return true
	}
	return slices.ContainsFunc(ticketMenuItems(in), func(it contextmenu.Item[ticketTarget]) bool {
		return it.Group == contextmenu.Suggested && it.Disabled == ""
	})
}
