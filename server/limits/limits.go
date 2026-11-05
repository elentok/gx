// Package limits owns the usage limits: the Claude limit store, the limit
// history, the per-harness usage samples and their gate and override, the usage
// summary and the burn stats.
package limits

import (
	"log/slog"
	"time"

	"github.com/elentok/gx/ralphloop/agent"
	"github.com/elentok/gx/server/broker"
	"github.com/elentok/gx/server/projects"
	"github.com/elentok/gx/server/runs"
	"github.com/elentok/gx/server/settings"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tokenledger"
)

// DayLayout is the local-day key format of the budget ledger, the stats days
// and the limit burn.
const DayLayout = "2006-01-02"

// Config is the part of the server config the limits read.
type Config struct {
	StateDir    string
	TicketStore string
	// UsagePollInterval is how often agent records are scanned; zero means one minute.
	UsagePollInterval time.Duration
	// LimitsEventInterval spaces limits-changed events; see broker.LimitsInterval.
	LimitsEventInterval time.Duration
	// TokenPollInterval is how often live sessions' tokens are metered; zero means DefaultPoll.
	TokenPollInterval time.Duration
}

// Deps is what the limits need from the root. SpendDays returns a copy of the
// budget ledger's per-day totals; the ledger stays in the root until budget moves.
// Tokens is the token ledger, shared with the stats, which only read it.
type Deps struct {
	Config    Config
	Settings  *settings.Holder
	Projects  *projects.Registry
	Events    *broker.Broker
	Guards    *runs.Guards
	Registry  *runs.Registry
	Tokens    *tokenledger.Ledger
	TokenHost TokenHost
	Log       *slog.Logger
	SpendDays func() map[string]float64
}

// Limits is the limit store, the history, the usage samples and their loop.
type Limits struct {
	cfg       Config
	settings  *settings.Holder
	projects  *projects.Registry
	events    *broker.Broker
	guards    *runs.Guards
	log       *slog.Logger
	spendDays func() map[string]float64

	registry  *runs.Registry
	tokens    *tokenledger.Ledger
	tokenHost TokenHost
	tokenPoll tokenPoller

	store      *limitsStore
	history    *limitHistory
	usage      *usageSamples // Claude's
	usageCodex *usageSamples
	throttle   *broker.Throttle
}

// New opens the persisted state in the state dir.
func New(d Deps) *Limits {
	return &Limits{
		cfg: d.Config, settings: d.Settings, projects: d.Projects, events: d.Events, guards: d.Guards, log: d.Log,
		spendDays: d.SpendDays,
		registry:  d.Registry, tokens: d.Tokens, tokenHost: d.TokenHost,
		store:      openLimitsStore(d.Config.StateDir),
		history:    openLimitHistory(d.Config.StateDir),
		usage:      openUsageSamples(d.Config.StateDir),
		usageCodex: openUsageFiles(d.Config.StateDir, usageSampleFileCodex, usageLiftFileCodex),
		throttle: broker.NewThrottle(broker.LimitsInterval(d.Config.LimitsEventInterval),
			func() { d.Events.Publish(wire.EventLimitsChanged, "") }),
	}
}

// Stop ends the limits-changed throttle.
func (l *Limits) Stop() { l.throttle.Stop() }

// IsCodexRun reports a run on a harness other than Claude (today only Codex).
// Only Claude has a dollar cost for the budget to count. An unset agent counts
// as Claude.
func IsCodexRun(r runs.Run) bool {
	return r.Agent != "" && r.Agent != string(agent.AgentClaude)
}
