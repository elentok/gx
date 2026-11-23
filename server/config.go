package server

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/agentrunner/deliver"
	"github.com/elentok/gx/commandlist"
	"github.com/elentok/gx/migrations"
	"github.com/elentok/gx/osnotify"
	"github.com/elentok/gx/ralphloop/deps"
	"github.com/elentok/gx/server/broker"
	"github.com/elentok/gx/server/devvite"
	"github.com/elentok/gx/server/sessions"
	"github.com/elentok/gx/server/wire"
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

	// RestartBinary returns the binary a restart would start; empty means none
	// could be found. Nil means this executable. Read per request: a reinstall
	// changes it while the server runs.
	RestartBinary func() string

	// SpawnRestart starts the restart from the given binary, detached. Nil means
	// `<binary> server restart --continue` in its own session; tests fake it.
	SpawnRestart func(bin string) error

	// ViteLauncher spawns the Dev mode Vite and probes it. Nil means pnpm;
	// tests fake it.
	ViteLauncher devvite.Launcher

	// Test seam (with files.Checker.statOverride): HostedFilesUID is the gx
	// user id that must own hosted temp files; nil means this process's user.
	// A test sets another id to see a foreign-owned temp file refused.
	HostedFilesUID *uint32

	StateDir    string
	Build       string
	TicketStore string // ticket-store root; kept fresh by watch + poll
	// Migrations is the registry the start-up migration runs; nil means
	// migrations.Default. Tests inject one; the release binary leaves it nil.
	Migrations *migrations.Registry
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

	// RunnerWait is the default runner's configured wait; zero means
	// config.LoadRunnerWait().
	RunnerWait time.Duration
	// RateLimitResetBuffer is how long past a rate limit's reset a hold waits;
	// zero means ralphloop's default.
	RateLimitResetBuffer time.Duration
	// GuardPoll is how often a running iteration ticks its guards; zero means
	// ralphloop's 30s default.
	GuardPoll time.Duration

	// RefreshSkills reinstalls the skills after an agent launch ended with an
	// unknown-skill error; nil means no refresh and no retry.
	RefreshSkills func() error

	// WrapLateFinishScan decorates the rescan's late-finish adoption step. It
	// is the one narrow seam for the gxfixture-only hold (cmd/serverctl), so a
	// flow can answer before the scan wins the race; nil in a release build.
	WrapLateFinishScan func(scan func()) func()

	// Sleep paces the ralphloop waits (finish debounce, poll backoffs); nil
	// means time.Sleep. Tests shrink it so each merge skips the real seconds.
	Sleep func(time.Duration)
	// InstallDeps replaces the dependency install a worktree gets before its
	// agent starts; nil means the real one. Tests fake it so no package manager runs.
	InstallDeps deps.InstallFunc

	// CleanupRunner runs one disk cleanup command in dir; nil runs the real command.
	CleanupRunner func(ctx context.Context, argv []string, dir string) ([]byte, error)
	// TipsGOOS is the OS the tips list is filtered for; empty means the real one.
	// TipsExtra is appended to the built-in tips list; tests only.
	TipsGOOS  string
	TipsExtra []wire.Tip
	// CleanupTimeout is the time limit per cleanup command; zero means ten minutes.
	CleanupTimeout time.Duration
	// TempSweepDir is the temp dir a clean sweeps for stale gx-epic-* and
	// go-build* dirs; empty means os.TempDir(). TempSweepMinAge is how long a
	// dir must sit untouched first; zero means an hour.
	TempSweepDir    string
	TempSweepMinAge time.Duration
	// CacheGateCap bounds the wait for running Go builds before a clean; zero
	// means ten minutes. CacheGateScan replaces the `go` process scan; nil
	// lists real processes. Both are test hooks.
	CacheGateCap  time.Duration
	CacheGateScan func() (bool, error)
	// ForkHookDir arms the draft test hooks: a save-fail or start-fail file in
	// it makes that step fail (see chatdrafts.HookArmed). Empty means off.
	ForkHookDir string

	// Runner is the configured agent runner; nil means the default runner.
	Runner agentrunner.Runner
	// RunnerFor, when set, returns the runner named runner for project, or nil
	// when it cannot build one. Runners that keep per-project state on disk
	// (headless agent dirs) need it; Runner still answers host health and any
	// pair RunnerFor returns nil for when runner is Runner's own name.
	RunnerFor func(runner, project string) agentrunner.Runner

	BudgetPollInterval time.Duration // zero means the default

	TokenPollInterval time.Duration // zero means the budget default

	// LimitsEventInterval is the coalescing window of limits-changed; zero
	// means DefaultLimitsEventInterval.
	LimitsEventInterval time.Duration

	// BudgetEventInterval is the coalescing window of budget-changed; zero
	// means DefaultBudgetEventInterval.
	BudgetEventInterval time.Duration

	// BudgetEventClock is the budget-changed throttle's time source; the zero
	// value is the wall clock. A test seam.
	BudgetEventClock broker.Clock

	// LookPath finds a harness's program; nil looks over Env's PATH.
	LookPath func(file string) (string, error)
	// FreeSpace reads the disk holding a path; nil reads the real one.
	FreeSpace func(path string) (wire.DiskReading, error)
	// GoCacheDir finds the Go build cache; nil runs `go env GOCACHE`.
	GoCacheDir func() (string, error)
	// DiskPollInterval is how often the disk watcher reads; zero means one minute.
	DiskPollInterval time.Duration
	// SchedulesClock is the Schedule scheduler's clock; nil means time.Now.
	SchedulesClock func() time.Time
	// RestClock is the Rest engine's clock; nil means time.Now. It is not Clock
	// (web auth) or SchedulesClock.
	RestClock func() time.Time
	// SchedulesInterval is how often the scheduler checks; zero means one minute.
	SchedulesInterval time.Duration
	// MigrationFailurePollInterval is how often the server checks the migration
	// failure record for a change; zero means 2 seconds.
	MigrationFailurePollInterval time.Duration

	// SystemHealth takes one CPU and memory reading; nil reads the real machine.
	SystemHealth syshealth.Reader
	// UsagePollInterval is how often the usage sample reader scans the agent
	// dirs; zero means one minute.
	UsagePollInterval time.Duration
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
	// prompt to go Working; zero means prompt.DefaultActionStepStartTimeout.
	ActionStepStartTimeout time.Duration
	// SessionExitCleanupDelay is how long the design cleanup of a session exit
	// waits before it checks, so a test can make one run late.
	SessionExitCleanupDelay time.Duration
	// ActionSlashSettle is how long a slash step must stay Idle after typing
	// to count as done; zero means prompt.DefaultActionSlashSettle.
	ActionSlashSettle time.Duration
	// KeysSettleStep and KeysSettleCap are the read interval and the limit of
	// a keys request's settle; zero means the prompt.DefaultKeysSettle ones.
	KeysSettleStep, KeysSettleCap time.Duration
	// CodexQuestionWait bounds the wait for each question's header on Codex's
	// overlay; zero means prompt.DefaultCodexQuestionWait.
	CodexQuestionWait time.Duration
	// FirstPromptStartTimeout bounds the Starting time, counted from launch,
	// before a pending first prompt is handed to a person; zero means
	// DefaultFirstPromptStartTimeout.
	FirstPromptStartTimeout time.Duration
	// PromptGrace bounds each wait for a typed prompt's turn to start
	// before it is nudged; zero means deliver.DefaultNudgeCaps.
	PromptGrace time.Duration
	// WebAttachIdle closes a web client's holder attach left unused this long;
	// zero means webattach.DefaultIdleTimeout.
	WebAttachIdle time.Duration
	// LaunchProofCaps bound the check that a claude session's first slash
	// prompt reached the transcript; a zero field means the matching
	// claudeui.DefaultLaunchProofCaps one.
	LaunchProofCaps deliver.LaunchTimings

	// CompactRewaitTimeout bounds recovery's R7 wait; zero means
	// DefaultCompactRewaitTimeout.
	CompactRewaitTimeout time.Duration

	// WebAddr, when set, adds a separate read-only HTTP listener for the web UI.
	// A bind failure is reported in the handshake and never stops the server.
	WebAddr string

	// Clock is the web auth clock: the web TLS leaf's expiry checks, the login
	// store and the cookie renewal read it. Nil means time.Now.
	Clock func() time.Time

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
	// PushHTTPClient sends Web Push requests; nil means a client with a
	// timeout of about 15 seconds. Tests point it at an httptest service.
	PushHTTPClient *http.Client
	// SessionPush replaces the dispatcher as the sink of session pushes; tests
	// use it to record them.
	SessionPush sessions.PushSender
	// PushMergeDelay is how long a push waits for events of its tag to
	// merge; zero means one second.
	PushMergeDelay time.Duration
	// PushAllowHTTPEndpoints is a test seam: the subscription check accepts an
	// http: endpoint, for the fixture's loopback push service. gxfixture sets it
	// through GX_PUSH_ALLOW_HTTP_ENDPOINTS; false for every user.
	PushAllowHTTPEndpoints bool
	// ChatPollInterval is how often a working session's transcript size is
	// checked; zero means defaultChatPollInterval.
	ChatPollInterval time.Duration
	// StallThreshold is how long a Working session's transcript may stay
	// untouched before it counts as stalled; zero means defaultStallThreshold.
	StallThreshold time.Duration
	// DialogFallbackDelay is how long a Claude session waits on "input needed"
	// before its screen dialog lists as a card; zero means
	// chat.DefaultDialogFallbackDelay.
	DialogFallbackDelay time.Duration
	// TranscriptMissTTL is how long a failed transcript lookup is remembered;
	// zero means defaultTranscriptMissTTL and a negative value remembers none.
	TranscriptMissTTL time.Duration
	// TurnSettleTimeout caps how long an idle edge waits for claude to write
	// the end of its turn; zero means sessprompt.DefaultTurnSettleTimeout.
	TurnSettleTimeout time.Duration
	// AutoAdvanceDelay is the countdown before the server sends a run-end
	// Continue; zero means sessprompt.DefaultAutoAdvanceDelay and a negative value turns
	// auto-advance off.
	AutoAdvanceDelay time.Duration
	// CommandListAge is how long a command list is served from the cache;
	// zero means commandlist.DefaultMaxAge.
	CommandListAge time.Duration
	// Models is the model-list cache; nil builds the real one, which runs the
	// real harness binaries. Tests set one with fake probes.
	Models *commandlist.ModelCache
}
