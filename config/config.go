package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/elentok/gx/agentrunner"
)

// UserConfigDir hardcodes ~/.config as gx's config base directory on every
// platform, deliberately bypassing os.UserConfigDir's per-OS/XDG resolution
// (e.g. ~/Library/Application Support on macOS, or $XDG_CONFIG_HOME when
// set). It's the single source of truth other gx packages call into for
// this decision.
func UserConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config"), nil
}

// UserCacheDir hardcodes ~/.cache as gx's cache base directory on every
// platform, mirroring UserConfigDir's deliberate bypass of per-OS/XDG
// resolution.
func UserCacheDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".cache"), nil
}

// UserStateDir is the runtime-state base directory: ~/.local/state on every
// platform, or XDG_STATE_HOME when set. Runtime state (notifications-state.json)
// lives here rather than under UserConfigDir,
// which is reserved for user-edited config (config.json).
func UserStateDir() (string, error) {
	return xdgBase("XDG_STATE_HOME", ".local", "state")
}

var userConfigDirFn = UserConfigDir
var userStateDirFn = UserStateDir

const SchemaURL = "https://raw.githubusercontent.com/elentok/gx/main/docs/config-schema.json"

// Config is gx's user configuration.
type Config struct {
	Schema                string               `json:"$schema,omitempty"`
	UseNerdFontIcons      bool                 `json:"use-nerdfont-icons"`
	ImageDiffs            bool                 `json:"image-diffs"`
	StageDiffContextLines int                  `json:"stage-diff-context-lines"`
	InputModalBottom      InputModalBottom     `json:"input-modal-bottom"`
	NameAliases           map[string]string    `json:"name-aliases,omitempty"`
	Log                   LogConfig            `json:"log,omitempty"`
	ExecutionQueue        ExecutionQueueConfig `json:"execution-queue"`
	Budget                BudgetConfig         `json:"budget"`
	Notifications         NotificationsConfig  `json:"notifications"`
	Skills                SkillsConfig         `json:"skills"`
	Agents                AgentsConfig         `json:"agents"`
	Subscription          SubscriptionConfig   `json:"subscription"`
	TicketStore           TicketStoreConfig    `json:"ticket-store"`
	Server                ServerConfig         `json:"server"`
	Recovery              RecoveryConfig       `json:"recovery"`
	CPUWatchdog           CPUWatchdogConfig    `json:"cpu-watchdog"`
	// AgentRunner is auto, herdr, headless or pty. It is validated when the
	// server starts, not here, so a bad value never breaks the TUI.
	AgentRunner string `json:"agent-runner"`
	// RunnerWait is N, the wait every runner bounds its guards by. Read from
	// the "runner-wait" key as a Go duration string.
	RunnerWait time.Duration `json:"-"`
	// Evolve is off, maintainer or contributor. Unlike agent-runner it is
	// validated in Load: a guessed mode would file findings to the wrong place.
	Evolve string `json:"evolve"`
	// EvolveDeepReadCap bounds the deep-read subagents of one gx-evolve run.
	// The key is a flat dotted name because "evolve" is already a string.
	EvolveDeepReadCap int `json:"evolve.deep-read-cap"`
}

// RecoveryConfig controls the recovery catalog.
type RecoveryConfig struct {
	// Enabled is the global kill switch.
	Enabled bool `json:"enabled"`
	// Disabled lists catalog entry IDs to switch off.
	Disabled []string `json:"disabled,omitempty"`
	// FollowUps is the epic (project:epic) that receives the draft research
	// tickets an investigate ticket's report files. Empty means the default.
	FollowUps string `json:"follow-ups,omitempty"`
	// NotifyHold is how long a park's chat message waits for recovery before
	// it is sent anyway.
	NotifyHold time.Duration `json:"-"`
}

// DefaultRecoveryNotifyHold is the NotifyHold when none is configured.
const DefaultRecoveryNotifyHold = 10 * time.Minute

