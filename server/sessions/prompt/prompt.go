package prompt

import (
	"log/slog"
	"time"

	"github.com/elentok/gx/server/broker"
	"github.com/elentok/gx/server/sessions"
	"github.com/elentok/gx/server/sessions/chat"
	"github.com/elentok/gx/server/settings"
	"github.com/elentok/gx/server/wire"
)

// Config is the part of the server config the prompt code reads.
type Config struct {
	// AutoAdvanceDelay is the countdown before a run-end Continue is sent; zero
	// means DefaultAutoAdvanceDelay and a negative value turns auto-advance off.
	AutoAdvanceDelay time.Duration
	// TurnSettleTimeout caps the wait for claude's end-of-turn line; zero means
	// DefaultTurnSettleTimeout.
	TurnSettleTimeout time.Duration
}

// Host is what the prompt code calls back into the root for.
type Host interface {
	// SendAction runs the session's Continue action; auto marks a send the
	// server made.
	SendAction(req wire.SessionRequest, auto bool) (wire.SessionResult, error)
	// DetectDesignActionable recomputes the Actionable of a Design session.
	DetectDesignActionable(dir, id string, design chat.DesignRef)
}

// Deps is everything the prompt code copies when it is built. A field the root
// swaps later is not seen.
type Deps struct {
	Cfg         Config
	Log         *slog.Logger
	Events      *broker.Broker
	SessionChat *chat.Chat
	Settings    *settings.Holder
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
	host        Host
	// sessions comes later than Prompt: the Manager calls the Prompter back, so
	// it is built after Prompt and handed over by GiveSessions.
	sessions *sessions.Manager

	autoAdvances autoAdvances
}

// New builds Prompt from d.
func New(d Deps) *Prompt {
	return &Prompt{
		cfg: d.Cfg, log: d.Log, events: d.Events, sessionChat: d.SessionChat,
		settings: d.Settings, host: d.Host,
	}
}

// GiveSessions sets the Manager the prompt code reads and writes sessions through.
func (p *Prompt) GiveSessions(m *sessions.Manager) { p.sessions = m }
