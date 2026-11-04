package server

import (
	"net/http"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/sound"
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

// sessionsRead is a root handler: it reads the Queue's sessions.
func (s *Server) sessionsRead(w http.ResponseWriter, _ *http.Request) {
	list, err := s.queue.ListSessionsWithQueue()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, list)
}

// One-line forwarders to the Manager for the root's routes and hooks.

func (s *Server) triggerBanner(id string, snd sound.Name, reason string) {
	s.sessions.triggerBanner(id, snd, reason)
}
func (s *Server) noteFailure(id string)         { s.sessions.noteFailure(id) }
func (s *Server) onMapEpic(address string) bool { return s.sessions.onMapEpic(address) }
func (s *Server) sessionCreateWrite(w http.ResponseWriter, r *http.Request) {
	s.sessions.sessionCreateWrite(w, r)
}
func (s *Server) sessionFocusWrite(w http.ResponseWriter, r *http.Request) {
	s.sessions.sessionFocusWrite(w, r)
}
func (s *Server) sessionFocusWriteWeb(w http.ResponseWriter, r *http.Request) {
	s.sessions.sessionFocusWriteWeb(w, r)
}
func (s *Server) markSessionRead(req wire.SessionRequest) (wire.SessionResult, error) {
	return s.sessions.markSessionRead(req)
}
func (s *Server) focusSession(req wire.SessionRequest) (wire.SessionResult, error) {
	return s.sessions.focusSession(req)
}
func (s *Server) sessionWriteDesignWeb(do func(wire.SessionRequest) (wire.SessionResult, error)) http.HandlerFunc {
	return s.sessions.sessionWriteDesignWeb(do)
}
func (s *Server) sessionWriteAgentWeb(do func(wire.SessionRequest) (wire.SessionResult, error)) http.HandlerFunc {
	return s.sessions.sessionWriteAgentWeb(do)
}
func (s *Server) designLaunchTier(address string) ralphloop.Tier {
	return s.sessions.designLaunchTier(address)
}
func (s *Server) sessionCreateWeb(w http.ResponseWriter, r *http.Request) {
	s.sessions.sessionCreateWeb(w, r)
}

// One-line forwarders to the Resumer.

func (s *Server) fixAgentWrite(w http.ResponseWriter, r *http.Request) {
	s.resumer.fixAgentWrite(w, r)
}
func (s *Server) fixAgentWeb() http.HandlerFunc { return s.resumer.fixAgentWeb() }
func (s *Server) sessionLinkWrite(w http.ResponseWriter, r *http.Request) {
	s.resumer.sessionLinkWrite(w, r)
}
func (s *Server) sessionStepWrite(w http.ResponseWriter, r *http.Request) {
	s.resumer.sessionStepWrite(w, r)
}
