package server

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/elentok/gx/sound"
)

func (s *Server) sessionActionableWrite(w http.ResponseWriter, r *http.Request) {
	var req MarkActionableRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	res, err := s.markSessionActionable(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, res)
}

// markSessionActionable saves an agent-raised Actionable, replacing any
// earlier one.
func (s *Server) markSessionActionable(req MarkActionableRequest) (SessionResult, error) {
	if req.Reason == "" {
		return sessionRefusal(ReasonSessionBadRequest, "an actionable needs a reason"), nil
	}
	dir, meta, res, err := s.sessionDir(req.ID)
	if res.Refused || err != nil {
		return res, err
	}
	if meta.Status.State == SessionExited {
		return sessionRefusal(ReasonSessionNotLive, fmt.Sprintf("session %s has exited", req.ID)), nil
	}
	var info SessionInfo
	s.updateSessionMeta(dir, req.ID, func(m *sessionMeta) bool {
		m.Actionable = SessionActionable{Reason: req.Reason, Action: req.Action}
		info = m.info()
		return true
	})
	s.triggerBanner(req.ID, sound.Attention, req.Reason)
	return SessionResult{Session: &info}, nil
}
