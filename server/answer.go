package server

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// Refusal reasons of the answer verb.
const (
	ReasonStale         = "stale"
	ReasonNotParked     = "not-parked"
	ReasonNotAnswerable = "not-answerable"
	ReasonBadChoice     = "bad-choice"
	ReasonEmptyAnswer   = "empty-answer"
	ReasonTooLong       = "too-long"
)

// Outcomes of the answer verb.
const (
	OutcomeDelivered = "delivered"
	OutcomeAdopted   = "adopted"
)

// maxAnswerChars caps a note, and the text of a free-text answer.
const maxAnswerChars = 4000

// answerRefused carries a refusal out of the log guard, which holds the ticket
// lock: the recheck, the prompt and the log line are one step.
type answerRefused struct{ res QueueResult }

func (e answerRefused) Error() string { return e.res.Message }

// answerState is the token a client sends back: it covers the latest
// question's seq and the ticket's status, so an answer to a question that was
// replaced, closed or re-parked is stale.
func answerState(q tickets.LogEvent, status schema.Status) string {
	return fmt.Sprintf("%d:%s", q.Seq, status)
}

// normalizeAnswer turns every line ending into \n so a CR cannot submit the
// prompt early.
func normalizeAnswer(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\r", "\n")
}

// answerFrame is the prompt the live agent receives: the choice first, the note
// under it. A free-text answer prints the text and no note.
func answerFrame(seq int, answer, note string) string {
	frame := fmt.Sprintf("Reply from the user to your question %d: %s", seq, answer)
	if note != "" {
		frame += "\n" + note
	}
	return frame
}

// validateAnswer checks the answer against the question's kind and returns the
// text to deliver and log: the choice (or the free text) and the note.
func validateAnswer(q tickets.Question, req QueueRequest) (answer, note string, bad *QueueResult) {
	note = strings.TrimSpace(normalizeAnswer(req.Note))
	switch q.Kind {
	case tickets.QuestionText:
		answer, note = strings.TrimSpace(normalizeAnswer(req.Text)), ""
		if req.Choice != "" {
			r := refusal(ReasonBadChoice, "this question takes free text, not a choice")
			return "", "", &r
		}
	default:
		answer = strings.TrimSpace(req.Choice)
		valid := []string{"yes", "no"}
		if q.Kind == tickets.QuestionOptions {
			valid = nil
			for _, o := range q.Options {
				valid = append(valid, o.ID)
			}
		}
		if answer != "" && !slices.Contains(valid, answer) {
			r := refusal(ReasonBadChoice, fmt.Sprintf("choice %q is not one of %s", answer, strings.Join(valid, ", ")))
			return "", "", &r
		}
	}
	if answer == "" {
		r := refusal(ReasonEmptyAnswer, "an answer is required")
		return "", "", &r
	}
	if utf8.RuneCountInString(answer) > maxAnswerChars || utf8.RuneCountInString(note) > maxAnswerChars {
		r := refusal(ReasonTooLong, fmt.Sprintf("an answer or note is limited to %d characters", maxAnswerChars))
		return "", "", &r
	}
	return answer, note, nil
}

// ticketAnswer delivers a person's reply to the live agent that asked, then
// claims the ticket again and settles the run when the agent finishes. The
// state token, the prompt and the park.answer line are one step under the
// ticket lock, so a refusal logs nothing and a stale reply never reaches the
// agent.
func (s *Server) ticketAnswer(req QueueRequest) (QueueResult, error) {
	return s.resolvedWrite(req, func(ref ticketRef) (QueueResult, error) {
		if s.adoptFirst(ref.projectDir, ref.addr, ref.ticket.Path) {
			return QueueResult{Outcome: OutcomeAdopted}, nil
		}
		address := ref.addr.String()
		var run trackedRun
		_, err := tickets.AppendLogFrom(ref.ticket.Path, func(evs []tickets.LogEvent) (tickets.LogEvent, error) {
			t, err := schema.ParseTicket(ref.ticket.Path)
			if err != nil {
				return tickets.LogEvent{}, err
			}
			refuse := func(res QueueResult) (tickets.LogEvent, error) {
				return tickets.LogEvent{}, answerRefused{res}
			}
			open := tickets.OpenQuestion(evs)
			switch {
			case t.Status != schema.StatusNeedsAnswer && t.Status != schema.StatusNeedsRepair:
				return refuse(refusal(ReasonNotParked, address+" is not parked"))
			case t.Status == schema.StatusNeedsRepair || t.ParkKind != schema.ParkKindSelfReported:
				return refuse(refusal(ReasonNotAnswerable, address+" was not parked by a question; use unpark"))
			case open == nil:
				return refuse(refusal(ReasonNotParked, address+" has no open question"))
			}
			state := answerState(*open, t.Status)
			if req.State != "" && req.State != state {
				res := refusal(ReasonStale, "the question changed; answer the current one")
				res.Question, res.State = open, state
				return refuse(res)
			}
			q := tickets.QuestionOfEvent(*open)
			answer, note, bad := validateAnswer(q, req)
			if bad != nil {
				return refuse(*bad)
			}
			if res, refused := s.budgetRefusal(); refused {
				return refuse(res)
			}
			var ok bool
			if run, ok = s.registry.tracked(address); !ok || run.WaitingSince.IsZero() {
				return refuse(refusal(ReasonIterationNotLive, "no live agent is waiting on "+address))
			}
			runner, err := s.runnerOf(run.Run)
			if err != nil {
				return tickets.LogEvent{}, err
			}
			if err := runner.Prompt(run.Session, answerFrame(open.Seq, answer, note)); err != nil {
				if errors.Is(err, agentrunner.ErrNotReady) {
					return refuse(refusal(ReasonAgentNotReady, err.Error()))
				}
				return tickets.LogEvent{}, fmt.Errorf("prompt %s: %w", address, err)
			}
			ev := tickets.LogEvent{Type: tickets.EventParkAnswer, Source: req.Source, Question: open.Seq, Choice: answer, Note: note}
			if q.Kind == tickets.QuestionText {
				ev.Choice, ev.Text = "", answer
			}
			return ev, nil
		})
		var refused answerRefused
		if errors.As(err, &refused) {
			return refused.res, nil
		}
		if err != nil {
			return QueueResult{}, fmt.Errorf("answer %s: %w", ref.addr, err)
		}
		if err := ralphloop.Claim(ref.ticket.Path); err != nil {
			return QueueResult{}, fmt.Errorf("claim %s: %w", ref.addr, err)
		}
		if err := s.resettleWaiting(run); err != nil {
			return QueueResult{}, fmt.Errorf("settle %s: %w", ref.addr, err)
		}
		s.rescan()
		return QueueResult{Outcome: OutcomeDelivered}, nil
	})
}

// resettleWaiting ends a run's wait for an answer and settles it as a normal
// run: the run context is rebuilt from the registry entry, as a restart does.
func (s *Server) resettleWaiting(t trackedRun) error {
	runner, err := s.runnerOf(t.Run)
	if err != nil {
		return err
	}
	one, wt, mode, err := s.resume(t)
	if err != nil {
		return err
	}
	root, err := parseRootRef(t.Root)
	if err != nil {
		return err
	}
	s.registry.clearWaiting(t.Address)
	deps := s.depsWith(runner)
	deps.GateReleased = s.registry.gateReleased(t.Address)
	s.goSettle(func() { s.finishRun(runContext{deps, root, mode, one, wt, t.Run, t.Address}) })
	return nil
}
