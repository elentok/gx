package sched

import (
	"errors"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/events"
	"github.com/elentok/gx/herdr"
)

// markedIterationReason is the iteration-error pause reason for err: the full
// text, prefixed with the marker of the typed cause when there is one, so Chat
// can word it plainly while the ticket file keeps the whole reason.
func markedIterationReason(err error, text string) string {
	var lost *herdr.AgentNotFoundError
	switch {
	case errors.Is(err, agentrunner.ErrNotDelivered):
		return events.StallNudgeMarker.Reason(text)
	case errors.Is(err, agentrunner.ErrSessionDied):
		return events.SessionDiedMarker.Reason(text)
	case errors.As(err, &lost):
		return events.HerdrLostMarker.Reason(text)
	}
	return text
}
