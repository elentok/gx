package rowactions

import (
	"github.com/elentok/gx/tickets"
)

// actionResume is the server's resume.
const ActionResume = "resume"

// actionInvestigate starts the server's Fix agent for the ticket (see
// investigateCmd), for any status signaling a problem (see statusActionItems).
const ActionInvestigate = "investigate"

// actionUnmuteReopen is offered on any ticket carrying a non-empty Mutes
// field, independent of status — a mute that tripped with no pausing-a-ticket
// callback (see ralphloop.muteSource) never moves the ticket to
// paused (repair), so gating on status alone would leave that mute invisible
// and unclearable from the Tickets tab.
const ActionUnmuteReopen = "unmute"

// resumeSuggested, investigateSuggested and unmuteSuggested decide which
// ticket Context menu items move to Suggested.
func ResumeSuggested(status tickets.RenderedStatus) bool {
	return status == tickets.StatusPausedAnswer
}

// investigateSuggested whitelists the statuses that signal an actual problem
// rather than excluding the healthy ones: Blocked, Draft and
// WaitingForChildren are healthy too, with no live or stalled session worth
// investigating, and a status added to the enum later fails closed.
func InvestigateSuggested(status tickets.RenderedStatus) bool {
	return status == tickets.StatusPausedAnswer || status == tickets.StatusPausedRepair || status == tickets.StatusError
}

func UnmuteSuggested(ticket tickets.Ticket) bool {
	return len(ticket.Mutes) > 0
}
