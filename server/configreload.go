package server

import (
	"errors"
	"io/fs"
	"net/http"
	"os"
	"reflect"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/config"
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

// Reload triggers, as logged.
const (
	reloadTriggerCommand = "command"
)

func (s *Server) configReloadWrite(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, s.reloadConfig(reloadTriggerCommand))
}

// reloadConfig is the one reload path: read → parse → build → diff → apply →
// swap → kick → log. Live settings are untouched on any refusal.
func (s *Server) reloadConfig(trigger string) ReloadResult {
	s.reloadMu.Lock()
	defer s.reloadMu.Unlock()
	res := s.reloadLocked()
	s.logReload(trigger, res)
	return res
}

func (s *Server) reloadLocked() ReloadResult {
	if s.cfg.ConfigPath == "" {
		return refuseReload(ReasonReloadUnavailable, "this server has no config file to reload")
	}
	data, err := os.ReadFile(s.cfg.ConfigPath)
	if errors.Is(err, fs.ErrNotExist) {
		return refuseReload(ReasonConfigMissing, "config file "+s.cfg.ConfigPath+" does not exist; settings unchanged")
	}
	if err != nil {
		return refuseReload(ReasonInvalidConfig, err.Error())
	}
	cfg, err := config.Parse(data)
	if err == nil {
		err = agentrunner.ValidateSetting(cfg.AgentRunner)
	}
	if err != nil {
		return refuseReload(ReasonInvalidConfig, err.Error())
	}

	old := s.settings.Load()
	next := reloadableSettings(*old, SettingsFromConfig(cfg))
	res := ReloadResult{Changed: changedKeys(*old, next), Warnings: s.startupOnlyWarnings(cfg)}
	s.settings.Store(&next)
	if next.LogRetention != old.LogRetention {
		s.pruneAgentLogs(time.Now())
	}
	s.kickRunner()
	return res
}

// reloadableSettings is next with the fields reload does not apply yet
// (budget, chat, store commits) kept at their live values.
func reloadableSettings(old, next Settings) Settings {
	next.BudgetSoftLimit, next.BudgetHardLimit = old.BudgetSoftLimit, old.BudgetHardLimit
	next.Chat = old.Chat
	next.StoreCommitDebounce, next.StorePushRemote = old.StoreCommitDebounce, old.StorePushRemote
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
		{"server.tab-env", old.TabEnv, next.TabEnv},
		{"server.auto-merge-epic", old.AutoMergeEpic, next.AutoMergeEpic},
		{"server.log-retention", old.LogRetention, next.LogRetention},
		{"evolve", old.Evolve, next.Evolve},
		{"recovery.enabled", old.RecoverySettings.Enabled, next.RecoverySettings.Enabled},
		{"recovery.disabled", old.RecoverySettings.Disabled, next.RecoverySettings.Disabled},
		{"recovery.follow-ups", old.RecoverySettings.FollowUps, next.RecoverySettings.FollowUps},
		{"recovery.notify-hold", old.RecoverySettings.NotifyHold, next.RecoverySettings.NotifyHold},
		{"subscription.suppress-extra-usage-warning", old.SuppressExtraUsageWarning, next.SuppressExtraUsageWarning},
	}
	var keys []string
	for _, f := range fields {
		if !reflect.DeepEqual(f.old, f.next) {
			keys = append(keys, f.key)
		}
	}
	return keys
}

// startupOnlyWarnings compares the startup-only keys with the values the
// server started with, so a warning repeats on every reload until restart.
func (s *Server) startupOnlyWarnings(cfg config.Config) []string {
	var warnings []string
	if cfg.TicketStore.Path != s.cfg.TicketStore {
		warnings = append(warnings, "restart needed for ticket-store.path")
	}
	if cfg.Server.TCPListen != (s.cfg.TCPAddr != "") {
		warnings = append(warnings, "restart needed for server.tcp-listen")
	}
	if runnerChoice(cfg.AgentRunner) != runnerChoice(s.cfg.AgentRunner) {
		warnings = append(warnings, "restart needed for agent-runner")
	}
	return warnings
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

func (s *Server) logReload(trigger string, res ReloadResult) {
	if res.Refused {
		s.log.Warn("config reload refused", "trigger", trigger, "reason", res.Reason, "message", res.Message)
		return
	}
	s.log.Info("config reloaded", "trigger", trigger, "changed", res.Changed, "warnings", res.Warnings)
}
