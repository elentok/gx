package ticketwrite

import (
	"fmt"

	"github.com/elentok/gx/events"
	"github.com/elentok/gx/tickets"
)

// PauseChatDetail is the chat line for a pause. A self-reported pause shows the
// agent's own question summary, which says more than the generic reason. It
// falls back to the generic line when the ticket has no open question.
func PauseChatDetail(ticketPath string, kind events.Kind, reason string) string {
	if kind == events.SelfReported && ticketPath != "" {
		if evs, err := tickets.ReadLog(ticketPath); err == nil {
			if q := tickets.OpenQuestion(evs); q != nil && q.Summary != "" {
				return q.Summary
			}
		}
	}
	return fmt.Sprintf("%s: %s", kind, reason)
}
