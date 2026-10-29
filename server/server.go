// Package server is the orchestrator daemon: HTTP/JSON over a unix socket in
// the state dir, routes under /v1/. Its writes are the server-wide queue in the
// state dir and the claim of a queued root's frontier ticket.
package server

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/elentok/gx/agentrunner/herdrrunner"
	"github.com/elentok/gx/brief"
	"github.com/elentok/gx/config"
	"github.com/elentok/gx/conversations"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/harness"
	"github.com/elentok/gx/herdr"
	"github.com/elentok/gx/osnotify"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/recovery"
	"github.com/elentok/gx/sessionlookup"
	"github.com/elentok/gx/skills"
	"github.com/elentok/gx/sound"
	"github.com/elentok/gx/storecommit"
	"github.com/elentok/gx/syshealth"
	"github.com/elentok/gx/tickets/schema"
)

// APIVersion is bumped on any incompatible change to the /v1 wire contract.
const APIVersion = 3

const (
	lockFileName   = "server.lock"
	socketFileName = "server.sock"
)

// SocketPath is where the server listens inside stateDir.
func SocketPath(stateDir string) string { return filepath.Join(stateDir, socketFileName) }

// Handshake is the /v1/handshake payload. Shared with the API client.
type Handshake struct {
	APIVersion int    `json:"api_version"`
	Build      string `json:"build"`
	Pid        int    `json:"pid"`
	// TCPAddr is the loopback address of the opt-in TCP listener; empty when off.
	TCPAddr string `json:"tcp_addr,omitempty"`
	// WebURL is the base URL of the web listener; empty when it is off or failed to bind.
	WebURL string `json:"web_url,omitempty"`
	// WebError is why the web listener failed to bind; empty otherwise.
	WebError string `json:"web_error,omitempty"`
	// HerdrUnavailable is set while herdr isn't answering, so a client can say
	// why nothing starts.
	HerdrUnavailable bool `json:"herdr_unavailable,omitempty"`
}

// Settings are the Config fields a config reload may change. The server reads
// them through an atomic snapshot (Server.settings), never from Config.
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
	Chat ralphloop.ServerChatConfig

	// SuppressExtraUsageWarning is config subscription.suppress-extra-usage-warning.
	SuppressExtraUsageWarning bool

	// WatchConfig reloads Config.ConfigPath whenever it is saved.
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

	// WebAllowedHosts is config server.web.allowed-hosts: names, besides IP
	// literals and localhost, that the web listener accepts in Host.
	WebAllowedHosts []string

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

