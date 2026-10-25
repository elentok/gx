package server

import (
	"encoding/json"
	"fmt"
	"net/http"
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

// setAutoAdvance flips a Design session's auto-advance switch. Nothing reads it
// yet but the wire.
func (s *Server) setAutoAdvance(req AutoAdvanceRequest) (SessionResult, error) {
	dir, meta, res, err := s.sessionDir(req.ID)
	if res.Refused || err != nil {
		return res, err
	}
	if !meta.Design {
		return sessionRefusal(ReasonSessionNotDesign, fmt.Sprintf("session %s is not a design session", req.ID)), nil
	}
	var info SessionInfo
	if err := s.updateSessionMeta(dir, req.ID, func(m *sessionMeta) bool {
		m.AutoAdvanceOff = req.Off
		info = m.info()
		return true
	}); err != nil {
		return SessionResult{}, err
	}
	return SessionResult{Session: &info}, nil
}
