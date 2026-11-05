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
	s.sessions.TriggerBanner(id, snd, reason)
}
func (s *Server) noteFailure(id string)         { s.sessions.NoteFailure(id) }
func (s *Server) onMapEpic(address string) bool { return s.sessions.OnMapEpic(address) }
func (s *Server) sessionCreateWrite(w http.ResponseWriter, r *http.Request) {
	s.sessions.SessionCreateWrite(w, r)
}
func (s *Server) sessionFocusWrite(w http.ResponseWriter, r *http.Request) {
	s.sessions.SessionFocusWrite(w, r)
}
func (s *Server) sessionFocusWriteWeb(w http.ResponseWriter, r *http.Request) {
	s.sessions.SessionFocusWriteWeb(w, r)
}
func (s *Server) markSessionRead(req wire.SessionRequest) (wire.SessionResult, error) {
	return s.sessions.MarkSessionRead(req)
}
func (s *Server) focusSession(req wire.SessionRequest) (wire.SessionResult, error) {
	return s.sessions.FocusSession(req)
}
func (s *Server) sessionWriteDesignWeb(do func(wire.SessionRequest) (wire.SessionResult, error)) http.HandlerFunc {
	return s.sessions.SessionWriteDesignWeb(do)
}
func (s *Server) sessionWriteAgentWeb(do func(wire.SessionRequest) (wire.SessionResult, error)) http.HandlerFunc {
	return s.sessions.SessionWriteAgentWeb(do)
}
func (s *Server) designLaunchTier(address string) ralphloop.Tier {
	return s.sessions.DesignLaunchTier(address)
}
func (s *Server) sessionCreateWeb(w http.ResponseWriter, r *http.Request) {
	s.sessions.SessionCreateWeb(w, r)
}

// One-line forwarders to the Resumer.

func (s *Server) fixAgentWrite(w http.ResponseWriter, r *http.Request) {
	s.resumer.FixAgentWrite(w, r)
}
func (s *Server) fixAgentWeb() http.HandlerFunc { return s.resumer.FixAgentWeb() }
func (s *Server) sessionLinkWrite(w http.ResponseWriter, r *http.Request) {
	s.resumer.SessionLinkWrite(w, r)
}
func (s *Server) sessionStepWrite(w http.ResponseWriter, r *http.Request) {
	s.resumer.SessionStepWrite(w, r)
}
