package config

import "encoding/json"

// rawConfig is config.json as written in the file: a nil field is an absent
// key. It holds every key the file accepts, including those that are
// `json:"-"` on Config.
type rawConfig struct {
	UseNerdFontIcons      *bool             `json:"use-nerdfont-icons"`
	ImageDiffs            *bool             `json:"image-diffs"`
	StageDiffContextLines *int              `json:"stage-diff-context-lines"`
	InputModalBottom      *InputModalBottom `json:"input-modal-bottom"`
	NameAliases           map[string]string `json:"name-aliases"`
	Log                   *LogConfig        `json:"log"`
	ExecutionQueue        *struct {
		MaxAgentsPerEpic   *int    `json:"max-agents-per-epic"`
		MaxAgents          *int    `json:"max-agents"`
		MaxOneOffAgents    *int    `json:"max-one-off-agents"`
		RetryStormLaunches *int    `json:"retry-storm-launches"`
		SpinCycles         *int    `json:"spin-cycles"`
		SpinWindow         *string `json:"spin-window"`
		WaitingSlotGrace   *string `json:"waiting-slot-grace"`
		MaxReleasedAgents  *int    `json:"max-released-agents"`
		StalledNudgeDelay  *string `json:"stalled-nudge-delay"`
	} `json:"execution-queue"`
	Budget *struct {
		SoftLimit              *float64  `json:"soft-limit"`
		HardLimit              *float64  `json:"hard-limit"`
		NotificationThresholds []float64 `json:"notification-thresholds"`
		LongTicketTurns        *int      `json:"long-ticket-turns"`
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
		CIFix      []string `json:"ci-fix"`
		PRText     *string  `json:"pr-text"`
	} `json:"skills"`
	PR *struct {
		CIFixAttemptsPerCheck *int    `json:"ci-fix-attempts-per-check"`
		CIFixAttemptsPerPR    *int    `json:"ci-fix-attempts-per-pr"`
		NoChecksGrace         *string `json:"no-checks-grace"`
	} `json:"pr"`
	Agents *struct {
		Claude          *rawAgentConfig `json:"claude"`
		Codex           *rawAgentConfig `json:"codex"`
		DefaultHarness  *string         `json:"default-harness"`
		LeaderKey       json.RawMessage `json:"leader-key"`
		FocusSidebarKey json.RawMessage `json:"focus-sidebar-key"`
		ResumeOnStart   *bool           `json:"restore-sessions-on-start"`
		// OldResumeOnStart is the hidden alias for one release.
		OldResumeOnStart *bool `json:"resume-on-start"`
		Banners          *struct {
			Enabled *bool `json:"enabled"`
		} `json:"banners"`
		Sounds *struct {
			Enabled   *bool   `json:"enabled"`
			Done      *string `json:"done"`
			Attention *string `json:"attention"`
		} `json:"sounds"`
		ClaudeArgs            []string `json:"claude-args"`
		FixAgentRemoteControl *bool    `json:"fix-agent-remote-control"`
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

		RecentlyFinished *int `json:"recently-finished"`
		Web              *struct {
			Listen      *string `json:"listen"`
			DevFrontend *bool   `json:"dev-frontend"`
			DevDir      *string `json:"dev-dir"`

			AllowedHosts []string `json:"allowed-hosts"`
			TOTP         *bool    `json:"totp"`
			ViEscape     *string  `json:"vi-escape"`

			LeaderKeyMac *string `json:"leader-key-mac"`
			LeaderKey    *string `json:"leader-key"`
		} `json:"web"`
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
	Watchdog *struct {
		MaxTreeProcesses *int    `json:"max-tree-processes"`
		Interval         *string `json:"interval"`
		UlimitHeadroom   *int    `json:"ulimit-headroom"`
	} `json:"watchdog"`
	DiskSpace *struct {
		WarnBelow      *string  `json:"warn-below"`
		CleanBelow     *string  `json:"clean-below"`
		CleanupCommand []string `json:"cleanup-command"`
	} `json:"disk-space"`
}

// rawTierConfig is a tier as written in the file: a nil field is an absent key.
type rawTierConfig struct {
	Model  *string `json:"model"`
	Effort *string `json:"effort"`
}

type rawAgentConfig struct {
	rawTierConfig
	Light     *rawTierConfig `json:"light"`
	Strongest *rawTierConfig `json:"strongest"`
}