// SettingsFromConfig maps the user config to the server's reloadable
// settings. Startup and reload both use it, so the two never disagree.
func SettingsFromConfig(cfg config.Config) Settings {
	return Settings{
		TabEnv:              cfg.Server.TabEnv,
		Evolve:              cfg.Evolve,
		LogRetention:        cfg.Server.LogRetention,
		BudgetSoftLimit:     cfg.Budget.SoftLimit,
		BudgetHardLimit:     cfg.Budget.HardLimit,
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
		Chat: ralphloop.ServerChatConfig{
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
		WebAllowedHosts:             cfg.Server.Web.AllowedHosts,
	}
}

// Defaults for the session-prompting timings in Config. A zero Config field
// means the default; PromptGrace defaults to claudeui.DefaultNudgeCaps.
const (
	// DefaultActionStepStartTimeout is how long an Action step waits for its
	// prompt to go Working before the rest of the Action is dropped.
	DefaultActionStepStartTimeout = 10 * time.Second

	// DefaultActionSlashSettle is how long a slash step stays Idle after
	// typing before it counts as done: claude runs some commands locally and
	// never goes Working.
	DefaultActionSlashSettle = time.Second

	// DefaultFirstPromptStartTimeout is how long a session may stay Starting,
	// counted from launch, before its first prompt is handed to a person.
	DefaultFirstPromptStartTimeout = 30 * time.Second
)

// DefaultTCPAddr is where the opt-in TCP listener binds.
const DefaultTCPAddr = "127.0.0.1:7421"

// Server owns the state-dir lock and the listening socket.
type Server struct {
	cfg Config
	// settings is the live copy of cfg.Settings. A reader loads it once per
	// decision; a reload swaps in a new value and never mutates the old one.
	settings atomic.Pointer[Settings]
	reloadMu sync.Mutex // serializes config reloads
	reloads  reloadHistory
	// cpuSpikes holds the CPU ticket cooldown.
	cpuSpikes  cpuSpikes
	disks      diskWatch
	healthW    healthWatch
	procs      procWatch
	ownerCache ownerCache
	// chats caches parsed session chats by transcript file.
	chats chatCache
	// transcripts caches where each conversation's transcript is.
	transcripts transcriptPaths
	// autoAdvances holds the running auto-advance countdowns.
	autoAdvances autoAdvances
	// authFailures caches whether each transcript ends in an auth error.
	authFailures authFailures
	// reportedModels caches the model ID on each transcript's newest reply.
	reportedModels fileCache[string]
	// filedRounds holds the round markers already in the follow-ups epic, so an
	// Idle detection never rescans its tickets.
	filedRounds sync.Map
	// storeCommits is nil until the store commit loop starts; guarded by
	// reloadMu so a reload racing the start is never lost.
	storeCommits *storecommit.Loop
	// configWatch runs the config watcher; guarded by reloadMu, so a reload
	// toggling server.watch-config only ever cancels a context.
	configWatch configWatch
	lock        *serverLock
	ln          net.Listener
	tcp         net.Listener // nil unless Config.TCPAddr is set
	http        *http.Server
	tcpHTTP     *http.Server // the socket's routes behind refuseBrowsers
	web         net.Listener // nil unless Config.WebAddr is set and bound
	webDev      http.Handler // nil unless Config.WebDevURL is set and valid
	webHTTP     *http.Server
	webErr      string // why the web bind failed
	webRoot     *x509.Certificate
	webListenIP net.IP // nil when the listener binds every interface
	clientErrs  ipLimiter
	logf        *rotatingFile
	log         *slog.Logger
	idx         *index

	queued       *queueStore
	pause        *pauseState
	afk          *afkState
	ledger       *budgetLedger
	budgetNotes  budgetNotes
	hardGrace    hardGrace
	extraUsage   extraUsageNotes
	costOf       func(IterationInfo) (float64, bool) // swapped in tests
	events       *broker
	herdr        herdrWatch
	pauseFold    pauseFold
	pauseHold    recoveryHold
	defectScan   sync.Mutex
	lateFinishes inFlight
	designSteps  sync.Map // map address -> design step at the last rescan
	deadlocks    deadlocks
	gatesRaised  map[gateHold]bool      // touched only by the gate watchdog
	rewatch      atomic.Pointer[func()] // set by keepFresh when the watch is active; read by every rescan caller
	sweepMu      sync.Mutex             // one archive sweep at a time; the move's own writes wake the watcher

	chat           *ralphloop.ServerChat // never nil; sends nothing when no destination is configured
	registry       *runRegistry
	savedRuns      []trackedRun // handles the previous server left; consumed by reclaimRuns
	refused        refusals
	heldMerges     refusals // roots whose merge waits for a trunk-conflict ticket, keyed by root
	merges         mergeGuard
	adoptMu        sync.Mutex // a late-finish adoption holds it from its finish to the cancel of unstarted investigations; the runner claims under it
	trunkLocks     trunkLocks
	mergeTargets   mergeTargetCache
	projectRepos   projectRepoCache
	mergeInto      func(git.Repo, []git.Worktree, string, string) (git.MergeOutcome, error) // swapped in tests
	mergedProbe    func(repo, ancestor, descendant string) (bool, error)                    // the poll-time ancestry check; swapped in tests
	verdicts       verdictLog
	nudges         nudgeLimiter
	unavailable    unavailableNotes
	trunkMissing   missingTrunks
	kick           chan struct{} // wakes keepClaiming
	sessionWatches *sessionWatches
	designOpens    sync.Map // epic address -> *sync.Mutex, serializing designs open
	focus          *focusReports
	sessionBanners sessionBanners
	settling       sync.WaitGroup // goSettle goroutines; Serve's stop leaves them running
	unblockMu      sync.Mutex     // makes leaving a headless block happen once, whoever sees it first
}

// New prepares the state dir, takes the server lock and binds the socket.
// It does not serve until Serve is called.
func New(cfg Config) (*Server, error) {
	if cfg.Env == nil {
		cfg.Env = os.Environ()
	}
	if cfg.Runner == nil {
		h := herdrrunner.NewIn(herdr.CLI{Env: cfg.Env}, harness.LookupEnv(cfg.Env, "HOME"), sessionlookup.RootsFrom(cfg.Env).CodexHome)
		h.N = cfg.RunnerWait
		if h.N == 0 {
			h.N = config.LoadRunnerWait()
		}
		cfg.Runner = h
	}
	if cfg.Notifier == nil {
		cfg.Notifier = osnotify.NewSystemNotifier()
	}
	if cfg.ProcessSource == nil {
		cfg.ProcessSource = syshealth.DefaultProcess()
	}
	if cfg.SessionBannerDelay == 0 {
		cfg.SessionBannerDelay = defaultSessionBannerDelay
	}
	if cfg.ChatPollInterval == 0 {
		cfg.ChatPollInterval = defaultChatPollInterval
	}
	if cfg.Chat.GateStatePath == "" {
		cfg.Chat.GateStatePath = ralphloop.NotificationStatePath(cfg.StateDir)
	}
	if cfg.StallThreshold == 0 {
		cfg.StallThreshold = defaultStallThreshold
	}
	if cfg.TurnSettleTimeout == 0 {
		cfg.TurnSettleTimeout = defaultTurnSettleTimeout
	}
	if cfg.TranscriptMissTTL == 0 {
		cfg.TranscriptMissTTL = defaultTranscriptMissTTL
	}
	if cfg.SessionsDir == "" {
		data, err := config.DataDir()
		if err != nil {
			return nil, err
		}
		cfg.SessionsDir = filepath.Join(data, "sessions")
	}
	if cfg.ConversationsDir == "" {
		dir, err := conversations.DirIn(config.DirsFrom(cfg.Env))
		if err != nil {
			return nil, err
		}
		cfg.ConversationsDir = dir
	}
	if err := conversations.MarkStarted(cfg.ConversationsDir, time.Now()); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(cfg.StateDir, 0o700); err != nil {
		return nil, err
	}
	// MkdirAll leaves a pre-existing dir's mode alone; the socket's only
	// protection beyond its own mode is the dir, so enforce it.
	if err := os.Chmod(cfg.StateDir, 0o700); err != nil {
		return nil, err
	}
	lock, err := acquireLock(filepath.Join(cfg.StateDir, lockFileName))
	if err != nil {
		return nil, err
	}
	if cfg.TicketStore != "" {
		if err := ensureScratchProject(cfg.TicketStore); err != nil {
			lock.release()
			return nil, fmt.Errorf("create scratch project: %w", err)
		}
		if err := pruneScratchWorkspace(cfg.TicketStore); err != nil {
			lock.release()
			return nil, fmt.Errorf("prune scratch workspace: %w", err)
		}
	}
	events := newBroker(cfg.SubscriberBuffer)
	idx, err := buildIndex(cfg.TicketStore, events)
	if err != nil {
		lock.release()
		return nil, fmt.Errorf("index ticket store: %w", err)
	}
	queued, err := openQueue(cfg.StateDir)
	if err != nil {
		lock.release()
		return nil, fmt.Errorf("load queue: %w", err)
	}
	pause, err := openPause(cfg.StateDir)
	if err != nil {
		lock.release()
		return nil, fmt.Errorf("load queue pause: %w", err)
	}
	afk, err := openAFK(cfg.StateDir)
	if err != nil {
		lock.release()
		return nil, fmt.Errorf("load afk mode: %w", err)
	}
	ledger, err := openLedger(cfg.StateDir, cfg.BudgetSoftLimit, cfg.BudgetHardLimit)
	if err != nil {
		lock.release()
		return nil, fmt.Errorf("load budget ledger: %w", err)
	}
	sock := SocketPath(cfg.StateDir)
	// We hold the lock, so any socket file is stale from a crashed server.
	_ = os.Remove(sock)
	ln, err := net.Listen("unix", sock)
	if err != nil {
		lock.release()
		return nil, err
	}
	if err := os.Chmod(sock, 0o600); err != nil {
		ln.Close()
		lock.release()
		return nil, err
	}
	var tcp net.Listener
	if cfg.TCPAddr != "" {
		if tcp, err = listenLoopback(cfg.TCPAddr); err != nil {
			ln.Close()
			lock.release()
			return nil, err
		}
	}
	logf, err := openRotatingFile(LogPath(cfg.StateDir), logMaxBytes, logMaxFiles)
	if err != nil {
		ln.Close()
		if tcp != nil {
			tcp.Close()
		}
		lock.release()
		return nil, err
	}
	log := slog.New(slog.NewJSONHandler(logf, &slog.HandlerOptions{Level: cfg.LogLevel}))
	registry, savedRuns, err := openRuns(cfg.StateDir, log)
	if err != nil {
		ln.Close()
		if tcp != nil {
			tcp.Close()
		}
		_ = logf.Close()
		lock.release()
		return nil, fmt.Errorf("load runs: %w", err)
	}
	s := &Server{
		cfg:  cfg,
		lock: lock,
		ln:   ln,
		tcp:  tcp,
		logf: logf,
		log:  log,
		idx:  idx,

		chat:      ralphloop.NewServerChat(cfg.Chat),
		registry:  registry,
		savedRuns: savedRuns,

		queued:      queued,
		pause:       pause,
		afk:         afk,
		ledger:      ledger,
		costOf:      iterationCostIn(cfg.Env),
		mergeInto:   git.MergeBranchInto,
		mergedProbe: git.IsAncestor,
		events:      events,
		kick:        make(chan struct{}, 1),

		sessionWatches: newSessionWatches(),
		focus:          newFocusReports(),
	}
	if h, ok := cfg.Runner.(*herdrrunner.Runner); ok {
		h.Finished = s.sessionFinished
	}
	if s.cfg.Player == nil {
		s.cfg.Player = sound.NewSystemPlayer(log)
	}
	s.pauseFold.onCap = s.flushPauseFold
	s.settings.Store(&cfg.Settings)
	mux := http.NewServeMux()
	for _, r := range routeTable {
		mux.HandleFunc(r.pattern, func(w http.ResponseWriter, req *http.Request) { r.handler(s, w, req) })
	}
	s.http = &http.Server{Handler: mux}
	s.tcpHTTP = &http.Server{Handler: refuseBrowsers(mux, log)}
	s.bindWeb()
	// A down herdr never stops the server; it is reported and retried.
	s.checkHerdr()
	s.initHealth()
	return s, nil
}

// routeTable is the single list of endpoints. cmd's parity test diffs it
// against the CLI command tree, so every route needs a `--json` verb.
var routeTable = []struct {
	pattern string
	handler func(*Server, http.ResponseWriter, *http.Request)
}{
	{"GET /v1/handshake", (*Server).handshake},
	{"GET /v1/snapshot", (*Server).snapshot},
	{"GET /v1/events", (*Server).streamEvents},
	{"GET /v1/projects", (*Server).projects},
	{"GET /v1/projects/worktrees", (*Server).projectWorktrees},
	{"POST /v1/projects/add", (*Server).projectAdd},
	{"POST /v1/projects/remove", func(s *Server, w http.ResponseWriter, r *http.Request) { s.projectWrite(s.removeProject)(w, r) }},
	{"POST /v1/projects/set-path", func(s *Server, w http.ResponseWriter, r *http.Request) { s.projectWrite(s.setProjectPath)(w, r) }},
	{"GET /v1/locks", (*Server).locks},
	{"GET /v1/budget", (*Server).budget},
	{"POST /v1/budget/override", (*Server).budgetOverrideWrite},
	{"POST /v1/config/reload", (*Server).configReloadWrite},
	{"GET /v1/config/status", (*Server).configStatusRead},
	{"GET /v1/disk/status", (*Server).diskStatusRead},
	{"GET /v1/health", (*Server).healthRead},
	{"POST /v1/disk/clean", (*Server).diskCleanWrite},
	{"GET /v1/checks/status", (*Server).checksStatusRead},
	{"POST /v1/budget/increase", (*Server).budgetIncreaseWrite},
	{"GET /v1/tickets/history", (*Server).history},
	{"GET /v1/tickets/explain", (*Server).explain},
	{"GET /v1/tickets/detail", (*Server).detail},
	{"GET /v1/epics", (*Server).epicList},
	{"GET /v1/epics/detail", (*Server).epicDetail},
	{"GET /v1/stats", (*Server).stats},
	{"GET /v1/iterations", (*Server).iterations},
	{"POST /v1/tickets/changed", (*Server).ticketChanged},
	{"GET /v1/queue", (*Server).queue},
	{"GET /v1/queue/items", (*Server).queueItems},
	{"POST /v1/queue/add", func(s *Server, w http.ResponseWriter, r *http.Request) { s.queueWrite(s.queueAdd)(w, r) }},
	{"POST /v1/oneoff", (*Server).oneOffHandler},
	{"POST /v1/cpu-spike", (*Server).cpuSpikeWrite},
	{"POST /v1/queue/remove", func(s *Server, w http.ResponseWriter, r *http.Request) { s.queueWrite(s.queueRemove)(w, r) }},
	{"POST /v1/queue/move", func(s *Server, w http.ResponseWriter, r *http.Request) { s.queueWrite(s.queueMove)(w, r) }},
	{"POST /v1/queue/replace", func(s *Server, w http.ResponseWriter, r *http.Request) { s.queueWrite(s.queueReplace)(w, r) }},
	{"POST /v1/queue/pause", func(s *Server, w http.ResponseWriter, r *http.Request) { s.modeWrite(s.queuePause)(w, r) }},
	{"POST /v1/queue/resume", func(s *Server, w http.ResponseWriter, r *http.Request) { s.modeWrite(s.queueResume)(w, r) }},
	{"POST /v1/queue/drain", func(s *Server, w http.ResponseWriter, r *http.Request) { s.modeWrite(s.queueFlush)(w, r) }},
	{"POST /v1/tickets/merge", func(s *Server, w http.ResponseWriter, r *http.Request) { repairWrite(s.mergeInQueue)(w, r) }},
	{"POST /v1/tickets/start-over", func(s *Server, w http.ResponseWriter, r *http.Request) { repairWrite(s.startOverInQueue)(w, r) }},
	{"POST /v1/tickets/resume", func(s *Server, w http.ResponseWriter, r *http.Request) { repairWrite(s.repairResume)(w, r) }},
	{"POST /v1/tickets/unpark", func(s *Server, w http.ResponseWriter, r *http.Request) { repairWrite(s.repairResume)(w, r) }},
	{"POST /v1/tickets/verify", func(s *Server, w http.ResponseWriter, r *http.Request) { repairWrite(s.repairVerify)(w, r) }},
	{"POST /v1/tickets/pause", func(s *Server, w http.ResponseWriter, r *http.Request) { s.queueWrite(s.ticketPause)(w, r) }},
	{"POST /v1/tickets/park", func(s *Server, w http.ResponseWriter, r *http.Request) { s.queueWrite(s.ticketPause)(w, r) }},
	{"POST /v1/tickets/unmute", func(s *Server, w http.ResponseWriter, r *http.Request) { s.queueWrite(s.ticketUnmute)(w, r) }},
	{"POST /v1/tickets/cancel", func(s *Server, w http.ResponseWriter, r *http.Request) { s.queueWrite(s.ticketCancel)(w, r) }},
	{"POST /v1/tickets/recommend", func(s *Server, w http.ResponseWriter, r *http.Request) { s.queueWrite(s.ticketRecommend)(w, r) }},
	{"POST /v1/tickets/nudge", func(s *Server, w http.ResponseWriter, r *http.Request) { s.queueWrite(s.ticketNudge)(w, r) }},
	{"POST /v1/tickets/answer", func(s *Server, w http.ResponseWriter, r *http.Request) { s.queueWrite(s.ticketAnswer)(w, r) }},
	{"POST /v1/tickets/ask", func(s *Server, w http.ResponseWriter, r *http.Request) { s.queueWrite(s.ticketAsk)(w, r) }},
	{"POST /v1/tickets/approve", func(s *Server, w http.ResponseWriter, r *http.Request) { s.queueWrite(s.ticketApprove)(w, r) }},
	{"POST /v1/tickets/retry", func(s *Server, w http.ResponseWriter, r *http.Request) { s.retryWrite(false)(w, r) }},
	// relaunch is the old name of retry, kept for one release.
	{"POST /v1/tickets/relaunch", func(s *Server, w http.ResponseWriter, r *http.Request) { s.queueWrite(s.ticketRelaunch)(w, r) }},
	{"POST /v1/epics/merge", func(s *Server, w http.ResponseWriter, r *http.Request) { s.queueWrite(s.epicMerge)(w, r) }},
	{"POST /v1/epics/stash-merge", func(s *Server, w http.ResponseWriter, r *http.Request) { s.queueWrite(s.epicStashMerge)(w, r) }},
	{"POST /v1/agents/prompt", func(s *Server, w http.ResponseWriter, r *http.Request) { s.queueWrite(s.agentPrompt)(w, r) }},
	{"POST /v1/agents/interrupt", func(s *Server, w http.ResponseWriter, r *http.Request) { s.queueWrite(s.agentInterrupt)(w, r) }},
	{"POST /v1/agents/answer", func(s *Server, w http.ResponseWriter, r *http.Request) { s.queueWrite(s.agentAnswer)(w, r) }},
	// Interactive sessions; /v1/agents/* is the loop agents'.
	{"GET /v1/sessions", (*Server).sessionsRead},
	{"GET /v1/sessions/chat", (*Server).sessionChatRead},
	{"GET /v1/sessions/screen", (*Server).sessionScreenRead},
	{"POST /v1/sessions/create", (*Server).sessionCreateWrite},
	{"POST /v1/sessions/stop", func(s *Server, w http.ResponseWriter, r *http.Request) { sessionWrite(s.stopSession)(w, r) }},
	{"POST /v1/sessions/remove", func(s *Server, w http.ResponseWriter, r *http.Request) { sessionWrite(s.removeSession)(w, r) }},
	{"POST /v1/sessions/mark-read", func(s *Server, w http.ResponseWriter, r *http.Request) { sessionWrite(s.markSessionRead)(w, r) }},
	{"POST /v1/sessions/focus", (*Server).sessionFocusWrite},
	{"POST /v1/sessions/show", func(s *Server, w http.ResponseWriter, r *http.Request) { sessionWrite(s.focusSession)(w, r) }},
	{"POST /v1/sessions/actionable", (*Server).sessionActionableWrite},
	{"POST /v1/sessions/link", (*Server).sessionLinkWrite},
	{"POST /v1/sessions/step", (*Server).sessionStepWrite},
	{"POST /v1/sessions/run-action", func(s *Server, w http.ResponseWriter, r *http.Request) { sessionWrite(s.runSessionAction)(w, r) }},
	{"POST /v1/sessions/resume", func(s *Server, w http.ResponseWriter, r *http.Request) { sessionWrite(s.resumeSession)(w, r) }},
	{"POST /v1/sessions/prompt", (*Server).sessionPromptWrite},
	{"POST /v1/sessions/interrupt", func(s *Server, w http.ResponseWriter, r *http.Request) { sessionWrite(s.interruptSession)(w, r) }},
	{"POST /v1/sessions/auto-advance", (*Server).sessionAutoAdvanceWrite},
	{"POST /v1/sessions/auto-advance/cancel", func(s *Server, w http.ResponseWriter, r *http.Request) { sessionWrite(s.cancelAutoAdvanceRoute)(w, r) }},
	{"POST /v1/designs/open", (*Server).designOpenWrite},
	{"POST /v1/designs/delete", (*Server).designDeleteWrite},
	{"POST /v1/designs/worktree", (*Server).designWorktreeWrite},
	{"POST /v1/tickets/fix-agent", (*Server).fixAgentWrite},
	{"POST /v1/afk", (*Server).afkWrite},
	{"POST /v1/checks/save", (*Server).checksSaveWrite},
}

// RoutePatterns lists every registered "METHOD /path" pattern.
func RoutePatterns() []string {
	out := make([]string, len(routeTable))
	for i, r := range routeTable {
		out[i] = r.pattern
	}
	return out
}

// listenLoopback refuses any address that isn't a loopback IP, since the TCP
// listener has no auth.
func listenLoopback(addr string) (net.Listener, error) {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	if ip := net.ParseIP(host); ip == nil || !ip.IsLoopback() {
		return nil, fmt.Errorf("tcp listener %q: host must be a loopback IP", addr)
	}
	return net.Listen("tcp", addr)
}

// TCPAddr is the bound TCP listener address, or "" when TCP is off.
func (s *Server) TCPAddr() string {
	if s.tcp == nil {
		return ""
	}
	return s.tcp.Addr().String()
}

func (s *Server) snapshot(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	snap := s.idx.snapshot()
	snap.HerdrUnavailable = s.herdr.isUnavailable()
	for i, t := range snap.Tickets {
		switch t.Status {
		case string(schema.StatusClaimed):
			snap.Tickets[i].ClaimedAt, _ = s.registry.startedAtOf(t.Address)
		case string(schema.StatusPaused), string(schema.StatusLegacyAnswer), string(schema.StatusLegacyRepair):
			snap.Tickets[i].Recovery = recoveryState(s.pauseHold.state(t.Address), t.Address, snap.Tickets)
		}
	}
	s.fillEpicMergeTargets(snap.Tickets)
	snap.Build = s.cfg.Build
	snap.Budget = s.budgetStatus(time.Now())
	snap.ExtraUsage = s.extraUsageOn()
	snap.Disks = s.listedDisks()
	snap.Health = s.currentHealth()
	snap.Pending = s.pendingRows()
	snap.Epics = s.epicSummaries(snap.Tickets)
	snap.RecentlyFinished = s.idx.recentlyFinished(s.settings.Load().RecentlyFinished)
	snap.Mode = s.pause.currentMode()
	snap.AFK = s.afk.isOn()
	snap.SkillsInstallProblem = skills.ReadProblem(skills.StatusPath(s.cfg.StateDir))
	// Checked on every snapshot: a PATH entry can vanish while the server
	// runs, like an fnm per-shell dir once its shell exits.
	_, nodeErr := brief.LookPath(s.cfg.Env, "node")
	snap.NodeMissing = nodeErr != nil
	sessions, err := s.listSessions()
	if err != nil {
		s.log.Warn("list sessions for snapshot", "err", err)
	}
	snap.Sessions = sessions
	_ = json.NewEncoder(w).Encode(snap)
}

// DefaultSSEHeartbeat is how long /v1/events may sit idle before it writes a
// comment line, so proxies and browsers do not drop a quiet stream.
const DefaultSSEHeartbeat = 15 * time.Second

// streamEvents is SSE: every event with seq > ?since, then live events until
// the client leaves or is dropped for lagging. 410 means re-snapshot. A
// ?client= id keeps that client's focus reports alive while the stream is.
// An idle stream gets a ":" comment line every SSEHeartbeat.
func (s *Server) streamEvents(w http.ResponseWriter, r *http.Request) {
	var since uint64
	if v := r.URL.Query().Get("since"); v != "" {
		n, err := strconv.ParseUint(v, 10, 64)
		if err != nil {
			http.Error(w, "bad since", http.StatusBadRequest)
			return
		}
		since = n
	}
	ch, cancel, err := s.events.subscribe(since)
	if err != nil {
		http.Error(w, err.Error(), http.StatusGone)
		return
	}
	defer cancel()
	if client := r.URL.Query().Get("client"); client != "" {
		s.focus.connect(client)
		defer s.focus.disconnect(client)
	}
	flusher, _ := w.(http.Flusher)
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	if flusher != nil {
		flusher.Flush()
	}
	interval := s.cfg.SSEHeartbeat
	if interval <= 0 {
		interval = DefaultSSEHeartbeat
	}
	beat := time.NewTicker(interval)
	defer beat.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-beat.C:
			if _, err := io.WriteString(w, ": heartbeat\n\n"); err != nil {
				return
			}
			if flusher != nil {
				flusher.Flush()
			}
		case ev, ok := <-ch:
			if !ok {
				return
			}
			data, _ := json.Marshal(ev)
			if _, err := fmt.Fprintf(w, "id: %d\nevent: %s\ndata: %s\n\n", ev.Seq, ev.Type, data); err != nil {
				return
			}
			if flusher != nil {
				flusher.Flush()
			}
		}
	}
}

