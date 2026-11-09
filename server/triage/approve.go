package triage

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/elentok/gx/server/wire"
	triagerec "github.com/elentok/gx/triage"
)

// jsonHandler decodes the request body into Req (400 on failure), calls fn
// (500 on error) and writes its result as JSON.
func jsonHandler[Req, Res any](fn func(Req) (Res, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req Req
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		res, err := fn(req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		wire.WriteJSON(w, res)
	}
}

// refusalError lets an edit's apply func refuse the edit with a result.
type refusalError struct{ res wire.TriageEditResult }

func (e *refusalError) Error() string { return e.res.Message }

func refusal(reason, msg string) error { return &refusalError{refusedEdit(reason, msg)} }

// edit loads the project's live record, runs apply on it and turns a record
// rule error or a refusalError into a refusal result. A saved edit publishes
// triage-changed and returns the new revision.
func (t *Triage) edit(project string, apply func(*triagerec.Store, *triagerec.Record) (*triagerec.Record, error)) (wire.TriageEditResult, error) {
	store, rec, noLive, err := t.liveRecord(project)
	if err != nil || noLive != nil {
		return orRefusal(noLive), err
	}
	saved, err := apply(store, rec)
	if res, handled := editRefusal(err); handled {
		return res, nil
	} else if err != nil {
		return wire.TriageEditResult{}, err
	}
	t.Changed(project)
	return wire.TriageEditResult{Revision: saved.Revision}, nil
}

// editAndLaunch is edit followed, when the edit saved, by launchWrite.
func (t *Triage) editAndLaunch(project string, skipIfLive bool, apply func(*triagerec.Store, *triagerec.Record) (*triagerec.Record, error)) (wire.TriageEditResult, error) {
	res, err := t.edit(project, apply)
	if err != nil || res.Refused {
		return res, err
	}
	if err := t.launchWrite(project, skipIfLive); err != nil {
		return wire.TriageEditResult{}, err
	}
	return res, nil
}

// liveRecord loads the project's live record, or a refusal result.
func (t *Triage) liveRecord(project string) (*triagerec.Store, *triagerec.Record, *wire.TriageEditResult, error) {
	dir, err := t.projects.Dir(project)
	if err != nil {
		return nil, nil, nil, err
	}
	store := triagerec.NewStore(dir)
	rec, err := store.LoadLive()
	if err != nil {
		return nil, nil, nil, err
	}
	if rec == nil {
		res := refusedEdit(wire.ReasonTriageNoLive, project+" has no live Triage")
		return nil, nil, &res, nil
	}
	return store, rec, nil, nil
}

// approve wins the record's compare-and-set and only then starts the Write
// session, so of two approves (web or chat) exactly one writes.
func (t *Triage) approve(req wire.TriageApproveRequest) (wire.TriageEditResult, error) {
	return t.editAndLaunch(req.Project, false, func(store *triagerec.Store, rec *triagerec.Record) (*triagerec.Record, error) {
		saved, err := store.Approve(rec.ID, req.Revision)
		if errors.Is(err, triagerec.ErrAlreadyApproved) {
			return nil, refusal(wire.ReasonTriageAlreadyApproved, err.Error())
		}
		return saved, err
	})
}

// sendBack returns the record to Plan with the person's answers kept, and starts
// the session that asks again. Like approve, it wins the record's compare-and-set
// first, so a double click makes one round.
func (t *Triage) sendBack(req wire.TriageSendBackRequest) (wire.TriageEditResult, error) {
	return t.editAndLaunch(req.Project, false, func(store *triagerec.Store, rec *triagerec.Record) (*triagerec.Record, error) {
		return store.SendBack(rec.ID, req.Revision)
	})
}

// retry sets a failed row pending and starts a Write session unless one is live.
func (t *Triage) retry(req wire.TriageRowActionRequest) (wire.TriageEditResult, error) {
	return t.editAndLaunch(req.Project, true, func(store *triagerec.Store, rec *triagerec.Record) (*triagerec.Record, error) {
		return store.Retry(rec.ID, req.Revision, req.Address)
	})
}

// keep marks a failed row kept while its follow-up is still in follow-ups.
func (t *Triage) keep(req wire.TriageRowActionRequest) (wire.TriageEditResult, error) {
	return t.edit(req.Project, func(store *triagerec.Store, rec *triagerec.Record) (*triagerec.Record, error) {
		return store.KeepForLater(rec.ID, req.Revision, req.Address)
	})
}

func orRefusal(r *wire.TriageEditResult) wire.TriageEditResult {
	if r == nil {
		return wire.TriageEditResult{}
	}
	return *r
}

// launchWrite starts the project's Triage session, which reads the record's step
// (Write after approve, Plan after a send back); with skipIfLive it does nothing
// when the project's Triage already has a live session.
func (t *Triage) launchWrite(project string, skipIfLive bool) error {
	defer t.lockProject(project)()
	if skipIfLive {
		list, err := t.manager.ListSessions()
		if err != nil {
			return err
		}
		if liveSession(list, project) != nil {
			return nil
		}
	}
	res, err := t.launch(project)
	if err != nil {
		return err
	}
	if res.Refused {
		return errors.New(res.Message)
	}
	return nil
}
