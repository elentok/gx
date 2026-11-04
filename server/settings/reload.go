package settings

import (
	"errors"
	"io/fs"
	"net/http"
	"os"
	"reflect"
	"strings"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/config"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/server/wire"
)

// Reload triggers, as logged.
const (
	TriggerCommand = "command"
	TriggerWatch   = "watch"
)

// Reload is the one reload path: read → parse → build → diff → apply →
// swap → kick → log. Live settings are untouched on any refusal.
func (h *Holder) Reload(trigger string) wire.ReloadResult {
	h.mu.Lock()
	res, closeRetired := h.reloadLocked()
	h.reportReload(trigger, res)
	h.mu.Unlock()
	if closeRetired != nil {
		go closeRetired()
	}
	return res
}

// reloadLocked returns, besides the result, a func that closes what the
// reload retired; it waits on the network, so it runs after the unlock.
func (h *Holder) reloadLocked() (wire.ReloadResult, func()) {
	if h.opts.ConfigPath == "" {
		return refuseReload(wire.ReasonReloadUnavailable, "this server has no config file to reload"), nil
	}
	data, err := os.ReadFile(h.opts.ConfigPath)
	if errors.Is(err, fs.ErrNotExist) {
		return refuseReload(wire.ReasonConfigMissing, "config file "+h.opts.ConfigPath+" does not exist; settings unchanged"), nil
	}
	if err != nil {
		return refuseReload(wire.ReasonInvalidConfig, err.Error()), nil
	}
	cfg, err := config.Parse(data)
	if err == nil {
		err = agentrunner.ValidateSetting(cfg.AgentRunner)
	}
	if err != nil {
		return refuseReload(wire.ReasonInvalidConfig, err.Error()), nil
	}

	old := h.Load()
	next := reloadableSettings(*old, FromConfig(cfg))
	pending := h.startupOnlyChanges(cfg)
	h.history.addPending(pending)
	res := wire.ReloadResult{Changed: changedKeys(*old, next)}
	for _, key := range pending {
		res.Warnings = append(res.Warnings, "restart needed for "+key)
	}
	closers := h.appliers.run(*old, next, func() { h.Store(&next) })
	return res, joinClosers(closers)
}

// Applier applies one part of a reload. It runs with the reload lock held and
// may return a func that closes what it retired; that runs after the unlock.
type Applier func(old, next Settings) (closeRetired func())

// Appliers are the reload's apply steps. Before runs ahead of the settings
// swap, in registration order; After runs behind it.
type Appliers struct {
	Before, After []Applier
}

// run applies one reload: Before, then swap, then After. It returns the
// retired-resource closers in the order they were produced.
func (a Appliers) run(old, next Settings, swap func()) []func() {
	var closers []func()
	apply := func(list []Applier) {
		for _, fn := range list {
			if c := fn(old, next); c != nil {
				closers = append(closers, c)
			}
		}
	}
	apply(a.Before)
	swap()
	apply(a.After)
	return closers
}

func joinClosers(closers []func()) func() {
	if len(closers) == 0 {
		return nil
	}
	return func() {
		for _, c := range closers {
			c()
		}
	}
}

// reloadableSettings is next with the chat test seams, which are not config
// keys, carried over from the live settings.
func reloadableSettings(old, next Settings) Settings {
	next.ChatHarness, next.RemoteControlHarness = old.ChatHarness, old.RemoteControlHarness
	// Config only holds claude or codex, so any other default is a test's fake
	// harness and survives a reload like the other seams.
	if old.AgentsDefaultHarness != "" && ralphloop.ValidateAgentKind(ralphloop.AgentKind(old.AgentsDefaultHarness)) != nil {
		next.AgentsDefaultHarness = old.AgentsDefaultHarness
	}
	next.Chat.TelegramBaseURL, next.Chat.GateStatePath, next.Chat.FlushInterval = old.Chat.TelegramBaseURL, old.Chat.GateStatePath, old.Chat.FlushInterval
	return next
}

