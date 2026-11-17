package sched

import (
	"errors"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/events"
	"github.com/elentok/gx/git"
)

// markedIterationReason is the iteration-error pause reason for err: the full
// text, prefixed with the marker of the typed cause when there is one, so Chat
// can word it plainly while the ticket file keeps the whole reason.
func markedIterationReason(err error, text string) string {
	var exists *git.WorktreeExistsError
	switch {
	case errors.As(err, &exists):
		return events.WorktreeExists.Reason(text)
	case errors.Is(err, agentrunner.ErrNotDelivered):
		return events.StallNudgeMarker.Reason(text)
	case errors.Is(err, agentrunner.ErrSessionDied):
		return events.SessionDiedMarker.Reason(text)
	}
	return text
}
