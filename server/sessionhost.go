package server

import (
	"net/http"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/server/sessions"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/sound"
)

// These three are the host methods other packages call on the Server through
// their Host interfaces; each forwards to the Manager.

// ListSessions reads every session dir, oldest first.
func (s *Server) ListSessions() ([]wire.SessionInfo, error) { return s.sessions.ListSessions() }

// SessionDir finds a session's dir and meta by id.
func (s *Server) SessionDir(id string) (string, sessions.Meta, wire.SessionResult, error) {
	return s.sessions.SessionDir(id)
}

// SessionFinished reports whether a finished session was a ticket agent that
// reported iteration_status finished.
func (s *Server) SessionFinished(sess agentrunner.Session) bool {
	return s.sessions.SessionFinished(sess)
}

// One-line forwarders to the Manager for the root's routes and hooks.

func (s *Server) triggerBanner(id string, snd sound.Name, reason string) {
	s.sessions.TriggerBanner(id, snd, reason)
}
func (s *Server) noteFailure(id string)         { s.sessions.NoteFailure(id) }
func (s *Server) onMapEpic(address string) bool { return s.sessions.Maps.OnMapEpic(address) }
func (s *Server) sessionWriteDesignWeb(do func(wire.SessionRequest) (wire.SessionResult, error)) http.HandlerFunc {
	return s.sessions.SessionWriteDesignWeb(do)
}
func (s *Server) designLaunchTier(address string) ralphloop.Tier {
	return s.sessions.Maps.DesignLaunchTier(address)
}
