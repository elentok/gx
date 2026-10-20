package server

import (
	"context"
	"errors"
	"io/fs"
	"net/http"
	"os"
	"reflect"
	"strings"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/config"
	"github.com/elentok/gx/osnotify"
)

// ReloadResult is the outcome of one config reload. A refusal is a result
// (Refused set), not an error (ADR 0028).
type ReloadResult struct {
	// Changed lists the config keys whose value changed, in config-key spelling.
	Changed  []string `json:"changed,omitempty"`
	Warnings []string `json:"warnings,omitempty"`
	Refused  bool     `json:"refused,omitempty"`
	Reason   string   `json:"reason,omitempty"`
	Message  string   `json:"message,omitempty"`
}

const (
	ReasonInvalidConfig     = "invalid-config"
	ReasonConfigMissing     = "config-missing"
	ReasonReloadUnavailable = "reload-unavailable"
)

// EventConfigReloaded reports one reload result; its Level and Message are set.
const EventConfigReloaded = "config-reloaded"

// Reload triggers, as logged.
const (
	reloadTriggerCommand = "command"
	reloadTriggerWatch   = "watch"
)

func (s *Server) configReloadWrite(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, s.reloadConfig(reloadTriggerCommand))
}

// reloadConfig is the one reload path: read → parse → build → diff → apply →
// swap → kick → log. Live settings are untouched on any refusal.
func (s *Server) reloadConfig(trigger string) ReloadResult {
	s.reloadMu.Lock()
	res, closeRetired := s.reloadLocked()
	s.reportReload(trigger, res)
	s.reloadMu.Unlock()
	if closeRetired != nil {
		go closeRetired()
	}
	return res
}

// reloadLocked returns, besides the result, a func that closes what the
// reload retired; it waits on the network, so it runs after the unlock.
func (s *Server) reloadLocked() (ReloadResult, func()) {
	if s.cfg.ConfigPath == "" {
		return refuseReload(ReasonReloadUnavailable, "this server has no config file to reload"), nil
	}
	data, err := os.ReadFile(s.cfg.ConfigPath)
	if errors.Is(err, fs.ErrNotExist) {
		return refuseReload(ReasonConfigMissing, "config file "+s.cfg.ConfigPath+" does not exist; settings unchanged"), nil
	}
	if err != nil {
		return refuseReload(ReasonInvalidConfig, err.Error()), nil
	}
	cfg, err := config.Parse(data)
	if err == nil {
		err = agentrunner.ValidateSetting(cfg.AgentRunner)
	}
	if err != nil {
		return refuseReload(ReasonInvalidConfig, err.Error()), nil
	}

	old := s.settings.Load()
	next := reloadableSettings(*old, SettingsFromConfig(cfg))
	pending := s.startupOnlyChanges(cfg)
	s.reloads.addPending(pending)
	res := ReloadResult{Changed: changedKeys(*old, next)}
	for _, key := range pending {
		res.Warnings = append(res.Warnings, "restart needed for "+key)
	}
	s.applyBudgetLimits(*old, next)
	var closeRetired func()
	if next.Chat != old.Chat {
		closeRetired = s.chat.SetGlobal(next.Chat)
	}
	if s.storeCommits != nil && (next.StoreCommitDebounce != old.StoreCommitDebounce || next.StorePushRemote != old.StorePushRemote) {
		s.storeCommits.SetSettings(next.storeCommitDebounce(), next.StorePushRemote)
	}
	s.settings.Store(&next)
	if next.WatchConfig != old.WatchConfig {
		s.setConfigWatchLocked(next.WatchConfig)
	}
	if next.LogRetention != old.LogRetention {
		s.pruneAgentLogs(time.Now())
	}
	s.kickRunner()
	return res, closeRetired
}

// applyBudgetLimits applies each budget limit whose file value changed since
// the last load to the ledger's live limit, then re-evaluates the latches as
// `gx budget increase` does. Comparing file values, not live ones, keeps a
// live increase on the field the person did not edit. A lowered hard limit
// already reached starts the grace countdown, whose start banner is the only
// report of it, so the reload's own banner stays a plain one; a raise that
// lifts the hold cancels it.
func (s *Server) applyBudgetLimits(old, next Settings) {
	softChanged := next.BudgetSoftLimit != old.BudgetSoftLimit
	hardChanged := next.BudgetHardLimit != old.BudgetHardLimit
	if !softChanged && !hardChanged {
		return
	}
	now := time.Now()
	soft, hard := s.ledger.limits()
	if softChanged {
		soft = next.BudgetSoftLimit
	}
	if hardChanged {
		held, spent := s.ledger.setLimitsHoldingHard(now, soft, next.BudgetHardLimit)
		if held {
			s.startHardGrace(now, next.BudgetHardLimit, spent)
		}
	} else {
		s.ledger.setLimits(now, soft, hard)
	}
	if err := s.ledger.save(now); err != nil {
		s.log.Error("save budget ledger", "err", err)
	}
	s.cancelHardGrace()
	s.rearmBudgetNotes(s.budgetStatus(now))
	s.events.publish(EventQueueChanged, "")
}