// ServerConfig configures the orchestrator server.
type ServerConfig struct {
	// TCPListen adds a loopback-only, unauthenticated TCP listener.
	TCPListen bool `json:"tcp-listen"`
	// TabEnv is KEY=VALUE entries set on every iteration tab's shell, for
	// overriding what the user's shell rc would otherwise decide (e.g. PATH).
	TabEnv []string `json:"tab-env"`
	// LogRetention is how long a headless agent's logs outlive its last
	// activity. Zero means the runner's default.
	LogRetention time.Duration `json:"-"`
	// WatchConfig reloads the config file whenever it is saved.
	WatchConfig bool `json:"watch-config"`
}

// Default returns the default configuration.
func Default() Config {
	return Config{
		UseNerdFontIcons:      true,
		ImageDiffs:            true,
		StageDiffContextLines: 1,
		InputModalBottom:      DefaultInputModalBottom(),
		Log:                   DefaultLogConfig(),
		ExecutionQueue:        DefaultExecutionQueueConfig(),
		Budget:                DefaultBudgetConfig(),
		Notifications:         DefaultNotificationsConfig(),
		Skills:                DefaultSkillsConfig(),
		Agents:                DefaultAgentsConfig(),
		Subscription:          DefaultSubscriptionConfig(),
		TicketStore:           DefaultTicketStoreConfig(),
		Server:                ServerConfig{WatchConfig: true},
		Recovery:              RecoveryConfig{Enabled: true, NotifyHold: DefaultRecoveryNotifyHold},
		CPUWatchdog:           DefaultCPUWatchdogConfig(),
		AgentRunner:           "auto",
		RunnerWait:            agentrunner.DefaultN,
		Evolve:                EvolveOff,
		EvolveDeepReadCap:     DefaultEvolveDeepReadCap,
	}
}

// LoadRunnerWait is N from the config file, or the default when the file is
// missing or unreadable. For runners built where no loaded Config is at hand.
func LoadRunnerWait() time.Duration {
	cfg, _ := Load()
	if cfg.RunnerWait <= 0 {
		return agentrunner.DefaultN
	}
	return cfg.RunnerWait
}

// FilePath returns the config file path, typically ~/.config/gx/config.json.
func FilePath() (string, error) {
	base, err := userConfigDirFn()
	if err != nil {
		return "", fmt.Errorf("resolve user config dir: %w", err)
	}
	return filepath.Join(base, "gx", "config.json"), nil
}

// Load reads user config from disk. Missing file returns defaults.
func Load() (Config, error) {
	cfg := Default()
	path, err := FilePath()
	if err != nil {
		return cfg, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, fmt.Errorf("read config %s: %w", path, err)
	}
	cfg, err = Parse(data)
	if err != nil {
		return cfg, fmt.Errorf("config %s: %w", path, err)
	}
	return cfg, nil
}

