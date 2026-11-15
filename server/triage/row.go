package triage

import (
	"errors"

	"github.com/elentok/gx/server/wire"
	triagerec "github.com/elentok/gx/triage"
)

// ungroup undoes the group the row is in, on the record revision the caller read.
func (t *Triage) ungroup(req wire.TriageUngroupRequest) (wire.TriageEditResult, error) {
	return t.edit(req.Project, func(store *triagerec.Store, rec *triagerec.Record) (*triagerec.Record, error) {
		return store.Ungroup(rec.ID, req.Revision, req.Address)
	})
}

// abandon ends the Triage's live sessions and marks the record abandoned,
// under the same offer rule the view uses. It writes nothing to tickets.
func (t *Triage) abandon(req wire.TriageAbandonRequest) (wire.TriageEditResult, error) {
	return t.edit(req.Project, func(store *triagerec.Store, rec *triagerec.Record) (*triagerec.Record, error) {
		if rec.Revision != req.Revision {
			return nil, triagerec.ErrStaleRevision
		}
		if !CanAbandon(true, rec.Step, rec.LegacyWrite()) {
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
