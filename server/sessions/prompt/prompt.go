package prompt

import (
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/elentok/gx/agentrunner/claudeui"
	"github.com/elentok/gx/server/broker"
	"github.com/elentok/gx/server/projects"
	"github.com/elentok/gx/server/sessions"
	"github.com/elentok/gx/server/sessions/chat"
	"github.com/elentok/gx/server/settings"
	"github.com/elentok/gx/server/wire"
)

// DefaultFirstPromptStartTimeout is how long a session may stay Starting,
// counted from launch, before its first prompt is handed to a person.
const DefaultFirstPromptStartTimeout = 30 * time.Second

const (
	// DefaultActionStepStartTimeout is how long an Action step waits for its
	// prompt to go Working.
	DefaultActionStepStartTimeout = 10 * time.Second
	// DefaultActionSlashSettle is how long a slash step stays Idle after
	// typing before it counts as done.
	DefaultActionSlashSettle = time.Second
	// DefaultKeysSettleStep and DefaultKeysSettleCap pace a keys request's
	// settle: two equal reads a step apart, or the cap.
	DefaultKeysSettleStep = 100 * time.Millisecond
	DefaultKeysSettleCap  = 1500 * time.Millisecond
)

// Errors that end an Action or a send without raising an Actionable.
var (
	ErrServerStopping = errors.New("server stopping")
	ErrWentBlocked    = errors.New("session went blocked")
)

// Config is the part of the server config the prompt code reads.
type Config struct {
	// AutoAdvanceDelay is the countdown before a run-end Continue is sent; zero
	// means DefaultAutoAdvanceDelay and a negative value turns auto-advance off.
	AutoAdvanceDelay time.Duration
	// TurnSettleTimeout caps the wait for claude's end-of-turn line; zero means
	// DefaultTurnSettleTimeout.
	TurnSettleTimeout time.Duration
	// TicketStore is the ticket-store root the prompt events are logged under.
	TicketStore string
	// FirstPromptStartTimeout bounds the Starting time of a pending first
	// prompt; zero means DefaultFirstPromptStartTimeout.
	FirstPromptStartTimeout time.Duration
	// PromptGrace bounds each wait for a typed prompt's turn to start; zero
	// means claudeui.DefaultNudgeCaps.Grace.
	PromptGrace time.Duration
	// LaunchProofCaps bound the check that a first slash prompt reached the
	// transcript; a zero field means the claudeui default.
	LaunchProofCaps claudeui.LaunchProofCaps
	// ActionStepStartTimeout bounds how long an Action step waits for its
	// prompt to go Working; zero means DefaultActionStepStartTimeout.
	ActionStepStartTimeout time.Duration
	// ActionSlashSettle is how long a slash step must stay Idle after typing
	// to count as done; zero means DefaultActionSlashSettle.
	ActionSlashSettle time.Duration
	// KeysSettleStep and KeysSettleCap are the read interval and the limit of
	// a keys request's settle; zero means the Default one.
	KeysSettleStep, KeysSettleCap time.Duration
	// CodexQuestionWait bounds the wait for each question's header on the
	// overlay; zero means DefaultCodexQuestionWait.
	CodexQuestionWait time.Duration
}

// RunOpts hooks an action run into a person's own input. OnTyped runs once a
// step's prompt is typed. Cancel, when closed, ends a step's wait without
// another Enter.
type RunOpts struct {
	OnTyped func()
	Cancel  <-chan struct{}
}

// BadRounds files the badly formed grilling rounds a Design session's last
// message holds.
type BadRounds interface {
	FileBadRoundBlocks(id string, design chat.DesignRef, text string)
}

// Deps is everything the prompt code copies when it is built. A field the root
// swaps later is not seen.
type Deps struct {
	Cfg         Config
	Log         *slog.Logger
	Events      *broker.Broker
	SessionChat *chat.Chat
	Settings    *settings.Holder
	Projects    *projects.Registry
	BadRounds   BadRounds
	// Held reports whether a Design (its `project:epic` key) is held. Nil is none.
	Held func(key string) bool
	// Queue is the Queue the auto-queue code queues epics through. Nil is none.
	Queue EpicQueue
	// OnTriageIdle runs when a Triage session of a project goes idle. Nil is none.
	OnTriageIdle func(project string)
}

// Prompt holds the auto-advance countdowns and the turn settling of the
// sessions.
type Prompt struct {
	// autoQueuer holds the auto-queue methods, which keeps Prompt's own method
	// count under the sizeguard limit. Its methods are promoted onto Prompt.
	*autoQueuer
	*imageStore
	*turnSettler

	cfg          Config
	log          *slog.Logger
	events       *broker.Broker
	sessionChat  *chat.Chat
	settings     *settings.Holder
	projects     *projects.Registry
	badRounds    BadRounds
	held         func(key string) bool
	queue        EpicQueue
	onTriageIdle func(project string)
	// sessions comes later than Prompt: the Manager calls the Prompter back, so
	// it is built after Prompt and handed over by GiveSessions.
	sessions *sessions.Manager
	// resumer is built from the Manager, so it is handed over by GiveResumer.
	resumer *sessions.Resumer
	// AnswerTicket is the ticket answer code a ticket-ask card is answered
	// through. The ticket ops come after Prompt, so the root sets it.
	AnswerTicket func(wire.QueueRequest) (wire.QueueResult, error)

	autoAdvances autoAdvances
	autoQueues   autoAdvances

	// imageMu orders the numbering of a session's Prompt images.
	imageMu sync.Mutex
}

// New builds Prompt from d.
func New(d Deps) *Prompt {
	p := &Prompt{
		cfg: d.Cfg, log: d.Log, events: d.Events, sessionChat: d.SessionChat,
		settings: d.Settings, projects: d.Projects, badRounds: d.BadRounds, held: d.Held, queue: d.Queue,
		onTriageIdle: d.OnTriageIdle,
	}
	p.autoQueuer = &autoQueuer{p}
	p.imageStore = &imageStore{p}
	p.turnSettler = &turnSettler{p}
	return p
}

// autoQueuer is the auto-queue half of Prompt: it reads and writes Prompt's
// state through the embedded pointer.
type autoQueuer struct{ *Prompt }

// GiveSessions sets the Manager the prompt code reads and writes sessions through.
func (p *Prompt) GiveSessions(m *sessions.Manager) { p.sessions = m }

// GiveResumer sets the Resumer a Continue is handed to.
func (p *Prompt) GiveResumer(r *sessions.Resumer) { p.resumer = r }