// changedKeys names the applied config keys whose value differs.
func changedKeys(old, next Settings) []string {
	fields := []struct {
		key       string
		old, next any
	}{
		{"execution-queue.max-agents", old.MaxAgents, next.MaxAgents},
		{"execution-queue.max-agents-per-epic", old.MaxAgentsPerRoot, next.MaxAgentsPerRoot},
		{"execution-queue.max-one-off-agents", old.MaxOneOffAgents, next.MaxOneOffAgents},
		{"execution-queue.waiting-slot-grace", old.WaitingSlotGrace, next.WaitingSlotGrace},
		{"execution-queue.max-released-agents", old.MaxReleasedAgents, next.MaxReleasedAgents},
		{"execution-queue.stalled-nudge-delay", old.StalledNudgeDelay, next.StalledNudgeDelay},
		{"budget.soft-limit", old.BudgetSoftLimit, next.BudgetSoftLimit},
		{"budget.hard-limit", old.BudgetHardLimit, next.BudgetHardLimit},
		{"disk-space.warn-below", old.DiskSpace.WarnBelow, next.DiskSpace.WarnBelow},
		{"disk-space.clean-below", old.DiskSpace.CleanBelow, next.DiskSpace.CleanBelow},
		{"disk-space.cleanup-command", old.DiskSpace.CleanupCommand, next.DiskSpace.CleanupCommand},
		{"server.tab-env", old.TabEnv, next.TabEnv},
		{"server.log-retention", old.LogRetention, next.LogRetention},
		{"server.watch-config", old.WatchConfig, next.WatchConfig},
		{"server.recently-finished", old.RecentlyFinished, next.RecentlyFinished},
		{"server.web.allowed-hosts", old.WebAllowedHosts, next.WebAllowedHosts},
		{"server.web.totp", old.WebTOTPOff, next.WebTOTPOff},
		{"evolve", old.Evolve, next.Evolve},
		{"recovery.enabled", old.RecoverySettings.Enabled, next.RecoverySettings.Enabled},
		{"recovery.disabled", old.RecoverySettings.Disabled, next.RecoverySettings.Disabled},
		{"recovery.follow-ups", old.RecoverySettings.FollowUps, next.RecoverySettings.FollowUps},
		{"recovery.notify-hold", old.RecoverySettings.NotifyHold, next.RecoverySettings.NotifyHold},
		{"subscription.suppress-extra-usage-warning", old.SuppressExtraUsageWarning, next.SuppressExtraUsageWarning},
		{"notifications.telegram.bot-token", old.Chat.TelegramBotToken, next.Chat.TelegramBotToken},
		{"notifications.telegram.chat-id", old.Chat.TelegramChatID, next.Chat.TelegramChatID},
		{"notifications.slack.webhook-url", old.Chat.SlackWebhookURL, next.Chat.SlackWebhookURL},
		{"ticket-store.commit-debounce", old.StoreCommitDebounce, next.StoreCommitDebounce},
		{"ticket-store.push-remote", old.StorePushRemote, next.StorePushRemote},
		{"agents.restore-sessions-on-start", old.AgentsResumeOnStart, next.AgentsResumeOnStart},
		{"agents.banners.enabled", old.AgentsBanners, next.AgentsBanners},
		{"agents.sounds", old.AgentsSounds, next.AgentsSounds},
		{"agents.claude-args", old.AgentsClaudeArgs, next.AgentsClaudeArgs},
		{"agents.fix-agent-remote-control", old.AgentsFixAgentRemoteControl, next.AgentsFixAgentRemoteControl},
		{"agents.claude", old.AgentsClaude, next.AgentsClaude},
		{"agents.codex", old.AgentsCodex, next.AgentsCodex},
		{"agents.default-harness", old.AgentsDefaultHarness, next.AgentsDefaultHarness},
	}
	var keys []string
	for _, f := range fields {
		if !reflect.DeepEqual(f.old, f.next) {
			keys = append(keys, f.key)
		}
	}
	return keys
}

// startupOnlyChanges compares the startup-only keys with the values the
// server started with, so a warning repeats on every reload until restart.
func (h *Holder) startupOnlyChanges(cfg config.Config) []string {
	start := h.opts.Startup
	var keys []string
	if cfg.TicketStore.Path != start.TicketStore {
		keys = append(keys, "ticket-store.path")
	}
	if cfg.Server.TCPListen != start.TCPListen {
		keys = append(keys, "server.tcp-listen")
	}
	if cfg.Server.Web.Listen != start.WebAddr {
		keys = append(keys, "server.web.listen")
	}
	if cfg.Server.Web.DevFrontendURL() != start.WebDevURL {
		keys = append(keys, "server.web.dev-frontend")
	}
	if runnerChoice(cfg.AgentRunner) != runnerChoice(start.AgentRunner) {
		keys = append(keys, "agent-runner")
	}
	return keys
}

// runnerChoice treats an unset agent-runner as auto, its documented default.
func runnerChoice(setting string) agentrunner.Choice {
	if setting == "" {
		return agentrunner.ChoiceAuto
	}
	return agentrunner.Choice(setting)
}

func refuseReload(reason, message string) wire.ReloadResult {
	return wire.ReloadResult{Refused: true, Reason: reason, Message: message}
}

// reportReload sends one result to the log, an OS banner and the event
// stream. Never to chat: a reload is something the person just did at the
// machine, not news worth a phone notification.
func (h *Holder) reportReload(trigger string, res wire.ReloadResult) {
	if res.Refused {
		h.opts.Log.Warn("config reload refused", "trigger", trigger, "reason", res.Reason, "message", res.Message)
	} else {
		h.opts.Log.Info("config reloaded", "trigger", trigger, "changed", res.Changed, "warnings", res.Warnings)
	}
	level, title, body := reloadBanner(res)
	h.history.setLast(wire.ReloadRecord{At: time.Now(), Trigger: trigger, Outcome: level, Message: joinBanner(title, body)})
	h.Banner(level, title, body)
}

func joinBanner(title, body string) string {
	if body == "" {
		return title
	}
	return title + ": " + body
}

// maxBannerBody keeps a long parse error from overflowing the banner.
const maxBannerBody = 200

func reloadBanner(res wire.ReloadResult) (level, title, body string) {
	switch {
	case res.Refused:
		body = res.Message
		if r := []rune(body); len(r) > maxBannerBody {
			body = string(r[:maxBannerBody-1]) + "…"
		}
		return wire.EventLevelError, "gx config reload failed", body
	case len(res.Warnings) > 0:
		return wire.EventLevelWarning, "gx config reloaded with warnings", strings.Join(res.Warnings, "; ")
	case len(res.Changed) > 0:
		return wire.EventLevelSuccess, "gx config reloaded successfully", "changed: " + strings.Join(res.Changed, ", ")
	}
	return wire.EventLevelSuccess, "gx config reloaded successfully", "no changes"
}

// Routes are the config endpoints the root serves. h is resolved per request,
// so the list can be built before the holder is wired.
func Routes(h func() *Holder) []wire.Route {
	return []wire.Route{
		{Pattern: "POST /v1/config/reload", Handler: func(w http.ResponseWriter, _ *http.Request) {
			wire.WriteJSON(w, h().Reload(TriggerCommand))
		}},
		{Pattern: "GET /v1/config/status", Handler: func(w http.ResponseWriter, _ *http.Request) {
			wire.WriteJSON(w, h().Status())
		}},
	}
}
