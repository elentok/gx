package server

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// Refusal reasons of the answer verb.
const (
	ReasonStale         = "stale"
	ReasonNotPaused     = "not-paused"
	ReasonNotAnswerable = "not-answerable"
	ReasonBadChoice     = "bad-choice"
	ReasonEmptyAnswer   = "empty-answer"
	ReasonTooLong       = "too-long"
)

// Outcomes of the answer verb.
const (
	OutcomeDelivered = "delivered"
	OutcomeAdopted   = "adopted"
	OutcomeQueued    = "queued"
)

// maxAnswerChars caps a note, and the text of a free-text answer.
const maxAnswerChars = 4000

// answerRefused carries a refusal out of the log guard, which holds the ticket
// lock: the recheck, the prompt and the log line are one step.
type answerRefused struct{ res QueueResult }

func (e answerRefused) Error() string { return e.res.Message }

// storedStatus is t's status in the old stored spelling: a paused ticket is
// asking or stuck by class. The web API and pause events keep
// those labels until the events are renamed.
func storedStatus(t schema.Ticket) schema.Status {
	if t.Status == schema.StatusPaused {
		return t.PausedClass().LegacyStatus()
	}
	return t.Status
}

// answerState is the token a client sends back: it covers the latest
// question's seq and the ticket's pause class, so an answer to a question that
// was replaced, closed or re-paused is stale.
func answerState(q tickets.LogEvent, class schema.PauseClass) string {
	return fmt.Sprintf("%d:%s", q.Seq, class)
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
// state token, the prompt and the pause.answer line are one step under the
// ticket lock, so a refusal logs nothing and a stale reply never reaches the
// agent.
func (s *Server) ticketAnswer(req QueueRequest) (QueueResult, error) {
	return s.resolvedWrite(req, func(ref ticketRef) (QueueResult, error) {
		if s.adoptFirst(ref.projectDir, ref.addr, ref.ticket.Path) {
			return QueueResult{Outcome: OutcomeAdopted}, nil
		}
		address := ref.addr.String()
		var run trackedRun
		var queued bool
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
			case t.PausedClass() == "":
				return refuse(refusal(ReasonNotPaused, address+" is not paused"))
			case t.PausedClass() == schema.PauseClassRepair || t.PauseReason != schema.PauseSelfReported:
				return refuse(refusal(ReasonNotAnswerable, address+" was not paused by a question; use resume"))
			case open == nil:
				return refuse(refusal(ReasonNotPaused, address+" has no open question"))
			}
			state := answerState(*open, t.PausedClass())
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
			if queued, err = s.agentGone(address); err != nil {
				return tickets.LogEvent{}, err
			}
			if !queued {
				// The budget check guards live delivery only: a queued answer
				// launches nothing.
				if res, refused := s.budgetRefusal(); refused {
					return refuse(res)
				}
				run, _ = s.registry.tracked(address)
				runner, err := s.runnerOf(run.Run)
				if err != nil {
					return tickets.LogEvent{}, err
				}
				err = runner.Prompt(run.Session, answerFrame(open.Seq, answer, note))
				switch {
				case errors.Is(err, agentrunner.ErrNotReady):
					return refuse(refusal(ReasonAgentNotReady, err.Error()))
				case errors.Is(err, agentrunner.ErrNotFound), errors.Is(err, agentrunner.ErrSessionDied):
					queued = true
				case err != nil:
					return tickets.LogEvent{}, fmt.Errorf("prompt %s: %w", address, err)
				default:
					// The agent can die right after taking the prompt.
					if queued, err = s.agentGone(address); err != nil {
						return tickets.LogEvent{}, err
					}
				}
			}
			ev := tickets.LogEvent{Type: tickets.EventPauseAnswer, Source: req.Source, How: tickets.HowAnswer, Question: open.Seq, Choice: answer, Note: note}
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
		if queued {
			// Open again for gx to claim; the dead run has nothing to settle.
			s.registry.delete(address)
			if err := ralphloop.ResumeTicket(ref.ticket.Path, time.Now()); err != nil {
				return QueueResult{}, fmt.Errorf("reopen %s: %w", ref.addr, err)
			}
			s.rescan()
			return QueueResult{Outcome: OutcomeQueued}, nil
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

// agentGone reports that no live agent is waiting on address: its run is not
// tracked as waiting, or its session is missing or died.
func (s *Server) agentGone(address string) (bool, error) {
	run, ok := s.registry.tracked(address)
	if !ok || run.WaitingSince.IsZero() {
		return true, nil
	}
	runner, err := s.runnerOf(run.Run)
	if err != nil {
		return false, err
	}
	_, err = runner.Status(run.Session)
	if errors.Is(err, agentrunner.ErrNotFound) || errors.Is(err, agentrunner.ErrSessionDied) {
		return true, nil
	}
	return false, err
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
