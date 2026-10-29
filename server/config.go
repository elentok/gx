package server

import (
	"context"
	"log/slog"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/osnotify"
	"github.com/elentok/gx/sound"
	"github.com/elentok/gx/subscription"
	"github.com/elentok/gx/syshealth"
)

type Config struct {
	Settings

	// Env is the environment the server runs under, as KEY=VALUE entries like
	// exec.Cmd.Env. The server reads HOME, CLAUDE_CONFIG_DIR and CODEX_HOME from
	// it and starts every session holder with it. Nil means the process env.
	Env []string

	StateDir    string
	Build       string
	TicketStore string // ticket-store root; kept fresh by watch + poll
	// ConfigPath is the config file a reload reads; empty means reload is
	// unavailable.
	ConfigPath string
	// SessionsDir holds one state dir per interactive session; empty means
	// <gx data dir>/sessions.
	SessionsDir string
	// ConversationsDir holds the conversation records of linked sessions;
	// empty means <gx data dir>/conversations.
	ConversationsDir string
	// Holder is the command that hosts an interactive session; nil means
	// this executable's agent-holder.
	Holder []string
	// AgentRunner is the config agent-runner value the server started with;
	// a reload only compares against it.
	AgentRunner string

	PollInterval time.Duration // zero means the default
	DisableWatch bool          // poll only

	ConfigWatchDebounce time.Duration // zero means the default

	HerdrRetryInterval time.Duration // zero means the default

	// RunnerWait is the default runner's configured wait; zero means
	// config.LoadRunnerWait().
	RunnerWait time.Duration

	// Sleep paces the ralphloop waits (finish debounce, poll backoffs); nil
	// means time.Sleep. Tests shrink it so each merge skips the real seconds.
	Sleep func(time.Duration)
	// InstallDeps replaces the dependency install a worktree gets before its
	// agent starts; nil means the real one. Tests fake it so no package manager runs.
	InstallDeps func(path, logDir string) ([]string, error)

	// CleanupRunner runs one disk cleanup command in dir; nil runs the real command.
	CleanupRunner func(ctx context.Context, argv []string, dir string) ([]byte, error)
	// CleanupTimeout is the time limit per cleanup command; zero means ten minutes.
	CleanupTimeout time.Duration

	// Runner is the configured agent runner; nil means herdr. Herdr health
	// is only probed when it reports health (agentrunner.HealthChecker).
	Runner agentrunner.Runner
	// RunnerFor, when set, returns the runner named runner for project, or nil
	// when it cannot build one. Runners that keep per-project state on disk
	// (headless agent dirs) need it; Runner still answers host health and any
	// pair RunnerFor returns nil for when runner is Runner's own name.
	RunnerFor func(runner, project string) agentrunner.Runner

	BudgetPollInterval time.Duration // zero means the default

	// FreeSpace reads the disk holding a path; nil reads the real one.
	FreeSpace func(path string) (DiskReading, error)
	// GoCacheDir finds the Go build cache; nil runs `go env GOCACHE`.
	GoCacheDir func() (string, error)
	// DiskPollInterval is how often the disk watcher reads; zero means one minute.
	DiskPollInterval time.Duration

	// SystemHealth takes one CPU and memory reading; nil reads the real machine.
	SystemHealth syshealth.Reader
	// HealthPollInterval is how often the health watcher reads; zero means 5 seconds.
	HealthPollInterval time.Duration

	// ProcessSource takes one process table read for the health route's cause
	// lists and looks up a row's argv; nil reads the real table.
	ProcessSource syshealth.ProcessReader
	// ProcessSecondReadDelay is the wait between the first and the second
	// process read after a quiet spell; zero means 1 second.
	ProcessSecondReadDelay time.Duration
	// ProcessShareWindow is how long a process read answers later calls, from
	// the read's own time; zero means 1 second.
	ProcessShareWindow time.Duration
	// ProcessCutoff is the age after which a process read is too old to be the
	// first of a pair; zero means 15 seconds. It must exceed the popover's poll.
	ProcessCutoff time.Duration
	// ProcessWaitCap is the longest a `wait` call blocks; zero means 5 seconds.
	ProcessWaitCap time.Duration

	BudgetKillGrace time.Duration // wait between ctrl+c and closing a pane; zero means the default

	// HardLimitGrace is how long a reload-lowered hard limit waits before it
	// stops agents; zero means the default.
	HardLimitGrace time.Duration

	SubscriberBuffer int // events a stream may lag behind before it is dropped; zero means the default

	SSEHeartbeat time.Duration // idle gap before /v1/events writes a comment line; zero means DefaultSSEHeartbeat

	// MergeStopTimeout bounds how long a stop waits for a merge in flight; zero
	// means DefaultMergeStopTimeout.
	MergeStopTimeout time.Duration

	// LogLevel is the minimum level written to the server log; the zero value
	// is slog.LevelInfo.
	LogLevel slog.Level

	// ActionStepStartTimeout bounds how long an Action step waits for its
	// prompt to go Working; zero means DefaultActionStepStartTimeout.
	ActionStepStartTimeout time.Duration
	// SessionExitCleanupDelay is how long the design cleanup of a session exit
	// waits before it checks, so a test can make one run late.
	SessionExitCleanupDelay time.Duration
	// ActionSlashSettle is how long a slash step must stay Idle after typing
	// to count as done; zero means DefaultActionSlashSettle.
	ActionSlashSettle time.Duration
	// FirstPromptStartTimeout bounds the Starting time, counted from launch,
	// before a pending first prompt is handed to a person; zero means
	// DefaultFirstPromptStartTimeout.
	FirstPromptStartTimeout time.Duration
	// PromptGrace bounds each wait for a typed prompt's turn to start
	// before it is nudged; zero means claudeui.DefaultNudgeCaps.
	PromptGrace time.Duration

	// CompactRewaitTimeout bounds recovery's R7 wait; zero means
	// DefaultCompactRewaitTimeout.
	CompactRewaitTimeout time.Duration

	// TCPAddr, when set, adds a loopback-only TCP listener serving the same
	// handler as the socket. No auth: loopback is the only protection.
	TCPAddr string

	// WebAddr, when set, adds a separate read-only HTTP listener for the web UI.
	// A bind failure is reported in the handshake and never stops the server.
	WebAddr string
	// WebDevURL, when set with WebAddr, proxies the web UI's non-API requests to a Vite dev server.
	WebDevURL string

	// ExtraUsageCheck reads the subscription state; nil means subscription.CheckFresh.
	ExtraUsageCheck func() subscription.State

	// Notifier shows OS banners for config reloads; nil means
	// osnotify.NewSystemNotifier.
	Notifier osnotify.Notifier

	// Player plays session sounds; nil means sound.NewSystemPlayer.
	Player sound.Player
	// SessionBannerDelay is how long a session banner waits for triggers to
	// merge; zero means defaultSessionBannerDelay.
	SessionBannerDelay time.Duration
	// ChatPollInterval is how often a working session's transcript size is
	// checked; zero means defaultChatPollInterval.
	ChatPollInterval time.Duration
	// StallThreshold is how long a Working session's transcript may stay
	// untouched before it counts as stalled; zero means defaultStallThreshold.
	StallThreshold time.Duration
	// TranscriptMissTTL is how long a failed transcript lookup is remembered;
	// zero means defaultTranscriptMissTTL and a negative value remembers none.
	TranscriptMissTTL time.Duration
	// TurnSettleTimeout caps how long an idle edge waits for claude to write
	// the end of its turn; zero means defaultTurnSettleTimeout.
	TurnSettleTimeout time.Duration
	// AutoAdvanceDelay is the countdown before the server sends a run-end
	// Continue; zero means DefaultAutoAdvanceDelay and a negative value turns
	// auto-advance off.
	AutoAdvanceDelay time.Duration
}
