package triage

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"

	"github.com/elentok/gx/harness"
	"github.com/elentok/gx/server/sessions"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/ticketgit"
	triagerec "github.com/elentok/gx/triage"
)

// BeforeLaunch is a test seam: it runs after the record exists and before the
// session is created.
var BeforeLaunch func(project string)

// GiveSessions sets the Manager the Triage sessions are made and stopped through.
func (t *Triage) GiveSessions(m *sessions.Manager) { t.manager = m }

func (t *Triage) openHandler(web bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req wire.TriageOpenRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		res, err := t.open(req, web)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		wire.WriteJSON(w, res)
	}
}

// open starts the project's Triage or returns the live one. Calls for one
// project run one at a time, so a double click makes one record and one session.
func (t *Triage) open(req wire.TriageOpenRequest, web bool) (wire.SessionResult, error) {
	project := req.Project
	dir, err := t.projects.Dir(project)
	if err != nil {
		return sessions.Refusal(wire.ReasonSessionBadRequest, err.Error()), nil
	}
	mu, _ := t.opens.LoadOrStore(project, &sync.Mutex{})
	mu.(*sync.Mutex).Lock()
	defer mu.(*sync.Mutex).Unlock()

	store := triagerec.NewStore(dir)
	rec, err := store.LoadLive()
	if err != nil {
		return wire.SessionResult{}, err
	}
	if rec == nil {
		if !store.HasFollowUps() {
			return sessions.Refusal(wire.ReasonTriageNoFollowUps, project+" has no open follow-up to triage"), nil
		}
		rec, err = store.Create()
		if _, ok := errors.AsType[*triagerec.LiveError](err); ok {
			rec, err = store.LoadLive()
		}
		if err != nil {
			return wire.SessionResult{}, err
		}
		t.Changed(project)
	}
	if rec != nil && rec.Step == tickets.StepApprove {
		return sessions.Refusal(wire.ReasonTriageApprove, "the Triage waits for approval on its page"), nil
	}

	list, err := t.manager.ListSessions()
	if err != nil {
		return wire.SessionResult{}, err
	}
	if live := liveSession(list, project); live != nil {
		if !req.New {
			return wire.SessionResult{Session: live}, nil
		}
		if web && req.State != sessions.SessionStateToken(*live) {
			return sessions.Refusal(wire.ReasonStale, "the session changed; review its current state"), nil
		}
		if res, err := t.manager.StopSession(wire.SessionRequest{ID: live.ID}); err != nil || (res.Refused && res.Reason != wire.ReasonSessionNotLive) {
			return res, err
		}
	}

	checkout := ticketgit.TrunkCheckout(t.ticketStore, project)
	if checkout == "" {
		return sessions.Refusal(wire.ReasonSessionBadRequest, fmt.Sprintf("%s has no trunk checkout", project)), nil
	}
	if BeforeLaunch != nil {
		BeforeLaunch(project)
	}
	return t.manager.CreateSession(wire.CreateSessionRequest{
		Project: project,
		Dir:     checkout,
		Name:    "triage",
		Prompt:  harness.SkillCommand(t.projects.LaunchHarness(project), "gx-triage-follow-ups", "--project "+project),
	}, sessions.SessionCtx{Flow: tickets.FlowTriage, FlowKey: project})
}

// liveSession is the project's Triage session that has not exited and has the
// latest RunStartedAt, or nil.
func liveSession(list []wire.SessionInfo, project string) *wire.SessionInfo {
	var live *wire.SessionInfo
	for i := range list {
		in := &list[i]
		if in.Flow == string(tickets.FlowTriage) && in.FlowKey == project && in.Status.State != wire.SessionExited &&
			(live == nil || !in.RunStartedAt.Before(live.RunStartedAt)) {
			live = in
		}
	}
	return live
}
