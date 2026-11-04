package server

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/elentok/gx/server/wire"
)

func (s *Server) sessionAutoAdvanceWrite(w http.ResponseWriter, r *http.Request) {
	s.sessionAutoAdvanceHandler(nil)(w, r)
}

// sessionAutoAdvanceWeb is sessionAutoAdvanceWrite for the web mux: it needs
// the state token of the session the page saw.
func (s *Server) sessionAutoAdvanceWeb() http.HandlerFunc {
	return s.sessionAutoAdvanceHandler(func(req wire.AutoAdvanceRequest) (wire.SessionResult, error) {
		return s.checkDesignWrite(wire.SessionRequest{ID: req.ID, State: req.State})
	})
}

// sessionAutoAdvanceHandler serves the switch. A non-nil guard runs first; a
// refusal from it is the response.
func (s *Server) sessionAutoAdvanceHandler(guard func(wire.AutoAdvanceRequest) (wire.SessionResult, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req wire.AutoAdvanceRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if guard != nil {
			res, err := guard(req)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			if res.Refused {
				writeJSON(w, res)
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
// whatever its harness. Call sites add their own conditions: the
// detector needs a live turn and the cap, the switch needs no countdown running.
func (m sessionMeta) canAutoAdvance() bool {
	a := m.Actionable
	return !m.AutoAdvanceOff && a.Derived && !a.Grilling && len(a.Action) > 0 &&
		m.Status.Promptable()
}

// setAutoAdvance flips a Design session's auto-advance switch. Turning it off
// stops a running countdown.
func (s *Server) setAutoAdvance(req wire.AutoAdvanceRequest) (wire.SessionResult, error) {
	dir, meta, res, err := s.sessionDir(req.ID)
	if res.Refused || err != nil {
		return res, err
	}
	if !meta.Design {
		return sessionRefusal(wire.ReasonSessionNotDesign, fmt.Sprintf("session %s is not a design session", req.ID)), nil
	}
	var info wire.SessionInfo
	start := false
	if err := s.updateSessionMeta(dir, req.ID, func(m *sessionMeta) bool {
		m.AutoAdvanceOff = req.Off
		m.AutoSends = 0
		start = m.canAutoAdvance() && s.autoAdvances.deadline(req.ID).IsZero()
		info = m.info()
		return true
	}); err != nil {
		return wire.SessionResult{}, err
	}
	if req.Off {
		s.cancelAutoAdvance(req.ID, "Auto-advance switched off")
	}
	// Switching on over a raised Continue starts the countdown a run end would have.
	if start && s.startAutoAdvance(req.ID) {
		s.events.Publish(wire.EventSessionChanged, req.ID)
	}
	return wire.SessionResult{Session: &info}, nil
}
