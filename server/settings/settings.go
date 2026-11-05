// Package settings holds the server's live, reloadable settings: the atomic
// snapshot, the config reload, the reload status and the config file watcher.
// It imports no other server package except wire. What a reload applies
// outside it (budget, chat, certs, log prune, the runner kick) comes in as
// Appliers the root registers.
package settings

import (
	"cmp"
	"net"
	"time"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/harness"
	"github.com/elentok/gx/ralphloop/notify"
	"github.com/elentok/gx/recovery"
)

// Settings are the Config fields a config reload may change. The server reads
// them through an atomic snapshot (Holder.Load), never from Config.
type Settings struct {
	TabEnv []string // KEY=VALUE entries set on each iteration tab
	// Evolve is the global evolve mode (config.Evolve*); empty means off.
	Evolve string

	// LogRetention is how long a headless agent's logs are kept after its last
	// activity; zero means nativerunner.DefaultLogRetention.
	LogRetention time.Duration

	// BudgetSoftLimit and BudgetHardLimit are config.Budget's daily limits in
	// dollars, reported in the budget status; zero means off.
	BudgetSoftLimit float64
	BudgetHardLimit float64

	// LongTicketTurns is config.Budget's long-ticket alert threshold in
	// assistant turns; zero means off.
	LongTicketTurns int

	// Recovery is the catalog with the user's kill switch and disables applied.
	// The zero value is off.
	Recovery recovery.Catalog
	// RecoverySettings is the loaded config recovery block, defaults applied.
	// An empty FollowUps means DefaultFollowUps; a zero NotifyHold sends a
	// pause's chat message without waiting for recovery.
	RecoverySettings config.RecoveryConfig

	// StoreCommitDebounce and StorePushRemote configure the store commit loop,
	// A zero debounce means 60s.
	StoreCommitDebounce time.Duration
	StorePushRemote     string

	// MaxAgents caps live agents across all projects (config
	// execution-queue.max-agents); zero means the config default.
	MaxAgents int
	// MaxAgentsPerRoot caps live agents of one root (config
	// execution-queue.max-agents-per-epic). Zero means the default.
	MaxAgentsPerRoot int
	// MaxOneOffAgents caps live agents across one project's one-offs (config
	// execution-queue.max-one-off-agents).
	MaxOneOffAgents int
	// WaitingSlotGrace and MaxReleasedAgents are config
	// execution-queue.waiting-slot-grace and .max-released-agents; zero grace
	// means the config default.
	WaitingSlotGrace  time.Duration
	MaxReleasedAgents int
	// StalledNudgeDelay is config execution-queue.stalled-nudge-delay; zero
	// means the config default.
	StalledNudgeDelay time.Duration

	// Chat names the chat destinations the server notifies; empty means none.
	Chat notify.ServerChatConfig

	// SuppressExtraUsageWarning is config subscription.suppress-extra-usage-warning.
	SuppressExtraUsageWarning bool

	// WatchConfig reloads the config file whenever it is saved.
	WatchConfig bool

	// The interactive-session keys of config agents.*.
	AgentsResumeOnStart bool
	AgentsBanners       bool
	AgentsSounds        config.AgentsSoundsConfig
	AgentsClaudeArgs    []string
	// AgentsFixAgentRemoteControl is read at each fix-agent start.
	AgentsFixAgentRemoteControl bool

	// CPUWatchdog is config cpu-watchdog: when a CPU spike files a ticket.
	CPUWatchdog config.CPUWatchdogConfig

	// DiskSpace is config disk-space: the free-space thresholds and cleanup
	// command. Zero thresholds mean off, so hand-built Settings stay off.
	DiskSpace config.DiskSpaceConfig

	// RecentlyFinished is config server.recently-finished: how many completed
	// epics the snapshot lists. Zero lists none; the config layer supplies
	// config.DefaultRecentlyFinished when the key is unset.
	RecentlyFinished int

	// AgentsClaude and AgentsCodex are the global model and effort of config
	// agents.*, before a project's overrides.
	AgentsClaude config.AgentConfig
	AgentsCodex  config.AgentConfig
	// AgentsDefaultHarness is config agents.default-harness; empty means claude.
	AgentsDefaultHarness string

	// WebAllowedHosts is config server.web.allowed-hosts: names, besides IP
	// literals and localhost, that the web listener accepts in Host.
	WebAllowedHosts []string

	// WebTOTPOff is config server.web.totp turned off. Inverted so a zero
	// Settings keeps TOTP on.
	WebTOTPOff bool

	// WebInterfaceIPs lists the local interface IPs the web URL may fall back
	// to; nil reads the machine's interfaces. Tests inject it.
	WebInterfaceIPs func() []net.IP

	// ChatHarness names the harness whose transcripts the session chat reads;
	// empty means claude. Not a config key: servertest points it at the fake
	// harness.
	ChatHarness string

	// RemoteControlHarness names the harness a fix-agent session may start
	// with --remote-control; empty means claude. Not a config key: servertest
	// points it at the fake harness, whose launch args stay unflagged.
	RemoteControlHarness string
}

