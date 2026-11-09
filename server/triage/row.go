package triage

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
	triagerec "github.com/elentok/gx/triage"
)

func (t *Triage) rowHandler(w http.ResponseWriter, r *http.Request) {
	var req wire.TriageRowRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	res, err := t.saveRow(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	wire.WriteJSON(w, res)
}

func (t *Triage) abandonHandler(w http.ResponseWriter, r *http.Request) {
	var req wire.TriageAbandonRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	res, err := t.abandon(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	wire.WriteJSON(w, res)
}

// saveRow stores a row's person fields on the record revision the caller read.
func (t *Triage) saveRow(req wire.TriageRowRequest) (wire.TriageEditResult, error) {
	store, rec, refusal, err := t.liveRecord(req.Project)
	if err != nil || refusal != nil {
		return orRefusal(refusal), err
	}
	if msg := badAnswer(rec, req); msg != "" {
		return refusedEdit(wire.ReasonTriageAnswer, msg), nil
	}
	saved, err := store.SetPerson(rec.ID, req.Revision, req.Address, triagerec.PersonFields{
		Action: req.Action, Destination: req.Destination, Answers: req.Answers, KeepForLater: req.KeepForLater,
	})
	if res, handled := editRefusal(err); handled {
		return res, nil
	} else if err != nil {
		return wire.TriageEditResult{}, err
	}
	t.Changed(req.Project)
	return wire.TriageEditResult{Revision: saved.Revision}, nil
}

// abandon ends the Triage's live sessions and marks the record abandoned,
// under the same offer rule the view uses. It writes nothing to tickets.
func (t *Triage) abandon(req wire.TriageAbandonRequest) (wire.TriageEditResult, error) {
	store, rec, refusal, err := t.liveRecord(req.Project)
	if err != nil || refusal != nil {
		return orRefusal(refusal), err
	}
	if rec.Revision != req.Revision {
		return refusedEdit(wire.ReasonStale, triagerec.ErrStaleRevision.Error()), nil
	}
	if !CanAbandon(true, rec.Step, t.stopped(req.Project)) {
		return refusedEdit(wire.ReasonTriagePhase, "abandon is not offered at step "+rec.Step), nil
	}
	if err := t.stopSessions(req.Project); err != nil {
		return wire.TriageEditResult{}, err
	}
	saved, err := store.Abandon(rec.ID, req.Revision)
	if res, handled := editRefusal(err); handled {
		return res, nil
	} else if err != nil {
		return wire.TriageEditResult{}, err
	}
	t.Changed(req.Project)
	return wire.TriageEditResult{Revision: saved.Revision}, nil
}

// stopSessions stops the project's Triage sessions that have not exited.
func (t *Triage) stopSessions(project string) error {
	list, err := t.manager.ListSessions()
	if err != nil {
		return err
	}
	for _, in := range list {
		if in.Flow != string(tickets.FlowTriage) || in.FlowKey != project || in.Status.State == wire.SessionExited {
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
