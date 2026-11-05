package sessions

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
	focus *FocusReports
	afk   *queue.AFK
	// alerts sends the AFK chat alert of a banner.
	alerts *ralphloop.ServerChat
	// queue is the Queue whose live agents the reads show.
	queue   agentRows
	banners sessionBanners
	host    Host
	Watches *sessionWatches
	// Logs prunes the agent logs; Maps answers for the map epic of an address.
	Logs *AgentLogs
	Maps *MapEpics
	// prompter is the prompt, turn and auto-advance side the watch loop calls.
	prompter Prompter
	// designs is the design cleanup the remove path calls back.
	designs designCleaner
	// rollouts answers whether Codex still has a rollout for a session.
	rollouts codexRollouts
}

// ManagerDeps is what the root hands NewManager. Each field is the Manager
// field of the same name; the watches are made by NewManager.
type ManagerDeps struct {
	Cfg      SessionsConfig
	Log      *slog.Logger
	Events   *broker.Broker
	Projects *projects.Registry
	Settings *settings.Holder
	Harness  *harnesschoice.Harness
	Idx      *index.Index
	Chat     *chat.Chat
	Focus    *FocusReports
	AFK      *queue.AFK
	Alerts   *ralphloop.ServerChat
	Queue    agentRows
	Host     Host
	Prompter Prompter
	Designs  designCleaner
	Rollouts codexRollouts
}

// NewManager builds the Manager from d.
func NewManager(d ManagerDeps) *Manager {
	return &Manager{
		cfg: d.Cfg, log: d.Log, events: d.Events, projects: d.Projects,
		settings: d.Settings, harness: d.Harness, idx: d.Idx, chat: d.Chat,
		focus: d.Focus, afk: d.AFK, alerts: d.Alerts, queue: d.Queue, host: d.Host,
		Watches: NewSessionWatches(), prompter: d.Prompter,
		Logs:    &AgentLogs{ticketStore: d.Cfg.TicketStore, log: d.Log, settings: d.Settings, harness: d.Harness},
		Maps:    &MapEpics{projects: d.Projects},
		designs: d.Designs, rollouts: d.Rollouts,
	}
}

// Resumer brings sessions back: resume, restore, auto-resume, handoff, link,
// step and the fix agent.
type Resumer struct {
	mg   *Manager
	host Host
}

// NewResumer builds the Resumer over mg.
func NewResumer(mg *Manager, host Host) *Resumer {
	return &Resumer{mg: mg, host: host}
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
	AutoAdvanceDeadline(id string) time.Time
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
