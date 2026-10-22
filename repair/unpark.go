package repair

import (
	"fmt"
	"time"

	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/tickets"
)

// ResumeResult is the --json success payload of a resume. Message is set for
// a zero-commit pause, where the person needs to know what the resume did.
type ResumeResult struct {
	Ticket  string `json:"ticket"`
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

// ResumeAdopted is the Status of a resume the server answered by adopting
// the agent's late finish instead of reopening the ticket.
const ResumeAdopted = "adopted"

// ResumePrompted is the Status of a resume that sent a live agent the
// continue prompt and left the ticket paused.
const ResumePrompted = "prompted"

// Messages of a zero-commit resume.
const (
	MessageAdopted  = "adopted as done"
	MessageReopened = "reopened"
	MessagePrompted = "the agent is still running and was asked to continue"
)

// Resume reopens the asking ticket id in the epic at epicPath, retiring
// its question into the log. A non-empty base is written onto the
// ticket. It is the same write the tickets/queue tabs' suggested-actions menu
// performs.
func Resume(epicPath, id string, now time.Time, base string) (ResumeResult, error) {
	res := ResumeResult{Ticket: id, Status: "open"}
	epic, t, err := FindEpicTicket(epicPath, id)
	if err != nil {
		return res, err
	}
	if status := epic.RenderedStatus(t); status != tickets.StatusPausedAnswer {
		return res, &RefusalError{Reason: ReasonNotPaused, Message: fmt.Sprintf("ticket %s is %v, not asking", id, status)}
	}
	return res, ralphloop.ResumeTicketWithBase(t.Path, now, base)
}