// Parse decodes config file bytes over the defaults and validates them.
func Parse(data []byte) (Config, error) {
	cfg := Default()
	var raw struct {
		UseNerdFontIcons      *bool             `json:"use-nerdfont-icons"`
		ImageDiffs            *bool             `json:"image-diffs"`
		StageDiffContextLines *int              `json:"stage-diff-context-lines"`
		InputModalBottom      *InputModalBottom `json:"input-modal-bottom"`
		NameAliases           map[string]string `json:"name-aliases"`
		Log                   *LogConfig        `json:"log"`
		ExecutionQueue        *struct {
			MaxAgentsPerEpic   *int    `json:"max-agents-per-epic"`
			MaxAgents          *int    `json:"max-agents"`
			RetryStormLaunches *int    `json:"retry-storm-launches"`
			SpinCycles         *int    `json:"spin-cycles"`
			SpinWindow         *string `json:"spin-window"`
		} `json:"execution-queue"`
		Budget *struct {
			SoftLimit              *float64  `json:"soft-limit"`
			HardLimit              *float64  `json:"hard-limit"`
			NotificationThresholds []float64 `json:"notification-thresholds"`
		} `json:"budget"`
		Notifications *struct {
			Telegram *struct {
				BotToken *string `json:"bot-token"`
				ChatID   *string `json:"chat-id"`
			} `json:"telegram"`
			Slack *struct {
				WebhookURL *string `json:"webhook-url"`
			} `json:"slack"`
		} `json:"notifications"`
		Skills *struct {
			Implement  *string  `json:"implement"`
			CodeReview []string `json:"code-review"`
		} `json:"skills"`
		Agents *struct {
			Claude *struct {
				Model  *string `json:"model"`
				Effort *string `json:"effort"`
			} `json:"claude"`
			Codex *struct {
				Model  *string `json:"model"`
				Effort *string `json:"effort"`
			} `json:"codex"`
			LeaderKey       json.RawMessage `json:"leader-key"`
			FocusSidebarKey json.RawMessage `json:"focus-sidebar-key"`
			ResumeOnStart   *bool           `json:"resume-on-start"`
			Banners         *struct {
				Enabled *bool `json:"enabled"`
			} `json:"banners"`
			Sounds *struct {
				Enabled   *bool   `json:"enabled"`
				Done      *string `json:"done"`
				Attention *string `json:"attention"`
			} `json:"sounds"`
			ClaudeArgs []string `json:"claude-args"`
		} `json:"agents"`
		Subscription *struct {
			SuppressExtraUsageWarning *bool `json:"suppress-extra-usage-warning"`
		} `json:"subscription"`
		TicketStore *struct {
			Path           *string `json:"path"`
			CommitDebounce *int    `json:"commit-debounce"`
			PushRemote     *string `json:"push-remote"`
		} `json:"ticket-store"`
		Server *struct {
			TCPListen    *bool    `json:"tcp-listen"`
			TabEnv       []string `json:"tab-env"`
			LogRetention *string  `json:"log-retention"`
			WatchConfig  *bool    `json:"watch-config"`
		} `json:"server"`
		AgentRunner       *string `json:"agent-runner"`
		RunnerWait        *string `json:"runner-wait"`
		Evolve            *string `json:"evolve"`
		EvolveDeepReadCap *int    `json:"evolve.deep-read-cap"`
		Recovery          *struct {
			Enabled    *bool    `json:"enabled"`
			Disabled   []string `json:"disabled"`
			FollowUps  *string  `json:"follow-ups"`
			NotifyHold *string  `json:"notify-hold"`
		} `json:"recovery"`
		CPUWatchdog *struct {
			Enabled          *bool    `json:"enabled"`
			ThresholdPercent *float64 `json:"threshold-percent"`
			Sustain          *string  `json:"sustain"`
			Cooldown         *string  `json:"cooldown"`
		} `json:"cpu-watchdog"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return cfg, fmt.Errorf("parse: %w", err)
	}
	if raw.UseNerdFontIcons != nil {
		cfg.UseNerdFontIcons = *raw.UseNerdFontIcons
	}
	if raw.ImageDiffs != nil {
		cfg.ImageDiffs = *raw.ImageDiffs
	}
	if raw.StageDiffContextLines != nil {
		cfg.StageDiffContextLines = clampStageDiffContext(*raw.StageDiffContextLines)
	}
	if raw.InputModalBottom != nil {
		cfg.InputModalBottom = *raw.InputModalBottom
	}
	if raw.NameAliases != nil {
		cfg.NameAliases = make(map[string]string, len(raw.NameAliases))
		for k, v := range raw.NameAliases {
			cfg.NameAliases[k] = v
		}
	}
	if raw.Log != nil {
		if raw.Log.ImportantRefs != nil {
			cfg.Log.ImportantRefs = raw.Log.ImportantRefs
		}
		if raw.Log.HideRefs != nil {
			cfg.Log.HideRefs = raw.Log.HideRefs
		}
	}
	if raw.ExecutionQueue != nil {
		if raw.ExecutionQueue.MaxAgentsPerEpic != nil {
			cfg.ExecutionQueue.MaxConcurrentTicketsPerEpic = clampExecutionQueueLimit(*raw.ExecutionQueue.MaxAgentsPerEpic)
		}
		if raw.ExecutionQueue.MaxAgents != nil {
			cfg.ExecutionQueue.MaxAgents = clampExecutionQueueLimit(*raw.ExecutionQueue.MaxAgents)
		}
		if raw.ExecutionQueue.RetryStormLaunches != nil {
			cfg.ExecutionQueue.RetryStormLaunches = clampExecutionQueueLimit(*raw.ExecutionQueue.RetryStormLaunches)
		}
		if raw.ExecutionQueue.SpinCycles != nil {
			cfg.ExecutionQueue.SpinCycles = clampExecutionQueueLimit(*raw.ExecutionQueue.SpinCycles)
		}
		if raw.ExecutionQueue.SpinWindow != nil {
			if d, err := time.ParseDuration(*raw.ExecutionQueue.SpinWindow); err == nil && d > 0 {
				cfg.ExecutionQueue.SpinWindow = d
			}
		}
	}
	if raw.Budget != nil {
		if raw.Budget.SoftLimit != nil {
			cfg.Budget.SoftLimit = *raw.Budget.SoftLimit
		}
		if raw.Budget.HardLimit != nil {
			cfg.Budget.HardLimit = *raw.Budget.HardLimit
		}
		if raw.Budget.NotificationThresholds != nil {
			cfg.Budget.NotificationThresholds = raw.Budget.NotificationThresholds
		}
		cfg.Budget = clampBudget(cfg.Budget)
	}
	if raw.Notifications != nil && raw.Notifications.Telegram != nil {
		if raw.Notifications.Telegram.BotToken != nil {
			cfg.Notifications.Telegram.BotToken = *raw.Notifications.Telegram.BotToken
		}
		if raw.Notifications.Telegram.ChatID != nil {
			cfg.Notifications.Telegram.ChatID = *raw.Notifications.Telegram.ChatID
		}
	}
	if raw.Notifications != nil && raw.Notifications.Slack != nil {
		if raw.Notifications.Slack.WebhookURL != nil {
			cfg.Notifications.Slack.WebhookURL = *raw.Notifications.Slack.WebhookURL
		}
	}
	if raw.Skills != nil {
		if raw.Skills.Implement != nil {
			cfg.Skills.Implement = *raw.Skills.Implement
		}
		if raw.Skills.CodeReview != nil {
			cfg.Skills.CodeReview = raw.Skills.CodeReview
		}
	}
	if raw.Agents != nil {
		if raw.Agents.Claude != nil {
			applyAgentConfig(&cfg.Agents.Claude, raw.Agents.Claude.Model, raw.Agents.Claude.Effort)
		}
		if raw.Agents.Codex != nil {
			applyAgentConfig(&cfg.Agents.Codex, raw.Agents.Codex.Model, raw.Agents.Codex.Effort)
		}
		if keys, ok := parseKeys(raw.Agents.LeaderKey); ok {
			cfg.Agents.LeaderKey = keys
		}
		if keys, ok := parseKeys(raw.Agents.FocusSidebarKey); ok {
			cfg.Agents.FocusSidebarKey = keys
		}
		if raw.Agents.ResumeOnStart != nil {
			cfg.Agents.ResumeOnStart = *raw.Agents.ResumeOnStart
		}
		if raw.Agents.Banners != nil && raw.Agents.Banners.Enabled != nil {
			cfg.Agents.Banners.Enabled = *raw.Agents.Banners.Enabled
		}
		if s := raw.Agents.Sounds; s != nil {
			if s.Enabled != nil {
				cfg.Agents.Sounds.Enabled = *s.Enabled
			}
			if s.Done != nil {
				cfg.Agents.Sounds.Done = strings.TrimSpace(*s.Done)
			}
			if s.Attention != nil {
				cfg.Agents.Sounds.Attention = strings.TrimSpace(*s.Attention)
			}
		}
		if raw.Agents.ClaudeArgs != nil {
			cfg.Agents.ClaudeArgs = raw.Agents.ClaudeArgs
		}
	}
	if raw.Subscription != nil && raw.Subscription.SuppressExtraUsageWarning != nil {
		cfg.Subscription.SuppressExtraUsageWarning = *raw.Subscription.SuppressExtraUsageWarning
	}
	if raw.TicketStore != nil && raw.TicketStore.Path != nil {
		cfg.TicketStore.Path = *raw.TicketStore.Path
	}
	if raw.TicketStore != nil && raw.TicketStore.CommitDebounce != nil && *raw.TicketStore.CommitDebounce > 0 {
		cfg.TicketStore.CommitDebounce = *raw.TicketStore.CommitDebounce
	}
	if raw.TicketStore != nil && raw.TicketStore.PushRemote != nil {
		cfg.TicketStore.PushRemote = *raw.TicketStore.PushRemote
	}
	if raw.Server != nil && raw.Server.TCPListen != nil {
		cfg.Server.TCPListen = *raw.Server.TCPListen
	}
	if raw.Server != nil && raw.Server.WatchConfig != nil {
		cfg.Server.WatchConfig = *raw.Server.WatchConfig
	}
	if raw.Server != nil && raw.Server.LogRetention != nil {
		if d, err := time.ParseDuration(*raw.Server.LogRetention); err == nil && d > 0 {
			cfg.Server.LogRetention = d
		}
	}
	if raw.Server != nil && raw.Server.TabEnv != nil {
		cfg.Server.TabEnv = raw.Server.TabEnv
	}
	if raw.AgentRunner != nil {
		cfg.AgentRunner = *raw.AgentRunner
	}
	if raw.RunnerWait != nil {
		if d, err := time.ParseDuration(*raw.RunnerWait); err == nil && d > 0 {
			cfg.RunnerWait = d
		}
	}
	if raw.Evolve != nil {
		if !validEvolveMode(*raw.Evolve) {
			return cfg, fmt.Errorf("evolve must be off, maintainer or contributor, got %q", *raw.Evolve)
		}
		cfg.Evolve = *raw.Evolve
	}
	if raw.EvolveDeepReadCap != nil {
		cfg.EvolveDeepReadCap = max(*raw.EvolveDeepReadCap, 1)
	}
	if raw.Recovery != nil {
		if raw.Recovery.Enabled != nil {
			cfg.Recovery.Enabled = *raw.Recovery.Enabled
		}
		if raw.Recovery.Disabled != nil {
			cfg.Recovery.Disabled = raw.Recovery.Disabled
		}
		if raw.Recovery.FollowUps != nil {
			cfg.Recovery.FollowUps = *raw.Recovery.FollowUps
		}
		if raw.Recovery.NotifyHold != nil {
			if d, err := time.ParseDuration(*raw.Recovery.NotifyHold); err == nil && d > 0 {
				cfg.Recovery.NotifyHold = d
			}
		}
	}
	if w := raw.CPUWatchdog; w != nil {
		if w.Enabled != nil {
			cfg.CPUWatchdog.Enabled = *w.Enabled
		}
		if w.ThresholdPercent != nil && *w.ThresholdPercent > 0 {
			cfg.CPUWatchdog.ThresholdPercent = *w.ThresholdPercent
		}
		if w.Sustain != nil {
			if d, err := time.ParseDuration(*w.Sustain); err == nil && d > 0 {
				cfg.CPUWatchdog.Sustain = d
			}
		}
		if w.Cooldown != nil {
			if d, err := time.ParseDuration(*w.Cooldown); err == nil && d >= 0 {
				cfg.CPUWatchdog.Cooldown = d
			}
		}
	}

	return cfg, nil
}

// applyAgentConfig overlays explicitly-set model/effort values onto an
// AgentConfig, trimming whitespace and leaving unset keys at the built-in
// default. A trimmed-empty value survives as empty (meaning "inherit"),
// distinct from an absent key (meaning "use the default").
func applyAgentConfig(cfg *AgentConfig, model, effort *string) {
	if model != nil {
		cfg.Model = strings.TrimSpace(*model)
	}
	if effort != nil {
		cfg.Effort = strings.TrimSpace(*effort)
	}
}

func clampStageDiffContext(n int) int {
	if n < 0 {
		return 0
	}
	if n > 20 {
		return 20
	}
	return n
}

// Init writes the default config file and returns its path.
// It returns an error if the file already exists.
func Init() (string, error) {
	path, err := FilePath()
	if err != nil {
		return "", err
	}

	if _, err := os.Stat(path); err == nil {
		return "", fmt.Errorf("config already exists at %s", path)
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("stat config %s: %w", path, err)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return "", fmt.Errorf("create config dir: %w", err)
	}

	cfg := Default()
	cfg.Schema = SchemaURL
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return "", fmt.Errorf("encode default config: %w", err)
	}
	b = append(b, '\n')

	if err := os.WriteFile(path, b, 0644); err != nil {
		return "", fmt.Errorf("write config %s: %w", path, err)
	}
	return path, nil
}
