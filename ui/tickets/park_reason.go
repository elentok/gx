package tickets

import (
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/ui"
)

// parkReasonMaxRunes caps the ellipsized Park reason subtext, rather than
// threading the panel's exact width through renderQueueTicketRow — RenderPanel's
// own ansi.Truncate still applies as a backstop on narrow terminals, same as
// blockedBySuffix's unbounded text today.
const parkReasonMaxRunes = 60

// needsRepairHeading is the literal heading markNeedsRepairWithReason
// (ralphloop/claim.go) appends to a parked ticket's body.
const needsRepairHeading = tickets.LegacyRepairHeading

// needsAnswerHint is rendered under the preview's "Park rounds" heading.
const needsAnswerHint = "Answer in the web UI or run `gx tickets answer <addr>`."

// rowTitle is a list row's "<num> <title>". The title is a ticket's whole `#`
// heading, which can be long; capping it here (same reasoning as
// parkReasonMaxRunes) keeps the row's suffix and metrics visible instead of
// losing them to the panel's own clip.
func rowTitle(icons ui.IconSet, t tickets.Ticket) string {
	return t.DisplayNumber() + " " + ellipsize(t.Title, parkReasonMaxRunes, icons.Ellipsis)
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
