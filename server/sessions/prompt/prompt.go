package prompt

import (
	"errors"
	"log/slog"
	"time"

	"github.com/elentok/gx/agentrunner/claudeui"
	"github.com/elentok/gx/nativerunner"
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
}

// RunOpts hooks an action run into a person's own input. OnTyped runs once a
// step's prompt is typed. Cancel, when closed, ends a step's wait without
// another Enter.
type RunOpts struct {
	OnTyped func()
	Cancel  <-chan struct{}
}

// ActionRun follows one session's status on its own watch while prompts are
// typed into it.
type ActionRun interface {
	// CatchUp applies the statuses already sent.
	CatchUp() error
	// Step types one prompt and waits for it to end.
	Step(i int, prompt string) error
	// TypeStep types one prompt and presses its Enter, without waiting.
	TypeStep(i int, prompt string) error
	// State is the session state the run last saw.
	State() string
	// Holder is the session's holder client.
	Holder() *nativerunner.HolderClient
	// PersonTyped reports whether a person's own input reached the session;
	// ClearPersonTyped forgets input from before gx typed.
	PersonTyped() bool
	ClearPersonTyped()
	// NextStatus blocks for the next event of a step.
	NextStatus(timer <-chan time.Time) (expired bool, signal string, err error)
	// TurnRunning reports whether a turn is running.
	TurnRunning() bool
}

// Host is what the prompt code calls back into the root for.
type Host interface {
	// SendAction runs the session's Continue action; auto marks a send the
	// server made.
	SendAction(req wire.SessionRequest, auto bool) (wire.SessionResult, error)
	// DetectDesignActionable recomputes the Actionable of a Design session.
	DetectDesignActionable(dir, id string, design chat.DesignRef)
	// RunActionSteps sends an Action's prompts one by one, raising an
	// Actionable named after label for the ones it could not send.
	RunActionSteps(dir, label string, action []string)
	// RaiseGxActionable saves an Actionable gx raises itself.
	RaiseGxActionable(dir, reason string, action []string)
	// OpenActionRun dials the session's holder and follows its watch; the
	// returned func ends the watch.
	OpenActionRun(dir string, opts RunOpts) (ActionRun, func(), error)
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
	Host        Host
}

// Prompt holds the auto-advance countdowns and the turn settling of the
// sessions.
type Prompt struct {
	cfg         Config
	log         *slog.Logger
	events      *broker.Broker
	sessionChat *chat.Chat
	settings    *settings.Holder
	projects    *projects.Registry
	host        Host
	// sessions comes later than Prompt: the Manager calls the Prompter back, so
	// it is built after Prompt and handed over by GiveSessions.
	sessions *sessions.Manager
	// resumer is built from the Manager, so it is handed over by GiveResumer.
	resumer *sessions.Resumer

	autoAdvances autoAdvances
}

// New builds Prompt from d.
func New(d Deps) *Prompt {
	return &Prompt{
		cfg: d.Cfg, log: d.Log, events: d.Events, sessionChat: d.SessionChat,
		settings: d.Settings, projects: d.Projects, host: d.Host,
	}
}

// GiveSessions sets the Manager the prompt code reads and writes sessions through.
func (p *Prompt) GiveSessions(m *sessions.Manager) { p.sessions = m }

// GiveResumer sets the Resumer a Continue is handed to.
func (p *Prompt) GiveResumer(r *sessions.Resumer) { p.resumer = r }
