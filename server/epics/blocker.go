package epics

import (
	"encoding/json"
	"errors"
	"net/http"
	"path/filepath"
	"slices"

	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
)

func blockerRefusal(res wire.EpicBlockerResult, reason, msg string) wire.EpicBlockerResult {
	res.Refused, res.Reason, res.Message = true, reason, msg
	return res
}

// refusalOf turns a blocker check error into a refusal; ok is false for an
// error that is not one the person can act on.
func refusalOf(res wire.EpicBlockerResult, err error) (wire.EpicBlockerResult, bool) {
	var cyc *tickets.BlockedByCycleError
	if errors.As(err, &cyc) {
		res = blockerRefusal(res, wire.ReasonBlockedByCycle, err.Error())
		res.Blocker, res.Chain = cyc.Blocker, cyc.Chain
		return res, true
	}
	var ae *tickets.AddressError
	if errors.As(err, &ae) {
		return blockerRefusal(res, ae.Code, err.Error()), true
	}
	return res, false
}

// EpicBlocker adds or removes one blocker epic on an epic's own blocked_by.
// Only the entry being added is checked, so a stale entry never refuses it, and
// every other stored entry stays as it is. The mutex spans read to write: the
// write takes only a per-file lock, so two clicks could otherwise lose one.
func (s *Epics) EpicBlocker(req wire.EpicBlockerRequest) (wire.EpicBlockerResult, error) {
	var res wire.EpicBlockerResult
	project, epic, ok := tickets.ParseEpicAddress(req.Address)
	if !ok {
		return blockerRefusal(res, wire.ReasonInvalidAddress, req.Address+" is not an epic address (want project:epic)"), nil
	}
	bProject, bEpic, ok := tickets.ParseEpicAddress(req.Blocker)
	if !ok {
		return blockerRefusal(res, wire.ReasonInvalidAddress, req.Blocker+" is not an epic address (want project:epic)"), nil
	}

	s.blockerMu.Lock()
	defer s.blockerMu.Unlock()
	dir := filepath.Join(s.TicketStore, project)
	all, err := tickets.Load(dir)
	if err != nil {
		return res, err
	}
	i := slices.IndexFunc(all, func(e tickets.Epic) bool { return e.Name == epic })
	if i < 0 {
		return blockerRefusal(res, tickets.CodeUnknownEpic, "no epic "+req.Address), nil
	}
	stored := all[i].BlockedBy
	res.BlockedBy = stored
	names := func(ref string) bool {
		p, e, isEpic := tickets.BlockedByEpicRef(project, ref)
		return isEpic && p == bProject && e == bEpic
	}

	refs := slices.Clone(stored)
	if req.Remove {
		refs = slices.DeleteFunc(refs, names)
	} else if !slices.ContainsFunc(stored, names) {
		added := bEpic
		if bProject != project {
			added = req.Blocker
		}
		refs = append(refs, added)
		if err := tickets.CheckEpicBlockerAdd(filepath.Join(dir, epic, "ticket.md"), refs, added); err != nil {
			if out, ok := refusalOf(res, err); ok {
				return out, nil
			}
			return res, err
		}
	}
	if slices.Equal(refs, stored) {
		return res, nil
	}
	if refs == nil {
		refs = []string{}
	}
	if err := tickets.UpdateEpicFields(filepath.Join(dir, epic, "ticket.md"), tickets.EpicUpdate{BlockedBy: &refs}); err != nil {
		return res, err
	}
	res.BlockedBy = refs
	if s.Rescan != nil {
		s.Rescan()
	}
	return res, nil
}

func (s *Epics) blockerRoute(w http.ResponseWriter, r *http.Request) {
	var req wire.EpicBlockerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	res, err := s.EpicBlocker(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	wire.WriteJSON(w, res)
}
