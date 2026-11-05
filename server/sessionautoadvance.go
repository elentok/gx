package server

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/elentok/gx/server/sessions"
	"github.com/elentok/gx/server/wire"
)

func (s *Server) sessionAutoAdvanceWrite(w http.ResponseWriter, r *http.Request) {
	s.sessionAutoAdvanceHandler(nil)(w, r)
}

// sessionAutoAdvanceWeb is sessionAutoAdvanceWrite for the web mux: it needs
// the state token of the session the page saw.
func (s *Server) sessionAutoAdvanceWeb() http.HandlerFunc {
	return s.sessionAutoAdvanceHandler(func(req wire.AutoAdvanceRequest) (wire.SessionResult, error) {
		return s.sessions.CheckDesignWrite(wire.SessionRequest{ID: req.ID, State: req.State})
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

// setAutoAdvance flips a Design session's auto-advance switch. Turning it off
// stops a running countdown.
func (s *Server) setAutoAdvance(req wire.AutoAdvanceRequest) (wire.SessionResult, error) {
	dir, meta, res, err := s.sessions.SessionDir(req.ID)
	if res.Refused || err != nil {
		return res, err
	}
	if !meta.Design {
		return sessions.Refusal(wire.ReasonSessionNotDesign, fmt.Sprintf("session %s is not a design session", req.ID)), nil
	}
	var info wire.SessionInfo
	start := false
	if err := s.sessions.UpdateSessionMeta(dir, req.ID, func(m *sessions.Meta) bool {
		m.AutoAdvanceOff = req.Off
		m.AutoSends = 0
		start = m.CanAutoAdvance() && s.autoAdvances.deadline(req.ID).IsZero()
		info = m.Info()
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
