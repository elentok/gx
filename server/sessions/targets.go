package sessions

import (
	"encoding/json"
	"net/http"

	"github.com/elentok/gx/server/wire"
)

// RemoveTarget removes a session. With yes a live one is stopped first, in the
// same call; an Exited one has nothing to stop.
func (mg *Manager) RemoveTarget(id string, yes bool) (wire.SessionResult, error) {
	req := wire.SessionRequest{ID: id}
	if yes {
		res, err := mg.StopSession(req)
		if err != nil || (res.Refused && res.Reason != wire.ReasonSessionNotLive) {
			return res, err
		}
	}
	return mg.RemoveSession(req)
}

// SessionTargetsWrite serves a many-target stop or remove. The web mux also
// refuses a Queue session and a row whose token no longer matches; the plain mux
// sends no token and refuses neither. A refusal is a result and blocks no other
// target.
func (mg *Manager) SessionTargetsWrite(do func(id string, yes bool) (wire.SessionResult, error), web bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req wire.SessionTargetsRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if len(req.Targets) == 0 {
			http.Error(w, "no targets", http.StatusBadRequest)
			return
		}
		out := wire.TargetsResult{Results: make([]wire.TargetResult, 0, len(req.Targets))}
		for _, t := range req.Targets {
			res := wire.SessionResult{}
			var err error
			if web {
				res, err = mg.checkAgentWrite(t)
			}
			if err == nil && !res.Refused {
				res, err = do(t.ID, req.Yes)
			}
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			if res.Refused {
				out.Results = append(out.Results, wire.RefusedTarget(t.ID, res.Reason, res.Message))
			} else {
				out.Results = append(out.Results, wire.TargetResult{Target: t.ID, Status: wire.TargetDone})
			}
		}
		wire.WriteJSON(w, out)
	}
}
