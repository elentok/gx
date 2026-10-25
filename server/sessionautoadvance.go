package server

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/elentok/gx/harness"
)

// AutoAdvanceRequest is the body of POST /v1/sessions/auto-advance. State is
// the web's session state token. Off, not On, so a body without it leaves the
// switch on.
type AutoAdvanceRequest struct {
	ID    string `json:"id"`
	State string `json:"state,omitempty"`
	Off   bool   `json:"off,omitempty"`
}

func (s *Server) sessionAutoAdvanceWrite(w http.ResponseWriter, r *http.Request) {
	s.sessionAutoAdvanceHandler(false)(w, r)
}

// sessionAutoAdvanceWeb is sessionAutoAdvanceWrite for the web mux: it needs
// the state token of the session the page saw.
func (s *Server) sessionAutoAdvanceWeb() http.HandlerFunc { return s.sessionAutoAdvanceHandler(true) }

func (s *Server) sessionAutoAdvanceHandler(web bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req AutoAdvanceRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if web {
			res, err := s.checkDesignWrite(SessionRequest{ID: req.ID, State: req.State})
			if err == nil && res.Refused {
				writeJSON(w, res)
				return
			}
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}
		res, err := s.setAutoAdvance(req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, res)
	}
}

// canAutoAdvance is the one rule for whether the session's raised Actionable
// may be sent by the server: the switch is on, it is a derived Continue (not a
// grilling round, not agent- or gx-raised), the session waits for the person,
// and the harness is not Codex. Call sites add their own conditions: the
// detector needs a live turn and the cap, the switch needs no countdown running.
func (m sessionMeta) canAutoAdvance() bool {
	a := m.Actionable
	return !m.AutoAdvanceOff && a.Derived && !a.Grilling && len(a.Action) > 0 &&
		m.Status.Promptable() && m.Harness != string(harness.Codex)
}

// setAutoAdvance flips a Design session's auto-advance switch. Turning it off
// stops a running countdown.
func (s *Server) setAutoAdvance(req AutoAdvanceRequest) (SessionResult, error) {
	dir, meta, res, err := s.sessionDir(req.ID)
	if res.Refused || err != nil {
		return res, err
	}
	if !meta.Design {
		return sessionRefusal(ReasonSessionNotDesign, fmt.Sprintf("session %s is not a design session", req.ID)), nil
	}
	var info SessionInfo
	start := false
	if err := s.updateSessionMeta(dir, req.ID, func(m *sessionMeta) bool {
		m.AutoAdvanceOff = req.Off
		m.AutoSends = 0
		start = m.canAutoAdvance() && s.autoAdvances.deadline(req.ID).IsZero()
		info = m.info()
		return true
	}); err != nil {
		return SessionResult{}, err
	}
	if req.Off {
		s.cancelAutoAdvance(req.ID, "Auto-advance switched off")
	}
	// Switching on over a raised Continue starts the countdown a run end would have.
	if start && s.startAutoAdvance(req.ID) {
		s.events.publish(EventSessionChanged, req.ID)
	}
	return SessionResult{Session: &info}, nil
}
