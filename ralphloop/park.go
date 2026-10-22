package ralphloop

import (
	"errors"
	"fmt"
	"log"

	"github.com/elentok/gx/events"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// parkRequest describes one park outcome. Kind is mandatory: every park event
// carries the failure fields (kind, address, reason) so a catalog can match it.
type parkRequest struct {
	ScratchDir string
	EpicName   string
	Ticket     string // identifier
	Path       string // ticket file
	Type       events.Type
	Kind       events.Kind
	Reason     string
	// Repair is rendered into the ticket's "## Needs Repair" section.
	Repair schema.FaultState
	// Event carries optional agent context (runner, label, pane, session, cwd) merged
	// into the appended event; park overwrites its type, ticket, kind and reason.
	Event Event
	// AlreadyDone skips the ticket write of a manual-land whose ticket was
	// already done.
	AlreadyDone bool
	// ReparkDone lets a needs-repair park overwrite a done ticket, for the one
	// caller whose finding is that the done itself is false (its commits never
	// landed). Every other park on a done ticket is refused.
	ReparkDone bool
}

// gxQuestions is the fixed wording of the parks gx itself asks about. Other
// kinds carry the agent's own question or are not questions.
var gxQuestions = map[events.Kind]string{
	events.ZeroCommit:         "The agent finished with no commits. Adopt it as done, or reopen the ticket?",
	events.StrandedCommits:    "The agent stopped with commits that never landed. Reopen the ticket to retry?",
	events.BackgroundTaskGate: "The agent ended its turn with a background task still running. Reopen the ticket?",
	events.AmbiguousBase:      "Several blockers have different branches. Pick the base branch.",
}

var (
	errQuestionOpen = errors.New("open question")
	errQuestionGone = errors.New("question no longer open")
)

// logGxQuestion writes the by-gx park.question for kind before the status
// flips. A ticket that already has an open question keeps it. A blocked pane's
// question lives only in the pane, so its summary is the park reason.
func logGxQuestion(ticketPath string, kind events.Kind, reason string) error {
	summary, ok := gxQuestions[kind]
	if kind == events.BlockedPane && reason != "" {
		summary, ok = reason, true
	}
	if !ok {
		return nil
	}
	text := summary
	if reason != "" && reason != summary {
		// The fixed wording carries no detail; the reason (e.g. the dirty file) does.
		text += "\n\n" + reason
	}
	ev := tickets.LogEvent{
		Type:      tickets.EventPauseQuestion,
		By:        "gx",
		PauseKind: string(kind),
		Summary:   summary,
		Text:      text,
	}
	_, err := tickets.AppendLogIf(ticketPath, ev, func(evs []tickets.LogEvent) error {
		if tickets.OpenQuestion(evs) != nil {
			return errQuestionOpen
		}
		return nil
	})
	if errors.Is(err, errQuestionOpen) {
		return nil
	}
	return err
}

// Reasons a park.closed names: what ended the question without an answer.
const (
	CloseBudgetKilled = "budget-killed"
	CloseRelaunch     = "relaunch"
	CloseUnparked     = "unparked"
)

// CloseOpenQuestion writes park.closed with reason for the ticket's open
// question, so the log ends it before the status moves on. A ticket without an
// open question is left alone.
func CloseOpenQuestion(ticketPath, reason string) error {
	evs, err := tickets.ReadLog(ticketPath)
	if err != nil {
		return err
	}
	q := tickets.OpenQuestion(evs)
	if q == nil {
		return nil
	}
	// The guard re-checks under the ticket lock: an answer may have landed since the read.
	_, err = tickets.AppendLogIf(ticketPath, tickets.LogEvent{
		Type: tickets.EventPauseClosed, Question: q.Seq, Reason: reason,
	}, func(evs []tickets.LogEvent) error {
		if cur := tickets.OpenQuestion(evs); cur == nil || cur.Seq != q.Seq {
			return errQuestionGone
		}
		return nil
	})
	if errors.Is(err, errQuestionGone) {
		return nil
	}
	return err
}

// isParkType reports whether t is an event type a park writes for a person to
// act on (as opposed to manual-land).
func isParkType(t events.Type) bool {
	return t == events.Paused
}

// PauseStatus is the needs-answer / needs-repair label that notifications still
// show for a pause of this kind.
func PauseStatus(k events.Kind) string {
	if k.AwaitsAnswer() {
		return string(schema.StatusLegacyAnswer)
	}
	return string(schema.StatusLegacyRepair)
}

// RecordManualLand writes done through the park path and appends the one
// manual-merge event. Only the ticket write's error is returned, matching park.
func RecordManualLand(scratchDir, epicName, ticket, path string, alreadyDone bool, ev Event) error {
	_, err := park(noopEventSink{}, parkRequest{
		ScratchDir: scratchDir, EpicName: epicName, Ticket: ticket, Path: path,
		Type: events.ManualMerge, Reason: ev.Reason, AlreadyDone: alreadyDone, Event: ev,
	})
	return err
}

// Park parks the ticket through the park path: one status write, one event and
// no live notification. The event type and the reason stub come from the kind.
// A blank reason is refused before anything is written. ev carries the run's
// agent context onto the event, as parkRequest.Event does.
func Park(scratchDir, epicName, ticket, path string, kind events.Kind, reason string, ev Event) error {
	_, err := park(noopEventSink{}, parkRequest{
		ScratchDir: scratchDir, EpicName: epicName, Ticket: ticket, Path: path,
		Type: events.Paused, Kind: kind, Reason: reason, Event: ev,
	})
	return err
}

// park is the single park path: it writes the ticket, appends exactly one
// event and notifies. Every status writer is routed here:
// needs-repair, needs-answer and the CLI land's done write.
//
// A failed ticket write is folded into the notified/logged reason so the event
// and notification still fire — a silent park is the bug this exists to
// remove — and is also returned, for callers whose flow must not continue past
// an unwritten park.
func park(sink EventSink, req parkRequest) (reason string, writeErr error) {
	reason = req.Reason
	switch {
	case req.Type == events.Paused && !req.Kind.AwaitsAnswer():
		writeErr = markNeedsRepairWithReason(req.Path, reason, schema.PauseKind(req.Kind), req.Repair, req.ReparkDone)
	case req.Type == events.Paused:
		if err := logGxQuestion(req.Path, req.Kind, reason); err != nil {
			// A silent park is the bug this path exists to remove: park anyway.
			reason = fmt.Sprintf("%s (also failed logging question: %v)", reason, err)
		}
		writeErr = markNeedsAnswer(req.Path, schema.PauseKind(req.Kind))
	case req.Type == events.ManualMerge:
		if !req.AlreadyDone {
			writeErr = MarkDone(req.Path)
		}
	default:
		writeErr = fmt.Errorf("park: unsupported type %q", req.Type)
	}
	// A park event on a done ticket would mislead recovery, which reads the
	// latest park reason from the run log.
	if errors.Is(writeErr, ErrParkDone) {
		log.Printf("ralphloop: refused %s park of %s/%s (%s): ticket is done", req.Type, req.EpicName, req.Ticket, req.Kind)
		return reason, writeErr
	}
	if writeErr != nil {
		reason = fmt.Sprintf("%s (also failed marking %s: %v)", reason, req.Type, writeErr)
	}
	ev := req.Event
	ev.Type, ev.Ticket, ev.Kind, ev.Reason = string(req.Type), req.Ticket, string(req.Kind), reason
	if err := logEvent(req.ScratchDir, req.EpicName, ev); err != nil {
		reason = fmt.Sprintf("%s (also failed logging event: %v)", reason, err)
	}
	if req.Type == events.ManualMerge {
		return reason, writeErr // a merge needs no human
	}
	sink.TicketNeedsHuman(req.Ticket, req.EpicName, PauseStatus(req.Kind), req.Kind, reason)
	return reason, writeErr
}