// reloadableSettings is next with the chat test seams, which are not config
// keys, carried over from the live settings.
func reloadableSettings(old, next Settings) Settings {
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
		{"budget.soft-limit", old.BudgetSoftLimit, next.BudgetSoftLimit},
		{"budget.hard-limit", old.BudgetHardLimit, next.BudgetHardLimit},
		{"server.tab-env", old.TabEnv, next.TabEnv},
		{"server.log-retention", old.LogRetention, next.LogRetention},
		{"server.watch-config", old.WatchConfig, next.WatchConfig},
		{"server.recently-finished", old.RecentlyFinished, next.RecentlyFinished},
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
		{"agents.resume-on-start", old.AgentsResumeOnStart, next.AgentsResumeOnStart},
		{"agents.banners.enabled", old.AgentsBanners, next.AgentsBanners},
		{"agents.sounds", old.AgentsSounds, next.AgentsSounds},
		{"agents.claude-args", old.AgentsClaudeArgs, next.AgentsClaudeArgs},
		{"agents.claude", old.AgentsClaude, next.AgentsClaude},
		{"agents.codex", old.AgentsCodex, next.AgentsCodex},
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
func (s *Server) startupOnlyChanges(cfg config.Config) []string {
	var keys []string
	if cfg.TicketStore.Path != s.cfg.TicketStore {
		keys = append(keys, "ticket-store.path")
	}
	if cfg.Server.TCPListen != (s.cfg.TCPAddr != "") {
		keys = append(keys, "server.tcp-listen")
	}
	if cfg.Server.Web.Listen != s.cfg.WebAddr {
		keys = append(keys, "server.web.listen")
	}
	if cfg.Server.Web.DevFrontendURL() != s.cfg.WebDevURL {
		keys = append(keys, "server.web.dev-frontend")
	}
	if runnerChoice(cfg.AgentRunner) != runnerChoice(s.cfg.AgentRunner) {
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

func refuseReload(reason, message string) ReloadResult {
	return ReloadResult{Refused: true, Reason: reason, Message: message}
}

// reportReload sends one result to the log, an OS banner and the event
// stream. Never to chat: a reload is something the person just did at the
// machine, not news worth a phone notification.
func (s *Server) reportReload(trigger string, res ReloadResult) {
	if res.Refused {
		s.log.Warn("config reload refused", "trigger", trigger, "reason", res.Reason, "message", res.Message)
	} else {
		s.log.Info("config reloaded", "trigger", trigger, "changed", res.Changed, "warnings", res.Warnings)
	}
	level, title, body := reloadBanner(res)
	s.reloads.setLast(ReloadRecord{At: time.Now(), Trigger: trigger, Outcome: level, Message: joinBanner(title, body)})
	s.banner(level, title, body)
}

// configBannerGroup makes each config banner replace the previous one.
const configBannerGroup = "gx-config"

// banner shows one OS banner and streams it as a config-reloaded event, for
// the TUI to toast at its level.
func (s *Server) banner(level, title, body string) {
	go func() {
		if err := s.cfg.Notifier.Notify(context.Background(), osnotify.Notification{Title: title, Body: body, Group: configBannerGroup}); err != nil {
			s.log.Warn("OS banner failed", "title", title, "error", err)
		}
	}()
	s.events.publishEvent(Event{Type: EventConfigReloaded, Level: level, Message: joinBanner(title, body)})
}

func joinBanner(title, body string) string {
	if body == "" {
		return title
	}
	return title + ": " + body
}

// maxBannerBody keeps a long parse error from overflowing the banner.
const maxBannerBody = 200

func reloadBanner(res ReloadResult) (level, title, body string) {
	switch {
	case res.Refused:
		body = res.Message
		if r := []rune(body); len(r) > maxBannerBody {
			body = string(r[:maxBannerBody-1]) + "…"
		}
		return EventLevelError, "gx config reload failed", body
	case len(res.Warnings) > 0:
		return EventLevelWarning, "gx config reloaded with warnings", strings.Join(res.Warnings, "; ")
	case len(res.Changed) > 0:
		return EventLevelSuccess, "gx config reloaded successfully", "changed: " + strings.Join(res.Changed, ", ")
	}
	return EventLevelSuccess, "gx config reloaded successfully", "no changes"
}
