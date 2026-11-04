package server

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/elentok/gx/server/queue"
	"github.com/elentok/gx/server/runs"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// maxAnswerChars caps a note, and the text of a free-text answer.
const maxAnswerChars = 4000

// answerRefused carries a refusal out of the log guard, which holds the ticket
// lock: the recheck, the prompt and the log line are one step.
type answerRefused struct{ res wire.QueueResult }

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

// detailState is the token the detail page carries. A paused ticket's token
// names its latest round of any kind, so repair and manual pauses have one.
func detailState(evs []tickets.LogEvent, t schema.Ticket) string {
	rounds := tickets.Rounds(evs)
	if t.PausedClass() != "" {
		rounds = tickets.AllRounds(evs)
	}
	if len(rounds) == 0 {
		return ""
	}
	return answerState(rounds[len(rounds)-1].Question, t.PausedClass())
}

// pauseStale reports whether state no longer names t's open pause round.
func pauseStale(evs []tickets.LogEvent, t schema.Ticket, state string) bool {
	open := tickets.OpenRound(evs)
	return t.PausedClass() == "" || open == nil || answerState(*open, t.PausedClass()) != state
}

// ticketPauseStale is pauseStale for the ticket stored at ticketPath.
func ticketPauseStale(ticketPath, state string) (bool, error) {
	t, err := schema.ParseTicket(ticketPath)
	if err != nil {
		return false, err
	}
	evs, err := tickets.ReadLog(ticketPath)
	if err != nil {
		return false, fmt.Errorf("read log of %s: %w", ticketPath, err)
	}
	return pauseStale(evs, t, state), nil
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
func validateAnswer(q tickets.Question, req wire.QueueRequest) (answer, note string, bad *wire.QueueResult) {
	note = strings.TrimSpace(normalizeAnswer(req.Note))
	switch q.Kind {
	case tickets.QuestionText:
		answer, note = strings.TrimSpace(normalizeAnswer(req.Text)), ""
		if req.Choice != "" {
			r := queue.Refusal(wire.ReasonBadChoice, "this question takes free text, not a choice")
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
			r := queue.Refusal(wire.ReasonBadChoice, fmt.Sprintf("choice %q is not one of %s", answer, strings.Join(valid, ", ")))
			return "", "", &r
		}
	}
	if answer == "" {
		r := queue.Refusal(wire.ReasonEmptyAnswer, "an answer is required")
		return "", "", &r
	}
	if utf8.RuneCountInString(answer) > maxAnswerChars || utf8.RuneCountInString(note) > maxAnswerChars {
		r := queue.Refusal(wire.ReasonTooLong, fmt.Sprintf("an answer or note is limited to %d characters", maxAnswerChars))
		return "", "", &r
	}
	return answer, note, nil
}

// ticketAnswer delivers a person's reply to the live agent that asked, then
// claims the ticket again and settles the run when the agent finishes. The
// state token, the prompt and the pause.answer line are one step under the
// ticket lock, so a refusal logs nothing and a stale reply never reaches the
// agent.
func (s *Server) ticketAnswer(req wire.QueueRequest) (wire.QueueResult, error) {
	return s.resolvedWrite(req, func(ref ticketRef) (wire.QueueResult, error) {
		if s.adoptFirst(ref.projectDir, ref.addr, ref.ticket.Path) {
			return wire.QueueResult{Outcome: wire.OutcomeAdopted}, nil
		}
		address := ref.addr.String()
		var run runs.TrackedRun
		var queued bool
		_, err := tickets.AppendLogFrom(ref.ticket.Path, func(evs []tickets.LogEvent) (tickets.LogEvent, error) {
			t, err := schema.ParseTicket(ref.ticket.Path)
			if err != nil {
				return tickets.LogEvent{}, err
			}
			refuse := func(res wire.QueueResult) (tickets.LogEvent, error) {
				return tickets.LogEvent{}, answerRefused{res}
			}
			open := tickets.OpenQuestion(evs)
			switch {
			case t.PausedClass() == "":
				return refuse(queue.Refusal(wire.ReasonNotPaused, address+" is not paused"))
			case t.PausedClass() == schema.PauseClassRepair || t.PauseReason != schema.PauseSelfReported:
				return refuse(queue.Refusal(wire.ReasonNotAnswerable, address+" was not paused by a question; use resume"))
			case open == nil:
				return refuse(queue.Refusal(wire.ReasonNotPaused, address+" has no open question"))
			}
			state := answerState(*open, t.PausedClass())
			if req.State != "" && req.State != state {
				res := queue.Refusal(wire.ReasonStale, "the question changed; answer the current one")
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
				run, _ = s.registry.Tracked(address)
				runner, err := s.runnerOf(run.Run)
				if err != nil {
					return tickets.LogEvent{}, err
				}
				err = runner.Prompt(run.Session, answerFrame(open.Seq, answer, note))
				switch {
				case errors.Is(err, agentrunner.ErrNotReady):
					return refuse(queue.Refusal(wire.ReasonAgentNotReady, err.Error()))
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
			return wire.QueueResult{}, fmt.Errorf("answer %s: %w", ref.addr, err)
		}
		if err := s.cancelInvestigateChild(ref, req.Source); err != nil {
			return wire.QueueResult{}, err
		}
		if queued {
			// Open again for gx to claim; the dead run has nothing to settle.
			s.registry.Delete(address)
			if err := ralphloop.ResumeTicket(ref.ticket.Path, time.Now()); err != nil {
				return wire.QueueResult{}, fmt.Errorf("reopen %s: %w", ref.addr, err)
			}
			s.idx.Rescan()
			return wire.QueueResult{Outcome: wire.OutcomeQueued}, nil
		}
		if err := ralphloop.Claim(ref.ticket.Path); err != nil {
			return wire.QueueResult{}, fmt.Errorf("claim %s: %w", ref.addr, err)
		}
		if err := s.resettleWaiting(run); err != nil {
			return wire.QueueResult{}, fmt.Errorf("settle %s: %w", ref.addr, err)
		}
		s.idx.Rescan()
		return wire.QueueResult{Outcome: wire.OutcomeDelivered}, nil
	})
}

// agentGone reports that no live agent is waiting on address: its run is not
// tracked as waiting, or its session is missing or died.
func (s *Server) agentGone(address string) (bool, error) {
	run, ok := s.registry.Tracked(address)
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
func (s *Server) resettleWaiting(t runs.TrackedRun) error {
	runner, err := s.runnerOf(t.Run)
	if err != nil {
		return err
	}
	one, wt, mode, err := s.resume(t)
	if err != nil {
		return err
	}
	root, err := queue.ParseRootRef(t.Root)
	if err != nil {
		return err
	}
	s.registry.ClearWaiting(t.Address)
	deps := s.projects.DepsWith(runner)
	deps.GateReleased = s.registry.GateReleased(t.Address)
	s.guards.GoSettle(func() { s.finishRun(runContext{deps, root, mode, one, wt, t.Run, t.Address}) })
	return nil
}
