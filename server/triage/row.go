package triage

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/elentok/gx/server/wire"
	triagerec "github.com/elentok/gx/triage"
)

// saveRow stores a row's person fields on the record revision the caller read.
func (t *Triage) saveRow(req wire.TriageRowRequest) (wire.TriageEditResult, error) {
	return t.edit(req.Project, func(store *triagerec.Store, rec *triagerec.Record) (*triagerec.Record, error) {
		if msg := badAnswer(rec, req); msg != "" {
			return nil, refusal(wire.ReasonTriageAnswer, msg)
		}
		return store.SetPerson(rec.ID, req.Revision, req.Address, triagerec.PersonFields{
			Action: req.Action, Destination: req.Destination, Answers: req.Answers, KeepForLater: req.KeepForLater,
		})
	})
}

// abandon ends the Triage's live sessions and marks the record abandoned,
// under the same offer rule the view uses. It writes nothing to tickets.
func (t *Triage) abandon(req wire.TriageAbandonRequest) (wire.TriageEditResult, error) {
	return t.edit(req.Project, func(store *triagerec.Store, rec *triagerec.Record) (*triagerec.Record, error) {
		if rec.Revision != req.Revision {
			return nil, triagerec.ErrStaleRevision
		}
		if !CanAbandon(true, rec.Step, t.stopped(req.Project)) {
			return nil, refusal(wire.ReasonTriagePhase, "abandon is not offered at step "+rec.Step)
		}
		if err := t.stopSessions(req.Project); err != nil {
			return nil, err
		}
		return store.Abandon(rec.ID, req.Revision)
	})
}

// stopSessions stops the project's Triage sessions that have not exited.
func (t *Triage) stopSessions(project string) error {
	list, err := t.manager.ListSessions()
	if err != nil {
		return err
	}
	for _, in := range list {
		if !isLiveTriage(in, project) {
			continue
		}
		res, err := t.manager.StopSession(wire.SessionRequest{ID: in.ID})
		if err != nil {
			return err
		}
		if res.Refused && res.Reason != wire.ReasonSessionNotLive {
			return errors.New(res.Message)
		}
	}
	return nil
}

func refusedEdit(reason, msg string) wire.TriageEditResult {
	return wire.TriageEditResult{Refused: true, Reason: reason, Message: msg}
}

// editRefusal maps a record rule error to a refusal result; handled is false
// for any other error.
func editRefusal(err error) (res wire.TriageEditResult, handled bool) {
	if re, ok := errors.AsType[*refusalError](err); ok {
		return re.res, true
	}
	if errors.Is(err, triagerec.ErrStaleRevision) {
		return refusedEdit(wire.ReasonStale, err.Error()), true
	}
	if pe, ok := errors.AsType[*triagerec.PhaseError](err); ok {
		return refusedEdit(wire.ReasonTriagePhase, pe.Msg), true
	}
	return res, false
}

// noteMark joins an answer to its note: "<answer>\nNote: <note>".
const noteMark = "\nNote: "

// badAnswer says why the web may not save the request's answers, or "". Every
// answer must name a question of the row; one with options must be an option or
// Huh?, with or without a note. A page running an older script could otherwise
// save an answer meant for another question, so the server holds the line.
func badAnswer(rec *triagerec.Record, req wire.TriageRowRequest) string {
	if len(req.Answers) == 0 {
		return ""
	}
	var row *triagerec.Row
	for i := range rec.Rows {
		if rec.Rows[i].Address == req.Address {
			row = &rec.Rows[i]
		}
	}
	if row == nil {
		return "" // SetPerson refuses an unknown row
	}
	for id, answer := range req.Answers {
		qi := slices.IndexFunc(row.AllQuestions(), func(q triagerec.Question) bool { return q.ID == id })
		if qi < 0 {
			return fmt.Sprintf("%s has no question %s", req.Address, id)
		}
		q := row.AllQuestions()[qi]
		base, _, _ := strings.Cut(answer, noteMark)
		if len(q.Options) > 0 && base != wire.HuhMessage && !slices.Contains(q.Options, base) {
			return fmt.Sprintf("%q is not an option of %s question %s; reload the page", base, req.Address, id)
		}
	}
	return ""
}
