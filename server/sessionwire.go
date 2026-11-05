package server

import (
	"github.com/elentok/gx/server/queue"
	"github.com/elentok/gx/server/sessions"
	"github.com/elentok/gx/server/sessions/chat"
	sessprompt "github.com/elentok/gx/server/sessions/prompt"
	"github.com/elentok/gx/server/wire"
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

// promptHost is the root's sessprompt.Host.
type promptHost struct{ s *Server }

func (h promptHost) SendAction(req wire.SessionRequest, auto bool) (wire.SessionResult, error) {
	return h.s.sendSessionAction(req, auto)
}

func (h promptHost) DetectDesignActionable(dir, id string, design chat.DesignRef) {
	h.s.detectDesignActionable(dir, id, design)
}

func (h promptHost) RunActionSteps(dir, label string, action []string) {
	h.s.sendAction(dir, label, action)
}

func (h promptHost) RaiseGxActionable(dir, reason string, action []string) {
	h.s.raiseGxActionable(dir, reason, action)
}

func (h promptHost) OpenActionRun(dir string, opts sessprompt.RunOpts) (sessprompt.ActionRun, func(), error) {
	run, closeRun, err := h.s.openActionRun(dir, actionRunOpts{onTyped: opts.OnTyped, cancel: opts.Cancel})
	if err != nil {
		return nil, nil, err
	}
	return runAdapter{run}, closeRun, nil
}
