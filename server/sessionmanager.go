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
	SessionInfo(m sessionMeta) wire.SessionInfo
	WebURL() string
	FindTicket(address string) (queue.TicketRef, bool, error)
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

// Resumer brings sessions back: resume, restore, auto-resume, handoff, link,
// step and the fix agent.
type Resumer struct {
	mg   *Manager
	host Host
}

// Prompter is what the session watch loop needs from the prompt, turn and
// auto-advance code. The root implements it; the move into server/sessions/prompt
// keeps the hook.
type Prompter interface {
	NewFirstPrompt(dir, id string, run int) FirstPrompt
	MarkTurn(dir, id string) (chat.TurnMark, bool)
	SettleTurn(dir, id string, mark chat.TurnMark, design chat.DesignRef, cancel <-chan struct{})
	DetectDesignActionable(dir, id string, design chat.DesignRef)
	SendAction(dir, label string, action []string)
	CancelAutoAdvance(id, why string) bool
}

// FirstPrompt is the first-prompt tracker of one watch.
type FirstPrompt interface {
	Observe(state string, ready harness.ReadySignal) string
	NoteState(state string)
	PersonInput()
	Start(signal, prompt string)
	Stop()
}

// designCleaner is the designs back-call of the remove path.
type designCleaner interface {
	CleanupDesign(project, epic string)
}

// codexRollouts is the sched back-call of the resume path.
type codexRollouts interface {
	CodexRolloutExists(cwd, id string) bool
}
