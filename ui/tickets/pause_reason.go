package tickets

import (
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/ui"
)

// pauseReasonMaxRunes caps the ellipsized Pause reason subtext, rather than
// threading the panel's exact width through renderQueueTicketRow — RenderPanel's
// own ansi.Truncate still applies as a backstop on narrow terminals, same as
// blockedBySuffix's unbounded text today.
const pauseReasonMaxRunes = 60

// pausedRepairHeading is the literal heading markRepairWithReason
// (ralphloop/claim.go) appends to a paused ticket's body.
const pausedRepairHeading = tickets.LegacyRepairHeading

// pausedAnswerHint is rendered under the preview's "Pause rounds" heading.
const pausedAnswerHint = "Answer in the web UI or run `gx tickets answer <addr>`."

// rowTitle is a list row's "<num> <title>". The title is a ticket's whole `#`
// heading, which can be long; capping it here (same reasoning as
// pauseReasonMaxRunes) keeps the row's suffix and metrics visible instead of
// losing them to the panel's own clip.
func rowTitle(icons ui.IconSet, t tickets.Ticket) string {
	return t.DisplayNumber() + " " + ellipsize(t.Title, pauseReasonMaxRunes, icons.Ellipsis)
}

// ellipsize caps text to maxRunes runes, appending ellipsisIcon when
// truncated.
func ellipsize(text string, maxRunes int, ellipsisIcon string) string {
	runes := []rune(text)
	if len(runes) <= maxRunes {
		return text
	}
	return string(runes[:maxRunes]) + ellipsisIcon
}
