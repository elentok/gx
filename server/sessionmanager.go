package server

import (
	"log/slog"
	"time"

	"github.com/elentok/gx/harness"
	"github.com/elentok/gx/osnotify"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/server/broker"
	harnesschoice "github.com/elentok/gx/server/harness"
	"github.com/elentok/gx/server/index"
	"github.com/elentok/gx/server/projects"
	"github.com/elentok/gx/server/queue"
	"github.com/elentok/gx/server/sessions/chat"
	"github.com/elentok/gx/server/settings"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/sound"
)

// SessionsConfig is the slice of the server Config the Manager reads.
type SessionsConfig struct {
	SessionsDir             string
	ConversationsDir        string
	TicketStore             string
	Env                     []string
	Holder                  []string
	Notifier                osnotify.Notifier
	Player                  sound.Player
	SessionBannerDelay      time.Duration
	SessionExitCleanupDelay time.Duration
}

// Host is what the Manager and the Resumer need from the rest of the root.
type Host interface {
	sessionInfo(m sessionMeta) wire.SessionInfo
	WebURL() string
	findTicket(address string) (ticketRef, bool, error)
	MigrationFailures() []wire.MigrationFailure
}

// Manager creates, stops, removes and watches sessions, and reads and writes
// their meta. It owns the watch goroutines (watches).
type Manager struct {
	cfg      SessionsConfig
	log      *slog.Logger
	events   *broker.Broker
	projects *projects.Registry
	settings *settings.Holder
	harness  *harnesschoice.Harness
	idx      *index.Index
	// chat reads the sessions' transcripts for the watch loop.
	chat  *chat.Chat
	focus *focusReports
	afk   *queue.AFK
	// alerts sends the AFK chat alert of a banner.
	alerts  *ralphloop.ServerChat
	banners sessionBanners
	host    Host
	watches *sessionWatches
	// prompter is the prompt, turn and auto-advance side the watch loop calls.
	prompter Prompter
	// designs is the design cleanup the remove path calls back.
	designs designCleaner
	// rollouts answers whether Codex still has a rollout for a session.
	rollouts codexRollouts
}

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

// Resumer brings sessions back: resume, restore, auto-resume, handoff, link,
// step and the fix agent.
type Resumer struct {
	mg   *Manager
	host Host
}

// newResumer needs s.sessions set.
func newResumer(s *Server) *Resumer {
	return &Resumer{mg: s.sessions, host: s}
}

// Prompter is what the session watch loop needs from the prompt, turn and
// auto-advance code. The root implements it; the move into server/sessions/prompt
// keeps the hook.
type Prompter interface {
	newFirstPrompt(dir, id string, run int) FirstPrompt
	markTurn(dir, id string) (turnMark, bool)
	settleTurn(dir, id string, mark turnMark, design designRef, cancel <-chan struct{})
	detectDesignActionable(dir, id string, design designRef)
	sendAction(dir, label string, action []string)
	cancelAutoAdvance(id, why string) bool
}

// FirstPrompt is the first-prompt tracker of one watch.
type FirstPrompt interface {
	observe(state string, ready harness.ReadySignal) string
	noteState(state string)
	personInput()
	start(signal, prompt string)
	stop()
}

// designCleaner is the designs back-call of the remove path.
type designCleaner interface {
	cleanupDesign(project, epic string)
}

// codexRollouts is the sched back-call of the resume path.
type codexRollouts interface {
	codexRolloutExists(cwd, id string) bool
}

// serverPrompter is the root's Prompter.
type serverPrompter struct{ s *Server }

func (p serverPrompter) newFirstPrompt(dir, id string, run int) FirstPrompt {
	return p.s.newFirstPrompt(dir, id, run)
}

func (p serverPrompter) markTurn(dir, id string) (turnMark, bool) { return p.s.markTurn(dir, id) }

func (p serverPrompter) settleTurn(dir, id string, mark turnMark, design designRef, cancel <-chan struct{}) {
	p.s.settleTurn(dir, id, mark, design, cancel)
}

func (p serverPrompter) detectDesignActionable(dir, id string, design designRef) {
	p.s.detectDesignActionable(dir, id, design)
}

func (p serverPrompter) sendAction(dir, label string, action []string) {
	p.s.sendAction(dir, label, action)
}

func (p serverPrompter) cancelAutoAdvance(id, why string) bool {
	return p.s.cancelAutoAdvance(id, why)
}
