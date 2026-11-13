package ticketops

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/backgroundtask"
	"github.com/elentok/gx/ralphloop/agent"
	"github.com/elentok/gx/ralphloop/ticketops"
	"github.com/elentok/gx/server/queue"
	"github.com/elentok/gx/server/runs"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/syshealth"
	"github.com/elentok/gx/tickets/schema"
	"github.com/elentok/gx/tickets/ticketlog"
)

// maxAnswerChars caps a note, and the text of a free-text answer.
const maxAnswerChars = 4000

// answerRefused carries a refusal out of the log guard, which holds the ticket
// lock: the recheck, the prompt and the log line are one step.
type answerRefused struct{ res wire.QueueResult }

func (e answerRefused) Error() string { return e.res.Message }

// answerFrame is the prompt the live agent receives: the choice first, the note
// under it. A free-text answer prints the text and no note.
func answerFrame(seq int, answer, note string) string {
	frame := fmt.Sprintf("Reply from the user to your question %d: %s", seq, answer)
	if note != "" {
		frame += "\n" + note
	}
	return frame
}

// yesNoReply words a yes-no choice as "Yes - <yes line>", so the agent reads
// the consequence the person agreed to, not a bare Yes that fits two readings.
// Other kinds, and a yes-no question with no consequence line, pass through.
func yesNoReply(q ticketlog.Question, answer string) string {
	if answer == "" {
		return wire.NoOptionPicked
	}
	if q.Kind != ticketlog.QuestionYesNo {
		return answer
	}
	label, line := "Yes", q.Yes
	if answer == "no" {
		label, line = "No", q.No
	}
	if line == "" {
		return label
	}
	return label + " - " + line
}

