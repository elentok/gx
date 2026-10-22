package tickets

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

// pauseActionLabel is the name of the action that clears a pause, by reason.
// Resume is only for a pause the person chose; every other fault is a Retry.
func pauseActionLabel(reason schema.PauseReason) string {
	switch reason {
	case schema.PauseSelfReported:
		return "Answer"
	case schema.PauseAmbiguousBase:
		return "Choose base"
	case schema.PauseMergeConflict:
		return "Resolve conflict"
	case schema.PauseStrandedCommits:
		return "Merge commits"
	case schema.PauseManual, "":
		// No reason means the ticket is not paused, so the item is dim anyway.
		return resumeLabel
	}
	return "Retry"
}

// isPausedTicket reports whether t is Paused, including a ticket still stored
// under an old needs-answer / needs-repair status.
func isPausedTicket(t tickets.Ticket) bool {
	switch schema.Status(t.Status) {
	case schema.StatusPaused, schema.StatusLegacyAnswer, schema.StatusLegacyRepair:
		return true
	}
	return false
}

// pausedStatusText is the status field of a Paused ticket: "paused" and its
// reason label.
func pausedStatusText(t tickets.Ticket) string {
	if label := pauseReasonLabel(t.PauseReason); label != "" {
		return "paused: " + label
	}
	return "paused"
}
