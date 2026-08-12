package tickets

import (
	"time"

	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/ui/components"
)

// actionResumeAnswered is the only suggested action today: reopen a
// needs-answer ticket and demote its "## Needs Answer" stub, the same write
// unparkAnswered performs automatically for a gate park (ralphloop.UnparkTicket).
// This menu exists for the park kind that pre-pass can't safely auto-clear —
// an announce-and-stop (zero-commit) park's live-but-idle pane looks
// identical to an answered gate park, so a person confirms it explicitly here
// instead.
const actionResumeAnswered = "resume-answered"

// suggestedActionItems returns status's suggested-action menu items, empty
// when none apply. Only needs-answer carries one today; every other status
// intentionally has none (handleSuggestedActionsKey/handleQueueSuggestedActionsKey
// toast "no suggested actions" rather than opening an empty menu).
func suggestedActionItems(status tickets.RenderedStatus) []components.MenuItem {
	if status != tickets.StatusNeedsAnswer {
		return nil
	}
	return []components.MenuItem{
		{Label: "Resume (I answered)", Value: actionResumeAnswered},
	}
}

// ticketHasSuggestedActions reports whether status's row should carry the "m"
// suggested-actions badge (ui.IconSet.SuggestedAction).
func ticketHasSuggestedActions(status tickets.RenderedStatus) bool {
	return len(suggestedActionItems(status)) > 0
}

// applySuggestedAction performs action's write against the ticket at path.
func applySuggestedAction(path, action string, now time.Time) error {
	switch action {
	case actionResumeAnswered:
		return ralphloop.UnparkTicket(path, now)
	}
	return nil
}