// FromConfig maps the user config to the server's reloadable settings.
// Startup and reload both use it, so the two never disagree.
func FromConfig(cfg config.Config) Settings {
	return Settings{
		TabEnv:              cfg.Server.TabEnv,
		Evolve:              cfg.Evolve,
		LogRetention:        cfg.Server.LogRetention,
		BudgetSoftLimit:     cfg.Budget.SoftLimit,
		BudgetHardLimit:     cfg.Budget.HardLimit,
		LongTicketTurns:     cfg.Budget.LongTicketTurns,
		Recovery:            recovery.Default().WithConfig(cfg.Recovery.Enabled, cfg.Recovery.Disabled),
		RecoverySettings:    cfg.Recovery,
		StoreCommitDebounce: time.Duration(cfg.TicketStore.CommitDebounce) * time.Second,
		StorePushRemote:     cfg.TicketStore.PushRemote,
		MaxAgents:           cfg.ExecutionQueue.MaxAgents,
		MaxAgentsPerRoot:    cfg.ExecutionQueue.MaxConcurrentTicketsPerEpic,
		MaxOneOffAgents:     cfg.ExecutionQueue.MaxOneOffAgents,
		WaitingSlotGrace:    cfg.ExecutionQueue.WaitingSlotGrace,
		MaxReleasedAgents:   cfg.ExecutionQueue.MaxReleasedAgents,
		StalledNudgeDelay:   cfg.ExecutionQueue.StalledNudgeDelay,
		Chat: notify.ServerChatConfig{
			TelegramBotToken: cfg.Notifications.Telegram.BotToken,
			TelegramChatID:   cfg.Notifications.Telegram.ChatID,
			SlackWebhookURL:  cfg.Notifications.Slack.WebhookURL,
		},
		SuppressExtraUsageWarning:   cfg.Subscription.SuppressExtraUsageWarning,
		WatchConfig:                 cfg.Server.WatchConfig,
		AgentsResumeOnStart:         cfg.Agents.ResumeOnStart,
		AgentsBanners:               cfg.Agents.Banners.Enabled,
		AgentsSounds:                cfg.Agents.Sounds,
		AgentsClaudeArgs:            cfg.Agents.ClaudeArgs,
		AgentsFixAgentRemoteControl: cfg.Agents.FixAgentRemoteControl,
		CPUWatchdog:                 cfg.CPUWatchdog,
		DiskSpace:                   cfg.DiskSpace,
		RecentlyFinished:            cfg.Server.RecentlyFinished,
		AgentsClaude:                cfg.Agents.Claude,
		AgentsCodex:                 cfg.Agents.Codex,
		AgentsDefaultHarness:        cfg.Agents.DefaultHarness,
		WebAllowedHosts:             cfg.Server.Web.AllowedHosts,
		WebTOTPOff:                  !cfg.Server.Web.TOTP,
	}
}

// HasChatReader says whether sessions of harnessName have a chat reader: Codex,
// and the chat harness, which reads the Claude format.
func (s Settings) HasChatReader(harnessName string) bool {
	return harnessName == string(harness.Codex) || harnessName == cmp.Or(s.ChatHarness, string(harness.Claude))
}

func (s Settings) storeCommitDebounce() time.Duration {
	if s.StoreCommitDebounce <= 0 {
		return time.Duration(config.DefaultCommitDebounceSeconds) * time.Second
	}
	return s.StoreCommitDebounce
}
