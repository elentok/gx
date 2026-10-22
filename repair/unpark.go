package repair

import (
	"fmt"
	"time"

	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/tickets"
)

// UnparkResult is the --json success payload of an unpark. Message is set for
// a zero-commit park, where the person needs to know what the unpark did.
type UnparkResult struct {
	Ticket  string `json:"ticket"`
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

// UnparkAdopted is the Status of an unpark the server answered by adopting
// the agent's late finish instead of reopening the ticket.
const UnparkAdopted = "adopted"

// UnparkPrompted is the Status of an unpark that sent a live agent the
// continue prompt and left the ticket parked.
const UnparkPrompted = "prompted"

// Messages of a zero-commit unpark.
const (
	MessageAdopted  = "adopted as done"
	MessageReopened = "reopened"
	MessagePrompted = "the agent is still running and was asked to continue"
)

// Unpark reopens the needs-answer ticket id in the epic at epicPath, retiring
// its Needs Answer section into Comments. A non-empty base is written onto the
// ticket. It is the same write the tickets/queue tabs' suggested-actions menu
// performs.
func Unpark(epicPath, id string, now time.Time, base string) (UnparkResult, error) {
	res := UnparkResult{Ticket: id, Status: "open"}
	epic, t, err := FindEpicTicket(epicPath, id)
	if err != nil {
		return res, err
	}
	if status := epic.RenderedStatus(t); status != tickets.StatusPausedAnswer {
		return res, &RefusalError{Reason: ReasonNotParked, Message: fmt.Sprintf("ticket %s is %v, not needs-answer", id, status)}
	}
	return res, ralphloop.UnparkTicketWithBase(t.Path, now, base)
}
