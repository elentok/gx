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
func UserConfigDir() (string, error) { return DirsFrom(os.Environ()).UserConfigDir() }

// UserCacheDir is DirsFrom(os.Environ()).UserCacheDir.
func UserCacheDir() (string, error) { return DirsFrom(os.Environ()).UserCacheDir() }

// UserStateDir is DirsFrom(os.Environ()).UserStateDir.
func UserStateDir() (string, error) { return DirsFrom(os.Environ()).UserStateDir() }

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
	PR                    PRConfig             `json:"pr"`
	Agents                AgentsConfig         `json:"agents"`
	Subscription          SubscriptionConfig   `json:"subscription"`
	TicketStore           TicketStoreConfig    `json:"ticket-store"`
	Server                ServerConfig         `json:"server"`
	Recovery              RecoveryConfig       `json:"recovery"`
	CPUWatchdog           CPUWatchdogConfig    `json:"cpu-watchdog"`
	Watchdog              WatchdogConfig       `json:"watchdog"`
	DiskSpace             DiskSpaceConfig      `json:"disk-space"`
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
	// NotifyHold is how long a pause's chat message waits for recovery before
	// it is sent anyway.
	NotifyHold time.Duration `json:"-"`
}

// DefaultRecoveryNotifyHold is the NotifyHold when none is configured.
const DefaultRecoveryNotifyHold = 10 * time.Minute

// ServerConfig configures the orchestrator server.
type ServerConfig struct {
	// TabEnv is KEY=VALUE entries set on every iteration tab's shell, for
	// overriding what the user's shell rc would otherwise decide (e.g. PATH).
	TabEnv []string `json:"tab-env"`
	// LogRetention is how long a headless agent's logs outlive its last
	// activity. Zero means the runner's default.
	LogRetention time.Duration `json:"-"`
	// WatchConfig reloads the config file whenever it is saved.
	WatchConfig bool `json:"watch-config"`
	// RecentlyFinished is how many completed epics the snapshot's history lists.
	RecentlyFinished int `json:"recently-finished"`
	// Web configures the read-only web UI listener.
	Web WebConfig `json:"web"`
}

// WebConfig configures the web UI listener.
type WebConfig struct {
	// Listen is the host:port to serve the web UI on; empty means off.
	Listen string `json:"listen"`
	// DevFrontend turns on Dev mode: gx runs its own Vite from the Dev
	// checkout and serves the UI from it.
	DevFrontend bool `json:"dev-frontend"`
	// DevDir is the Dev checkout path overriding the one stamped into the
	// binary. Empty means the stamped one.
	DevDir string `json:"dev-dir"`
	// AllowedHosts are host names, besides IP literals and localhost, the
	// listener accepts in the Host header.
	AllowedHosts []string `json:"allowed-hosts"`
	// TOTP is whether web login asks for a TOTP code. True when absent.
	TOTP bool `json:"totp"`
	// ViEscape is a key sequence (like "jk") that leaves insert mode in the
	// web UI's vi editors. Empty means off.
	ViEscape string `json:"vi-escape"`
	// LeaderKeyMac and LeaderKey are the chord Leader keys, such as "cmd+e",
	// on Mac and on other platforms. Empty turns chords off.
	LeaderKeyMac string `json:"leader-key-mac"`
	LeaderKey    string `json:"leader-key"`
}

// DefaultRecentlyFinished is the RecentlyFinished when none is configured.
const DefaultRecentlyFinished = 5

// Default returns the default configuration.
func Default() Config { return defaultWith(DataDir) }

func defaultWith(dataDir func() (string, error)) Config {
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
		PR:                    DefaultPRConfig(),
		Agents:                DefaultAgentsConfig(),
		Subscription:          DefaultSubscriptionConfig(),
		TicketStore:           ticketStoreConfigWith(dataDir),
		Server:                ServerConfig{WatchConfig: true, RecentlyFinished: DefaultRecentlyFinished, Web: WebConfig{TOTP: true, LeaderKeyMac: "cmd+e", LeaderKey: "ctrl+e"}},
		Recovery:              RecoveryConfig{Enabled: true, NotifyHold: DefaultRecoveryNotifyHold},
		CPUWatchdog:           DefaultCPUWatchdogConfig(),
		Watchdog:              DefaultWatchdogConfig(),
		DiskSpace:             DefaultDiskSpaceConfig(),
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

func configFilePath(userConfigDir func() (string, error)) (string, error) {
	base, err := userConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve user config dir: %w", err)
	}
	return filepath.Join(base, "gx", "config.json"), nil
}

