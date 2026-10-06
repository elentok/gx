package repair

import (
	"fmt"
	"time"

	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/tickets"
)

// UnparkResult is the --json success payload of an unpark.
type UnparkResult struct {
	Ticket string `json:"ticket"`
	Status string `json:"status"`
}

// Unpark reopens the needs-answer ticket id in the epic at epicPath, retiring
// its Needs Answer section into Comments. It is the same write the
// tickets/queue tabs' suggested-actions menu performs.
func Unpark(epicPath, id string, now time.Time) (UnparkResult, error) {
	res := UnparkResult{Ticket: id, Status: "open"}
	epic, t, err := FindEpicTicket(epicPath, id)
	if err != nil {
		return res, err
	}
	if status := epic.RenderedStatus(t); status != tickets.StatusNeedsAnswer {
		return res, &RefusalError{Reason: ReasonNotParked, Message: fmt.Sprintf("ticket %s is %v, not needs-answer", id, status)}
	}
	return res, ralphloop.UnparkTicket(t.Path, now)
}
