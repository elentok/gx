package rowfmt

import (
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/ui"
)

// PauseReasonMaxRunes caps the ellipsized Pause reason subtext, rather than
// threading the panel's exact width through renderQueueTicketRow — RenderPanel's
// own ansi.Truncate still applies as a backstop on narrow terminals, same as
// blockedBySuffix's unbounded text today.
const PauseReasonMaxRunes = 60

// PausedRepairHeading is the literal heading markRepairWithReason
// (ralphloop/ticketops/claim.go) appends to a paused ticket's body.
const PausedRepairHeading = tickets.LegacyRepairHeading

// PausedAnswerHint is rendered under the preview's "Pause rounds" heading.
const PausedAnswerHint = "Answer in the web UI or run `gx tickets answer <addr>`."

// RowTitle is a list row's "<num> <title>". The title is a ticket's whole `#`
// heading, which can be long; capping it here (same reasoning as
// PauseReasonMaxRunes) keeps the row's suffix and metrics visible instead of
// losing them to the panel's own clip.
func RowTitle(icons ui.IconSet, t tickets.Ticket) string {
	return t.DisplayNumber() + " " + Ellipsize(t.Title, PauseReasonMaxRunes, icons.Ellipsis)
}

// Ellipsize caps text to maxRunes runes, appending ellipsisIcon when
// truncated.
func Ellipsize(text string, maxRunes int, ellipsisIcon string) string {
	runes := []rune(text)
	if len(runes) <= maxRunes {
		return text
	}
	return string(runes[:maxRunes]) + ellipsisIcon
}
