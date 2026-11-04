package server

import (
	"fmt"
	"net/http"

	"github.com/elentok/gx/server/wire"
)

// SessionStateToken is the web's proof of which session state it acted on: the
// id, the status and when the status last changed. A stop or Restore carrying
// an older one is refused as stale.
func SessionStateToken(s wire.SessionInfo) string {
	return fmt.Sprintf("%s:%s:%d", s.ID, s.Status.State, s.StatusChangedAt.UnixNano())
}

// sessionWriteDesignWeb is sessionWrite for the web mux: only a design session,
// and only with the state token of the session the page saw.
func (s *Server) sessionWriteDesignWeb(do func(wire.SessionRequest) (wire.SessionResult, error)) http.HandlerFunc {
	return sessionWriteAs(s.checkDesignWrite, do)
}

// checkDesignWrite returns the refusal for a web write on a session that is not
// a design session or whose state moved since the page saw it.
func (s *Server) checkDesignWrite(req wire.SessionRequest) (wire.SessionResult, error) {
	_, meta, res, err := s.SessionDir(req.ID)
	if res.Refused || err != nil {
		return res, err
	}
	if !meta.Design {
		return sessionRefusal(wire.ReasonSessionNotDesign, fmt.Sprintf("session %s is not a design session", req.ID)), nil
	}
	if req.State != SessionStateToken(meta.info()) {
		return sessionRefusal(wire.ReasonStale, "the session changed; review its current state"), nil
	}
	return wire.SessionResult{}, nil
}

// sessionWriteAgentWeb is sessionWrite for a web stop or remove: any session
// except a Queue agent, which ralph-loop drives, and only with the state token
// of the session the page saw.
func (s *Server) sessionWriteAgentWeb(do func(wire.SessionRequest) (wire.SessionResult, error)) http.HandlerFunc {
	return sessionWriteAs(s.checkAgentWrite, do)
}

// checkAgentWrite returns the refusal for a web write on a Queue session or on
// a session whose state moved since the page saw it.
func (s *Server) checkAgentWrite(req wire.SessionRequest) (wire.SessionResult, error) {
	_, meta, res, err := s.SessionDir(req.ID)
	if res.Refused || err != nil {
		return res, err
	}
	if meta.info().Queue {
		return sessionRefusal(wire.ReasonSessionQueue, fmt.Sprintf("session %s is a Queue agent; ralph-loop drives it", req.ID)), nil
	}
	if req.State != SessionStateToken(meta.info()) {
		return sessionRefusal(wire.ReasonStale, "the session changed; review its current state"), nil
	}
	return wire.SessionResult{}, nil
}
