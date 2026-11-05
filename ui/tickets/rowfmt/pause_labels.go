package rowfmt

import (
	"strings"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// pauseReasonLabel is the plain-words reason shown beside "Paused". A reason
// the schema has no label for (a stale value) falls back to its own spelling.
func pauseReasonLabel(reason schema.PauseReason) string {
	if label := reason.Label(); label != "" {
		return label
	}
	words := strings.ReplaceAll(strings.ReplaceAll(string(reason), "-", " "), "_", " ")
	if words == "" {
		return ""
	}
	return strings.ToUpper(words[:1]) + words[1:]
}

// IsPausedTicket reports whether t is Paused, including a ticket still stored
// under an old needs-answer / needs-repair status.
func IsPausedTicket(t tickets.Ticket) bool {
	switch schema.Status(t.Status) {
	case schema.StatusPaused, schema.StatusLegacyAnswer, schema.StatusLegacyRepair:
		return true
	}
	return false
}

// PausedStatusText is the status field of a Paused ticket: "paused" and its
// reason label.
func PausedStatusText(t tickets.Ticket) string {
	if label := pauseReasonLabel(t.PauseReason); label != "" {
		return "paused: " + label
	}
	return "paused"
}
