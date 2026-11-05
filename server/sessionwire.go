package server

import (
	"net/http"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/server/queue"
	"github.com/elentok/gx/server/sessions"
	"github.com/elentok/gx/server/settings"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/sound"
)

// newManager copies the fields it needs from s at call time, so s must have
// them set (projects, harness and sessionChat come late in New).
func newManager(s *Server) *sessions.Manager {
	return sessions.NewManager(sessions.ManagerDeps{
		Cfg: sessions.SessionsConfig{
			SessionsDir: s.cfg.SessionsDir, ConversationsDir: s.cfg.ConversationsDir,
			TicketStore: s.cfg.TicketStore, Env: s.cfg.Env, Holder: s.cfg.Holder,
			Notifier: s.cfg.Notifier, Player: s.cfg.Player,
			SessionBannerDelay: s.cfg.SessionBannerDelay, SessionExitCleanupDelay: s.cfg.SessionExitCleanupDelay,
		},
		Log:      s.log,
		Events:   s.events,
		Projects: s.projects,
		Settings: &s.settings,
		Harness:  s.harness,
		Idx:      s.idx,
		Chat:     s.sessionChat,
		Focus:    s.focus,
		AFK:      s.afk,
		Alerts:   s.chat,
		Queue:    s.queue,
		Host:     sessionHost{s},
		Prompter: s.prompt,
		Designs:  s.designs,
		Rollouts: sessionHost{s},
	})
}

// newResumer needs s.sessions set.
func newResumer(s *Server) *sessions.Resumer {
	return sessions.NewResumer(s.sessions)
}

// fixAgentHost is the Server as the fix agent sees it.
// FindTicket and MigrationFailures come from sessionHost and the Server.
type fixAgentHost struct{ sessionHost }

func (h fixAgentHost) CreateSession(req wire.CreateSessionRequest, remoteControl string) (wire.SessionResult, error) {
	return h.sessions.CreateSession(req, sessions.SessionCtx{RemoteControl: remoteControl})
}

func (h fixAgentHost) LaunchHarness(project string) string { return h.projects.LaunchHarness(project) }
func (h fixAgentHost) TicketStore() string                 { return h.cfg.TicketStore }
func (h fixAgentHost) Settings() *settings.Settings        { return h.settings.Load() }

// sessionHost is the Server as the sessions package, and the other packages
// that call into the Manager through their Host interfaces, see it. Its
// methods forward to the Manager or the owner of the call.
type sessionHost struct{ *Server }

// ListSessions reads every session dir, oldest first.
func (h sessionHost) ListSessions() ([]wire.SessionInfo, error) { return h.sessions.ListSessions() }

// SessionDir finds a session's dir and meta by id.
func (h sessionHost) SessionDir(id string) (string, sessions.Meta, wire.SessionResult, error) {
	return h.sessions.SessionDir(id)
}

// SessionFinished reports whether a finished session was a ticket agent that
// reported iteration_status finished.
func (h sessionHost) SessionFinished(sess agentrunner.Session) bool {
	return h.sessions.SessionFinished(sess)
}

func (h sessionHost) triggerBanner(id string, snd sound.Name, reason string) {
	h.sessions.TriggerBanner(id, snd, reason)
}
func (h sessionHost) noteFailure(id string)         { h.sessions.NoteFailure(id) }
func (h sessionHost) onMapEpic(address string) bool { return h.sessions.Maps.OnMapEpic(address) }
func (h sessionHost) sessionWriteDesignWeb(do func(wire.SessionRequest) (wire.SessionResult, error)) http.HandlerFunc {
	return h.sessions.SessionWriteDesignWeb(do)
}
func (h sessionHost) designLaunchTier(address string) ralphloop.Tier {
	return h.sessions.Maps.DesignLaunchTier(address)
}

// FindTicket is sessions.Host's lookup, served by the ticket writer.
func (h sessionHost) FindTicket(address string) (queue.TicketRef, bool, error) {
	return h.writer.FindTicket(address)
}

// CodexRolloutExists is the sessions rollouts lookup, served by the launcher.
func (h sessionHost) CodexRolloutExists(cwd, id string) bool {
	return h.launcher.CodexRolloutExists(cwd, id)
}
