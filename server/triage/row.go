package triage

import (
	"encoding/json"
	"errors"
	"net/http"

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
