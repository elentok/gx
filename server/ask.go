package server

import (
	"fmt"
	"strings"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// Refusal reasons of the ask verb.
const (
	ReasonInvalidQuestion = "invalid-question"
	ReasonHandoffRequired = "handoff-required"
	ReasonQuestionOpen    = "question-open"
)

const askAuthorAgent = "agent"

// ticketAsk records an agent's question: it appends one pause.question to the
// ticket's log and sets iteration_status asking. The body is validated
// strictly first, and any refusal writes nothing. It returns at once; pausing
// the ticket is a later step.
func (s *Server) ticketAsk(req QueueRequest) (QueueResult, error) {
	q, err := tickets.ParseQuestionStrict(req.Text)
	if err != nil {
		return refusal(ReasonInvalidQuestion, err.Error()), nil
	}
	if strings.TrimSpace(q.Handoff) == "" {
		return refusal(ReasonHandoffRequired, "a non-empty ## Handoff section is required"), nil
	}
	return s.resolvedWrite(req, func(ref ticketRef) (QueueResult, error) {
		ev := tickets.LogEvent{
			Type:    tickets.EventParkQuestion,
			By:      askAuthorAgent,
			Summary: q.Summary,
			Text:    q.Detail,
			Block:   tickets.QuestionBlock(req.Text),
			Handoff: q.Handoff,
		}
		_, err := tickets.AppendLogIf(ref.ticket.Path, ev, func(events []tickets.LogEvent) error {
			if tickets.OpenQuestion(events) != nil {
				return errQuestionOpen
			}
			return nil
		})
		if err == errQuestionOpen {
			return refusal(ReasonQuestionOpen, ref.addr.String()+" already has an open question"), nil
		}
		if err != nil {
			return QueueResult{}, fmt.Errorf("ask %s: %w", ref.addr, err)
		}
		err = schema.UpdateTicket(ref.ticket.Path, func(t *schema.Ticket) {
			t.IterationStatus = schema.IterationStatusAsking
		})
		if err != nil {
			return QueueResult{}, fmt.Errorf("ask %s: setting iteration_status: %w", ref.addr, err)
		}
		s.rescan()
		return QueueResult{}, nil
	})
}

var errQuestionOpen = fmt.Errorf("open question")