// validateAnswer checks the answer against the question's kind and returns the
// text to deliver and log: the choice (or the free text) and the note.
func validateAnswer(q ticketlog.Question, req wire.QueueRequest) (answer, note string, bad *wire.QueueResult) {
	note = strings.TrimSpace(wire.NormalizeAnswer(req.Note))
	if req.Huh {
		// Huh? stands in for the answer on any kind; free text takes no note.
		if q.Kind == ticketlog.QuestionText {
			note = ""
		}
		if utf8.RuneCountInString(note) > maxAnswerChars {
			r := queue.Refusal(wire.ReasonTooLong, fmt.Sprintf("an answer or note is limited to %d characters", maxAnswerChars))
			return "", "", &r
		}
		return wire.HuhMessage, note, nil
	}
	switch q.Kind {
	case ticketlog.QuestionText:
		answer, note = strings.TrimSpace(wire.NormalizeAnswer(req.Text)), ""
		if req.Choice != "" {
			r := queue.Refusal(wire.ReasonBadChoice, "this question takes free text, not a choice")
			return "", "", &r
		}
	default:
		answer = strings.TrimSpace(req.Choice)
		valid := []string{"yes", "no"}
		if q.Kind == ticketlog.QuestionOptions {
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
	// A choice question may be answered by a note alone; free text needs its text.
	if answer == "" && (q.Kind == ticketlog.QuestionText || note == "") {
		r := queue.Refusal(wire.ReasonEmptyAnswer, "an answer is required")
		return "", "", &r
	}
	if utf8.RuneCountInString(answer) > maxAnswerChars || utf8.RuneCountInString(note) > maxAnswerChars {
		r := queue.Refusal(wire.ReasonTooLong, fmt.Sprintf("an answer or note is limited to %d characters", maxAnswerChars))
		return "", "", &r
	}
	return answer, note, nil
}

// Answer delivers a person's reply to the live agent that asked, then
// claims the ticket again and settles the run when the agent finishes. The
// state token, the prompt and the pause.answer line are one step under the
// ticket lock, so a refusal logs nothing and a stale reply never reaches the
// agent.
func (o *Ops) Answer(req wire.QueueRequest) (wire.QueueResult, error) {
	return o.d.Writer.ResolvedWrite(req, func(ref queue.TicketRef) (wire.QueueResult, error) {
		if o.d.Rec.AdoptFirst(ref.ProjectDir, ref.Addr, ref.Ticket.Path) {
			return wire.QueueResult{Outcome: wire.OutcomeAdopted}, nil
		}
		address := ref.Addr.String()
		var run runs.TrackedRun
		var queued bool
		_, err := ticketlog.AppendLogFrom(ref.Ticket.Path, func(evs []ticketlog.LogEvent) (ticketlog.LogEvent, error) {
			t, err := schema.ParseTicket(ref.Ticket.Path)
			if err != nil {
				return ticketlog.LogEvent{}, err
			}
			refuse := func(res wire.QueueResult) (ticketlog.LogEvent, error) {
				return ticketlog.LogEvent{}, answerRefused{res}
			}
			open := ticketlog.OpenQuestion(evs)
			switch {
			case t.PausedClass() == "":
				return refuse(queue.Refusal(wire.ReasonNotPaused, address+" is not paused"))
			case t.PausedClass() == schema.PauseClassRepair || t.PauseReason != schema.PauseSelfReported:
				return refuse(queue.Refusal(wire.ReasonNotAnswerable, address+" was not paused by a question; use resume"))
			case open == nil:
				return refuse(queue.Refusal(wire.ReasonNotPaused, address+" has no open question"))
			}
			state := queue.AnswerState(*open, t.PausedClass())
			if req.State != "" && req.State != state {
				res := queue.Refusal(wire.ReasonStale, "the question changed; answer the current one")
				res.Question, res.State = open, state
				return refuse(res)
			}
			q := ticketlog.QuestionOfEvent(*open)
			answer, note, bad := validateAnswer(q, req)
			if bad != nil {
				return refuse(*bad)
			}
			if queued, err = o.AgentGone(address); err != nil {
				return ticketlog.LogEvent{}, err
			}
			if !queued {
				// The budget check guards live delivery only: a queued answer
				// launches nothing.
				if res, refused := o.d.Launch.BudgetRefusal(); refused {
					return refuse(res)
				}
				run, _ = o.d.Registry.Tracked(address)
				runner, err := o.d.Harness.RunnerOf(run.Run)
				if err != nil {
					return ticketlog.LogEvent{}, err
				}
				reply := yesNoReply(q, answer)
				if req.Huh {
					reply = answer
				}
				err = runner.Prompt(run.Session, answerFrame(open.Seq, reply, note))
				switch {
				case errors.Is(err, agentrunner.ErrNotReady):
					return refuse(queue.Refusal(wire.ReasonAgentNotReady, err.Error()))
				case errors.Is(err, agentrunner.ErrNotFound), errors.Is(err, agentrunner.ErrSessionDied):
					queued = true
				case err != nil:
					return ticketlog.LogEvent{}, fmt.Errorf("prompt %s: %w", address, err)
				default:
					// The agent can die right after taking the prompt.
					if queued, err = o.AgentGone(address); err != nil {
						return ticketlog.LogEvent{}, err
					}
				}
			}
			ev := ticketlog.LogEvent{Type: ticketlog.EventPauseAnswer, Source: req.Source, How: ticketlog.HowAnswer, Question: open.Seq, Choice: answer, Note: note, Huh: req.Huh}
			if q.Kind == ticketlog.QuestionText {
				ev.Choice, ev.Text = "", answer
			}
			if req.Huh {
				ev.Choice = ""
			}
			return ev, nil
		})
		var refused answerRefused
		if errors.As(err, &refused) {
			return refused.res, nil
		}
		if err != nil {
			return wire.QueueResult{}, fmt.Errorf("answer %s: %w", ref.Addr, err)
		}
		if err := o.CancelInvestigateChild(ref, req.Source); err != nil {
			return wire.QueueResult{}, err
		}
		if queued {
			// Open again for gx to claim; the dead run has nothing to settle.
			o.d.Registry.Delete(address)
			if err := ticketops.ResumeTicket(ref.Ticket.Path, time.Now()); err != nil {
				return wire.QueueResult{}, fmt.Errorf("reopen %s: %w", ref.Addr, err)
			}
			o.d.Index.Rescan()
			res := wire.QueueResult{Outcome: wire.OutcomeQueued}
			if o.d.Queue != nil {
				res.Queue = o.d.Queue.List()
			}
			return res, nil
		}
		if err := ticketops.Claim(ref.Ticket.Path); err != nil {
			return wire.QueueResult{}, fmt.Errorf("claim %s: %w", ref.Addr, err)
		}
		if err := o.d.Launch.ResettleWaiting(run); err != nil {
			return wire.QueueResult{}, fmt.Errorf("settle %s: %w", ref.Addr, err)
		}
		o.d.Index.Rescan()
		return wire.QueueResult{Outcome: wire.OutcomeDelivered}, nil
	})
}

// LiveTask reads the agent's background-task signal fresh from its runner. live
// is false when the run or its session is gone. stoppable is set for Claude, the
// only agent whose task gx can stop; a Codex task reports as working.
func (o *Ops) LiveTask(address string) (live, stoppable bool, err error) {
	run, ok := o.d.Registry.Tracked(address)
	if !ok {
		return false, false, nil
	}
	runner, err := o.d.Harness.RunnerOf(run.Run)
	if err != nil {
		return false, false, err
	}
	st, err := runner.Status(run.Session)
	if errors.Is(err, agentrunner.ErrNotFound) || errors.Is(err, agentrunner.ErrSessionDied) {
		return false, false, nil
	}
	if err != nil {
		return false, false, err
	}
	live = st.Background
	return live, live && agent.AgentFor(run.Agent) == agent.AgentClaude, nil
}

// TaskCard reads the facts of the live task of address's Claude agent for its
// gate card, or nil when there is no run, no session id or no outstanding task.
func (o *Ops) TaskCard(address string) (*wire.BackgroundTaskCard, error) {
	run, ok := o.d.Registry.Tracked(address)
	if !ok || run.Session.SessionID == "" || agent.AgentFor(run.Agent) != agent.AgentClaude {
		return nil, nil
	}
	home, err := o.d.Projects.EnvHome()
	if err != nil {
		return nil, err
	}
	rd := &backgroundtask.Reader{Home: home}
	f, ok := rd.Facts(run.Session.SessionID, backgroundtask.DefaultOutputBase(), backgroundtask.ProcessFinder{Procs: syshealth.DefaultProcess()})
	if !ok {
		return nil, nil
	}
	card := &wire.BackgroundTaskCard{Label: f.Label, StartedAt: f.StartedAt, Alive: f.Process.Alive, LastLines: f.Process.LastLines}
	if card.LastLines == nil {
		card.LastLines = []string{}
	}
	if !f.Process.LastOutput.IsZero() {
		card.LastOutputAt = &f.Process.LastOutput
	}
	return card, nil
}

// AgentGone reports that no live agent is waiting on address: its run is not
// tracked as waiting, or its session is missing or died.
func (o *Ops) AgentGone(address string) (bool, error) {
	run, ok := o.d.Registry.Tracked(address)
	if !ok || run.WaitingSince.IsZero() {
		return true, nil
	}
	runner, err := o.d.Harness.RunnerOf(run.Run)
	if err != nil {
		return false, err
	}
	_, err = runner.Status(run.Session)
	if errors.Is(err, agentrunner.ErrNotFound) || errors.Is(err, agentrunner.ErrSessionDied) {
		return true, nil
	}
	return false, err
}