// FilePath returns the config file path, typically ~/.config/gx/config.json.
func FilePath() (string, error) { return configFilePath(userConfigDirFn) }

// Load reads user config from disk. Missing file returns defaults.
func Load() (Config, error) { return loadFrom(FilePath, DataDir) }

func loadFrom(filePath, dataDir func() (string, error)) (Config, error) {
	cfg := defaultWith(dataDir)
	path, err := filePath()
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
	cfg, err = parseWith(data, dataDir)
	if err != nil {
		return cfg, fmt.Errorf("config %s: %w", path, err)
	}
	return cfg, nil
}

// Parse decodes config file bytes over the defaults and validates them.
func Parse(data []byte) (Config, error) { return parseWith(data, DataDir) }

func parseWith(data []byte, dataDir func() (string, error)) (Config, error) {
	cfg := defaultWith(dataDir)
	var raw rawConfig
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
		if raw.ExecutionQueue.MaxOneOffAgents != nil {
			cfg.ExecutionQueue.MaxOneOffAgents = clampExecutionQueueLimit(*raw.ExecutionQueue.MaxOneOffAgents)
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
		if raw.ExecutionQueue.WaitingSlotGrace != nil {
			if d, err := time.ParseDuration(*raw.ExecutionQueue.WaitingSlotGrace); err == nil && d > 0 {
				cfg.ExecutionQueue.WaitingSlotGrace = d
			}
		}
		if raw.ExecutionQueue.MaxReleasedAgents != nil {
			cfg.ExecutionQueue.MaxReleasedAgents = max(*raw.ExecutionQueue.MaxReleasedAgents, 0)
		}
		if raw.ExecutionQueue.StalledNudgeDelay != nil {
			if d, err := time.ParseDuration(*raw.ExecutionQueue.StalledNudgeDelay); err == nil && d > 0 {
				cfg.ExecutionQueue.StalledNudgeDelay = d
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
		if raw.Budget.LongTicketTurns != nil {
			cfg.Budget.LongTicketTurns = *raw.Budget.LongTicketTurns
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
		if raw.Skills.CIFix != nil {
			cfg.Skills.CIFix = raw.Skills.CIFix
		}
		if raw.Skills.PRText != nil {
			cfg.Skills.PRText = *raw.Skills.PRText
		}
	}
	if p := raw.PR; p != nil {
		if p.CIFixAttemptsPerCheck != nil && *p.CIFixAttemptsPerCheck >= 1 {
			cfg.PR.CIFixAttemptsPerCheck = *p.CIFixAttemptsPerCheck
		}
		if p.CIFixAttemptsPerPR != nil && *p.CIFixAttemptsPerPR >= 1 {
			cfg.PR.CIFixAttemptsPerPR = *p.CIFixAttemptsPerPR
		}
		if p.NoChecksGrace != nil {
			if d, err := time.ParseDuration(*p.NoChecksGrace); err == nil && d >= 0 {
				cfg.PR.NoChecksGrace = d
			}
		}
	}
	if raw.Agents != nil {
		if raw.Agents.Claude != nil {
			applyAgentConfig(&cfg.Agents.Claude, raw.Agents.Claude)
		}
		if raw.Agents.Codex != nil {
			applyAgentConfig(&cfg.Agents.Codex, raw.Agents.Codex)
		}
		if h := raw.Agents.DefaultHarness; h != nil && *h != "" {
			if err := validateDefaultHarness(*h); err != nil {
				return cfg, err
			}
			cfg.Agents.DefaultHarness = *h
		}
		if keys, ok := parseKeys(raw.Agents.LeaderKey); ok {
			cfg.Agents.LeaderKey = keys
		}
		if keys, ok := parseKeys(raw.Agents.FocusSidebarKey); ok {
			cfg.Agents.FocusSidebarKey = keys
		}
		if raw.Agents.OldResumeOnStart != nil {
			cfg.Agents.ResumeOnStart = *raw.Agents.OldResumeOnStart
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
		if raw.Agents.WritableDirectories != nil {
			cfg.Agents.WritableDirectories = raw.Agents.WritableDirectories
		}
		if raw.Agents.FixAgentRemoteControl != nil {
			cfg.Agents.FixAgentRemoteControl = *raw.Agents.FixAgentRemoteControl
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
	if raw.Server != nil && raw.Server.WatchConfig != nil {
		cfg.Server.WatchConfig = *raw.Server.WatchConfig
	}
	if raw.Server != nil && raw.Server.LogRetention != nil {
		if d, err := time.ParseDuration(*raw.Server.LogRetention); err == nil && d > 0 {
			cfg.Server.LogRetention = d
		}
	}
	if raw.Server != nil && raw.Server.RecentlyFinished != nil && *raw.Server.RecentlyFinished >= 0 {
		cfg.Server.RecentlyFinished = *raw.Server.RecentlyFinished
	}
	if raw.Server != nil && raw.Server.Web != nil {
		w := raw.Server.Web
		if w.Listen != nil {
			cfg.Server.Web.Listen = *w.Listen
		}
		if w.DevFrontend != nil {
			cfg.Server.Web.DevFrontend = *w.DevFrontend
		}
		if w.DevDir != nil {
			cfg.Server.Web.DevDir = *w.DevDir
		}
		if w.AllowedHosts != nil {
			cfg.Server.Web.AllowedHosts = w.AllowedHosts
		}
		if w.TOTP != nil {
			cfg.Server.Web.TOTP = *w.TOTP
		}
		if w.ViEscape != nil {
			cfg.Server.Web.ViEscape = *w.ViEscape
		}
		if w.LeaderKeyMac != nil {
			cfg.Server.Web.LeaderKeyMac = *w.LeaderKeyMac
		}
		if w.LeaderKey != nil {
			cfg.Server.Web.LeaderKey = *w.LeaderKey
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
	if w := raw.Watchdog; w != nil {
		if w.MaxTreeProcesses != nil && *w.MaxTreeProcesses > 0 {
			cfg.Watchdog.MaxTreeProcesses = *w.MaxTreeProcesses
		}
		if w.Interval != nil {
			if d, err := time.ParseDuration(*w.Interval); err == nil && d > 0 {
				cfg.Watchdog.Interval = d
			}
		}
		if w.UlimitHeadroom != nil && *w.UlimitHeadroom > 0 {
			cfg.Watchdog.UlimitHeadroom = *w.UlimitHeadroom
		}
	}
	if d := raw.DiskSpace; d != nil {
		var err error
		if cfg.DiskSpace, err = applyDiskSpace(cfg.DiskSpace, d.WarnBelow, d.CleanBelow, d.CleanupCommand); err != nil {
			return cfg, err
		}
	}

	return cfg, nil
}

// applyAgentConfig overlays explicitly-set model/effort values onto an
// AgentConfig and its tiers, trimming whitespace and leaving unset keys at the
// built-in default. A trimmed-empty value survives as empty (meaning
// "inherit"), distinct from an absent key (meaning "use the default").
func applyAgentConfig(cfg *AgentConfig, raw *rawAgentConfig) {
	applyTierConfig(&cfg.Model, &cfg.Effort, raw.rawTierConfig)
	if raw.Light != nil {
		applyTierConfig(&cfg.Light.Model, &cfg.Light.Effort, *raw.Light)
	}
	if raw.Strongest != nil {
		applyTierConfig(&cfg.Strongest.Model, &cfg.Strongest.Effort, *raw.Strongest)
	}
}

func applyTierConfig(model, effort *string, raw rawTierConfig) {
	if raw.Model != nil {
		*model = strings.TrimSpace(*raw.Model)
	}
	if raw.Effort != nil {
		*effort = strings.TrimSpace(*raw.Effort)
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
func Init() (string, error) { return initAt(FilePath) }

func initAt(filePath func() (string, error)) (string, error) {
	path, err := filePath()
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
