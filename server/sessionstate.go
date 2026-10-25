package server

import (
	"fmt"
	"net/http"
)

// SessionRequest names the session a stop or remove acts on. State is the
// token the web sends with a stop or Restore; see SessionStateToken. Only the
// web mux checks it (see checkDesignWrite): the plain mux ignores State, and on
// the web mux an empty State never matches a token, so it is refused as stale.
type SessionRequest struct {
	ID    string `json:"id"`
	State string `json:"state,omitempty"`
}

// SessionStateToken is the web's proof of which session state it acted on: the
// id, the status and when the status last changed. A stop or Restore carrying
// an older one is refused as stale.
func SessionStateToken(s SessionInfo) string {
	return fmt.Sprintf("%s:%s:%d", s.ID, s.Status.State, s.StatusChangedAt.UnixNano())
}

// sessionWriteDesignWeb is sessionWrite for the web mux: only a design session,
// and only with the state token of the session the page saw.
func (s *Server) sessionWriteDesignWeb(do func(SessionRequest) (SessionResult, error)) http.HandlerFunc {
	return sessionWriteAs(s.checkDesignWrite, do)
}

// checkDesignWrite returns the refusal for a web write on a session that is not
// a design session or whose state moved since the page saw it.
func (s *Server) checkDesignWrite(req SessionRequest) (SessionResult, error) {
	_, meta, res, err := s.sessionDir(req.ID)
	if res.Refused || err != nil {
		return res, err
	}
	if !meta.Design {
		return sessionRefusal(ReasonSessionNotDesign, fmt.Sprintf("session %s is not a design session", req.ID)), nil
	}
	if req.State != SessionStateToken(meta.info()) {
		return sessionRefusal(ReasonStale, "the session changed; review its current state"), nil
	}
	return SessionResult{}, nil
}
