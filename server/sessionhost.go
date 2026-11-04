package server

import (
	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/server/wire"
)

// These three are the host methods other packages call on the Server through
// their Host interfaces; each forwards to the Manager.

// ListSessions reads every session dir, oldest first.
func (s *Server) ListSessions() ([]wire.SessionInfo, error) { return s.sessions.ListSessions() }

// SessionDir finds a session's dir and meta by id.
func (s *Server) SessionDir(id string) (string, sessionMeta, wire.SessionResult, error) {
	return s.sessions.SessionDir(id)
}

// SessionFinished reports whether a finished session was a ticket agent that
// reported iteration_status finished.
func (s *Server) SessionFinished(sess agentrunner.Session) bool {
	return s.sessions.SessionFinished(sess)
}
