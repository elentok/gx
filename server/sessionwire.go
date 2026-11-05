package server

import (
	"github.com/elentok/gx/server/sessions/chat"
)

// newManager copies the fields it needs from s at call time, so s must have
// them set (projects, harness and sessionChat come late in New).
func newManager(s *Server) *Manager {
	return &Manager{
		cfg: SessionsConfig{
			SessionsDir: s.cfg.SessionsDir, ConversationsDir: s.cfg.ConversationsDir,
			TicketStore: s.cfg.TicketStore, Env: s.cfg.Env, Holder: s.cfg.Holder,
			Notifier: s.cfg.Notifier, Player: s.cfg.Player,
			SessionBannerDelay: s.cfg.SessionBannerDelay, SessionExitCleanupDelay: s.cfg.SessionExitCleanupDelay,
		},
		log:      s.log,
		events:   s.events,
		projects: s.projects,
		settings: &s.settings,
		harness:  s.harness,
		idx:      s.idx,
		chat:     s.sessionChat,
		focus:    s.focus,
		afk:      s.afk,
		alerts:   s.chat,
		host:     s,
		watches:  newSessionWatches(),
		prompter: serverPrompter{s},
		designs:  s,
		rollouts: s,
	}
}

// newResumer needs s.sessions set.
func newResumer(s *Server) *Resumer {
	return &Resumer{mg: s.sessions, host: s}
}

// serverPrompter is the root's Prompter.
type serverPrompter struct{ s *Server }

func (p serverPrompter) NewFirstPrompt(dir, id string, run int) FirstPrompt {
	return p.s.newFirstPrompt(dir, id, run)
}

func (p serverPrompter) MarkTurn(dir, id string) (chat.TurnMark, bool) {
	return p.s.markTurn(dir, id)
}

func (p serverPrompter) SettleTurn(dir, id string, mark chat.TurnMark, design chat.DesignRef, cancel <-chan struct{}) {
	p.s.settleTurn(dir, id, mark, design, cancel)
}

func (p serverPrompter) DetectDesignActionable(dir, id string, design chat.DesignRef) {
	p.s.detectDesignActionable(dir, id, design)
}

func (p serverPrompter) SendAction(dir, label string, action []string) {
	p.s.sendAction(dir, label, action)
}

func (p serverPrompter) CancelAutoAdvance(id, why string) bool {
	return p.s.cancelAutoAdvance(id, why)
}