func (s *Server) handshake(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(Handshake{APIVersion: APIVersion, Build: s.cfg.Build, Pid: os.Getpid(), TCPAddr: s.TCPAddr(), WebURL: s.WebURL(), WebError: s.webErr, HerdrUnavailable: s.herdr.isUnavailable()})
}

// DefaultMergeStopTimeout is how long a stop waits for a merge in flight.
const DefaultMergeStopTimeout = 30 * time.Second

// Serve blocks until ctx is cancelled, then shuts down and releases the lock.
func (s *Server) Serve(ctx context.Context) error {
	s.log.Info("server started", "pid", os.Getpid(), "build", s.cfg.Build, "socket", SocketPath(s.cfg.StateDir))
	s.seedBudgetNotes(time.Now())
	s.chat.Notice(ralphloop.ServerNotice{Kind: NoticeServerStarted, Emoji: "🚀", Title: "server started", Detail: "build " + s.cfg.Build})
	s.checkExtraUsage(time.Now())
	freshCtx, stopFresh := context.WithCancel(ctx)
	// Armed before the socket serves, so no save after startup goes unseen.
	s.startConfigWatch(freshCtx)
	// Before the socket serves, so a session created at startup is never
	// taken for one whose holder died while the server was down.
	s.restoreSessions()
	errc := make(chan error, 3)
	lns := []net.Listener{s.ln}
	go func() { errc <- s.http.Serve(s.ln) }()
	if s.tcp != nil {
		lns = append(lns, s.tcp)
		go func() { errc <- s.tcpHTTP.Serve(s.tcp) }()
	}
	shutdown := func() error {
		return errors.Join(s.http.Shutdown(context.Background()), s.tcpHTTP.Shutdown(context.Background()))
	}
	if s.web != nil {
		shutdown = func() error {
			return errors.Join(s.http.Shutdown(context.Background()), s.tcpHTTP.Shutdown(context.Background()), s.webHTTP.Shutdown(context.Background()))
		}
		lns = append(lns, s.web)
		go func() { errc <- s.webHTTP.Serve(s.web) }()
	}
	s.recoverMerges()
	s.reclaimRuns()
	s.logFlushingRunners()
	s.pruneAgentLogs(time.Now())
	s.scanParentDefects()
	stopCommits := func() {}
	if stop, err := s.startStoreCommits(); err != nil {
		s.log.Error("store commit loop failed to start", "err", err)
	} else {
		stopCommits = stop
	}
	freshDone := make(chan struct{})
	go func() { defer close(freshDone); s.keepFresh(freshCtx) }()
	herdrDone := make(chan struct{})
	go func() { defer close(herdrDone); s.keepHerdrChecked(freshCtx) }()
	// A claim must not see a low disk before the watcher has recorded its projects.
	s.checkDisks()
	claimDone := make(chan struct{})
	go func() { defer close(claimDone); s.keepClaiming(freshCtx) }()
	pruneDone := make(chan struct{})
	go func() { defer close(pruneDone); s.keepPruned(freshCtx) }()
	budgetDone := make(chan struct{})
	go func() { defer close(budgetDone); s.keepBudgetPolled(freshCtx) }()
	gatesDone := make(chan struct{})
	go func() { defer close(gatesDone); s.keepGatesWatched(freshCtx) }()
	cpuDone := make(chan struct{})
	go func() { defer close(cpuDone); s.keepCPUWatched(freshCtx) }()
	disksDone := make(chan struct{})
	go func() { defer close(disksDone); s.keepDisksWatched(freshCtx) }()
	healthDone := make(chan struct{})
	go func() { defer close(healthDone); s.keepHealthWatched(freshCtx) }()
	sweepDone := make(chan struct{})
	go func() { defer close(sweepDone); s.sweepDesignCounts() }()
	var err error
	select {
	case <-ctx.Done():
		s.events.close() // open streams would otherwise hold Shutdown
		err = shutdown()
		for range lns {
			<-errc
		}
	case err = <-errc:
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		_ = shutdown()
	}
	// Stop claiming, let a merge in flight finish, then flush the store commit
	// loop, and only then release the lock. Agents in their panes are not touched.
	stopFresh()
	s.sessionWatches.close()
	<-freshDone
	<-herdrDone
	<-claimDone
	<-pruneDone
	<-budgetDone
	<-gatesDone
	<-cpuDone
	<-disksDone
	<-healthDone
	<-sweepDone
	// A watcher reload still in flight can start a countdown, so stop it after.
	s.configWatch.running.Wait()
	s.stopHardGrace()
	timeout := s.cfg.MergeStopTimeout
	if timeout <= 0 {
		timeout = DefaultMergeStopTimeout
	}
	if !s.merges.closeAndWait(timeout) {
		s.log.Warn("stopping with a merge still in flight", "timeout", timeout)
	}
	stopCommits()
	s.chat.Close()
	_ = os.Remove(SocketPath(s.cfg.StateDir))
	s.log.Info("server stopped")
	_ = s.logf.Close()
	s.lock.release()
	if err != nil {
		return fmt.Errorf("serve: %w", err)
	}
	return nil
}

func (s Settings) storeCommitDebounce() time.Duration {
	if s.StoreCommitDebounce <= 0 {
		return time.Duration(config.DefaultCommitDebounceSeconds) * time.Second
	}
	return s.StoreCommitDebounce
}

// startStoreCommits makes the server the store's committer and commits edits
// made while it was down.
func (s *Server) startStoreCommits() (stop func(), err error) {
	if s.cfg.TicketStore == "" {
		return func() {}, nil
	}
	s.reloadMu.Lock()
	set := s.settings.Load()
	loop, err := storecommit.StartWith(s.cfg.TicketStore, storecommit.Options{
		Debounce:   set.storeCommitDebounce(),
		PushRemote: set.StorePushRemote,
		OnError:    func(err error) { s.log.Warn("store push failed", "err", err) },
		Env:        s.cfg.Env,
	})
	s.storeCommits = loop
	s.reloadMu.Unlock()
	if err != nil {
		return nil, err
	}
	if err := loop.Flush(); err != nil {
		s.log.Warn("committing down-time store edits failed", "err", err)
	}
	return loop.Stop, nil
}
