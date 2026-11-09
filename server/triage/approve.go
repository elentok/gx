package triage

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/elentok/gx/server/wire"
	triagerec "github.com/elentok/gx/triage"
)

func (t *Triage) approveHandler(w http.ResponseWriter, r *http.Request) {
	var req wire.TriageApproveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	res, err := t.approve(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	wire.WriteJSON(w, res)
}

func rowActionHandler(action func(wire.TriageRowActionRequest) (wire.TriageEditResult, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req wire.TriageRowActionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		res, err := action(req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		wire.WriteJSON(w, res)
	}
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
	store, rec, refusal, err := t.liveRecord(req.Project)
	if err != nil || refusal != nil {
		return orRefusal(refusal), err
	}
	saved, err := store.Approve(rec.ID, req.Revision)
	if errors.Is(err, triagerec.ErrAlreadyApproved) {
		return refusedEdit(wire.ReasonTriageAlreadyApproved, err.Error()), nil
	}
	if res, handled := editRefusal(err); handled {
		return res, nil
	} else if err != nil {
		return wire.TriageEditResult{}, err
	}
	t.Changed(req.Project)
	if err := t.launchWrite(req.Project, false); err != nil {
		return wire.TriageEditResult{}, err
	}
	return wire.TriageEditResult{Revision: saved.Revision}, nil
}

// retry sets a failed row pending and starts a Write session unless one is live.
func (t *Triage) retry(req wire.TriageRowActionRequest) (wire.TriageEditResult, error) {
	store, rec, refusal, err := t.liveRecord(req.Project)
	if err != nil || refusal != nil {
		return orRefusal(refusal), err
	}
	saved, err := store.Retry(rec.ID, req.Revision, req.Address)
	if res, handled := editRefusal(err); handled {
		return res, nil
	} else if err != nil {
		return wire.TriageEditResult{}, err
	}
	t.Changed(req.Project)
	if err := t.launchWrite(req.Project, true); err != nil {
		return wire.TriageEditResult{}, err
	}
	return wire.TriageEditResult{Revision: saved.Revision}, nil
}

// keep marks a failed row kept while its follow-up is still in follow-ups.
func (t *Triage) keep(req wire.TriageRowActionRequest) (wire.TriageEditResult, error) {
	store, rec, refusal, err := t.liveRecord(req.Project)
	if err != nil || refusal != nil {
		return orRefusal(refusal), err
	}
	saved, err := store.KeepForLater(rec.ID, req.Revision, req.Address)
	if res, handled := editRefusal(err); handled {
		return res, nil
	} else if err != nil {
		return wire.TriageEditResult{}, err
	}
	t.Changed(req.Project)
	return wire.TriageEditResult{Revision: saved.Revision}, nil
}

func orRefusal(r *wire.TriageEditResult) wire.TriageEditResult {
	if r == nil {
		return wire.TriageEditResult{}
	}
	return *r
}

// launchWrite starts the Write session; with skipIfLive it does nothing when
// the project's Triage already has a live session.
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
