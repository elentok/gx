package server

import (
	"github.com/elentok/gx/server/queue"
	"github.com/elentok/gx/server/sessions"
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
		Host:     s,
		Prompter: s.prompt,
		Designs:  s.designs,
		Rollouts: s,
	})
}

// FindTicket is sessions.Host's lookup, served by the ticket writer.
func (s *Server) FindTicket(address string) (queue.TicketRef, bool, error) {
	return s.writer.FindTicket(address)
}

// newResumer needs s.sessions set.
func newResumer(s *Server) *sessions.Resumer {
	return sessions.NewResumer(s.sessions, s)
}
