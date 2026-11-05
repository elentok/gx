// Package server is the orchestrator daemon: HTTP/JSON over a unix socket in
// the state dir, routes under /v1/. Its writes are the server-wide queue in the
// state dir and the claim of a queued root's frontier ticket.
package server

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/elentok/gx/server/sessions"
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

	"github.com/elentok/gx/server/queue"
	srvrecovery "github.com/elentok/gx/server/recovery"
	"github.com/elentok/gx/server/runs"

	"github.com/elentok/gx/agentrunner/herdrrunner"
	"github.com/elentok/gx/brief"
	"github.com/elentok/gx/config"
	"github.com/elentok/gx/conversations"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/osnotify"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/server/broker"
	"github.com/elentok/gx/server/budget"
	"github.com/elentok/gx/server/disk"
	harnesschoice "github.com/elentok/gx/server/harness"
	"github.com/elentok/gx/server/health"
	"github.com/elentok/gx/server/index"
	"github.com/elentok/gx/server/limits"
	"github.com/elentok/gx/server/projects"
	"github.com/elentok/gx/server/sessions/chat"
	"github.com/elentok/gx/server/settings"
	"github.com/elentok/gx/server/stats"
	"github.com/elentok/gx/server/ticketwrite"
	"github.com/elentok/gx/server/webauth"
	"github.com/elentok/gx/server/webtls"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/skills"
	"github.com/elentok/gx/sound"
	"github.com/elentok/gx/syshealth"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
	"github.com/elentok/gx/tokenledger"
)

const (
	lockFileName   = "server.lock"
	socketFileName = "server.sock"
)

// NoticeServerStarted is the chat notice kind sent when Serve begins.
const NoticeServerStarted = "server-started"

// SocketPath is where the server listens inside stateDir.
func SocketPath(stateDir string) string { return filepath.Join(stateDir, socketFileName) }

// Settings are the Config fields a config reload may change. The server reads
// them through an atomic snapshot (Server.settings.Load), never from Config.
// It is an alias so the many readers and hand-built test values keep the name.
type Settings = settings.Settings

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
	// routes is what the moved subsystems serve (subsystemRoutes), gathered once in New.
	routes []wire.Route
	// instanceID is new on every start: a restart is done when it changes.
	instanceID string
	// settings is the live copy of cfg.Settings, its reload, status, config
	// watcher and store commit loop. A reader loads it once per decision; a
	// reload swaps in a new value and never mutates the old one.
	settings settings.Holder
	// restarting is set once a restart is under way and never cleared on
	// success: the new server replaces this one.
	restarting atomic.Bool
	disks      *disk.Disk
	health     *health.Health
	// sessionChat reads the sessions' transcripts, commands, permissions and screens.
	sessionChat *chat.Chat
	// autoAdvances holds the running auto-advance countdowns.
	autoAdvances autoAdvances
	lock         *serverLock
	ln           net.Listener
	tcp          net.Listener // nil unless Config.TCPAddr is set
	http         *http.Server
	tcpHTTP      *http.Server // the socket's routes behind refuseBrowsers
	web          net.Listener // nil unless Config.WebAddr is set and bound
	webDev       http.Handler // nil unless Config.WebDevURL is set and valid
	webHTTP      *http.Server
	webErr       string        // why the web bind failed
	webListenIP  net.IP        // nil when the listener binds every interface
	webCerts     *webtls.Certs // nil unless the web listener is bound
	clientErrs   ipLimiter
	// webAuth is the web login store; nil unless the web listener is bound.
	webAuth    *webauth.Gate
	webStreams webStreams
	logf       *rotatingFile
	log        *slog.Logger
	idx        *index.Index
	harness    *harnesschoice.Harness
	projects   *projects.Registry
	migrator   *projects.Migrator
	checks     *projects.Checks

	queue        *queue.Queue
	afk          *queue.AFK
	viMode       *queue.ViMode
	oneOffs      *queue.OneOffs
	budget       *budget.Budget
	tokens       *tokenledger.Ledger
	limits       *limits.Limits
	stats        *stats.Stats
	events       *broker.Broker
	herdr        herdrWatch
	writer       *ticketwrite.Writer
	pauseHold    ticketwrite.RecoveryHold
	recovery     *srvrecovery.Recovery
	defectScan   sync.Mutex
	lateFinishes inFlight
	designSteps  sync.Map // map address -> design step at the last rescan
	deadlocks    deadlocks
	gatesRaised  map[gateHold]bool // touched only by the gate watchdog

	chat         *ralphloop.ServerChat // never nil; sends nothing when no destination is configured
	registry     *runs.Registry
	savedRuns    []runs.TrackedRun // handles the previous server left; consumed by reclaimRuns
	guards       *runs.Guards
	mergeTargets mergeTargetCache
	projectRepos projectRepoCache
	mergeInto    func(git.Repo, []git.Worktree, string, string) (git.MergeOutcome, error) // swapped in tests
	mergedProbe  func(repo, ancestor, descendant string) (bool, error)                    // the poll-time ancestry check; swapped in tests
	verdicts     verdictLog
	trunkMissing missingTrunks
	// sessions creates, stops and watches the sessions; resumer brings them back.
	sessions    *sessions.Manager
	resumer     *sessions.Resumer
	designOpens sync.Map // epic address -> *sync.Mutex, serializing designs open
	focus       *sessions.FocusReports
}

// New prepares the state dir, takes the server lock and binds the socket.
// It does not serve until Serve is called.
func New(cfg Config) (*Server, error) {
	if cfg.Env == nil {
		cfg.Env = os.Environ()
	}
	if cfg.LookPath == nil {
		cfg.LookPath = harnesschoice.DefaultLookPath(cfg.Env)
	}
	if cfg.Runner == nil {
		return nil, errors.New("server: Config.Runner is required: the server builds no default runner")
	}
	if cfg.Notifier == nil {
		cfg.Notifier = osnotify.NewSystemNotifier()
	}
	if cfg.ProcessSource == nil {
		cfg.ProcessSource = syshealth.DefaultProcess()
	}
	if cfg.SessionBannerDelay == 0 {
		cfg.SessionBannerDelay = sessions.DefaultSessionBannerDelay
	}
	if cfg.ChatPollInterval == 0 {
		cfg.ChatPollInterval = chat.DefaultPollInterval
	}
	if cfg.Chat.GateStatePath == "" {
		cfg.Chat.GateStatePath = ralphloop.NotificationStatePath(cfg.StateDir)
	}
	if cfg.StallThreshold == 0 {
		cfg.StallThreshold = chat.DefaultStallThreshold
	}
	if cfg.TurnSettleTimeout == 0 {
		cfg.TurnSettleTimeout = defaultTurnSettleTimeout
	}
	if cfg.TranscriptMissTTL == 0 {
		cfg.TranscriptMissTTL = chat.DefaultTranscriptMissTTL
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
		if err := projects.EnsureScratchProject(cfg.TicketStore); err != nil {
			lock.release()
			return nil, fmt.Errorf("create scratch project: %w", err)
		}
		if err := projects.PruneScratchWorkspace(cfg.TicketStore); err != nil {
			lock.release()
			return nil, fmt.Errorf("prune scratch workspace: %w", err)
		}
	}
	events := broker.New(cfg.SubscriberBuffer)
	q, err := queue.OpenQueue(cfg.StateDir)
	if err != nil {
		lock.release()
		return nil, fmt.Errorf("load queue: %w", err)
	}
	afk, err := queue.OpenAFK(cfg.StateDir, events)
	if err != nil {
		lock.release()
		return nil, fmt.Errorf("load afk mode: %w", err)
	}
	viMode, err := queue.OpenViMode(cfg.StateDir, events)
	if err != nil {
		lock.release()
		return nil, fmt.Errorf("load vi mode: %w", err)
	}
	tokens, err := tokenledger.Open(cfg.StateDir)
	if err != nil {
		lock.release()
		return nil, fmt.Errorf("load token ledger: %w", err)
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
	idx := index.New(cfg.TicketStore, events, log)
	registry, savedRuns, err := runs.Open(cfg.StateDir, log)
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

		instanceID: newInstanceID(),

		queue:       q,
		afk:         afk,
		viMode:      viMode,
		tokens:      tokens,
		mergeInto:   git.MergeBranchInto,
		mergedProbe: git.IsAncestor,
		events:      events,
		guards:      runs.NewGuards(),

		focus: sessions.NewFocusReports(),
	}
	s.initProjects()
	s.harness = harnesschoice.New(harnesschoice.Deps{
		Config: harnesschoice.Config{
			Env: cfg.Env, LookPath: cfg.LookPath, Runner: cfg.Runner, RunnerFor: cfg.RunnerFor,
			AgentRunner: cfg.AgentRunner, ConfigPath: cfg.ConfigPath,
		},
		Host: s, Log: log, Index: idx, Projects: s.projects, Queue: q, Registry: registry, Settings: &s.settings,
	})
	// The ledger loads here, after the listeners, so a failure releases them as a runs failure does.
	s.budget, err = budget.New(budget.Deps{
		Config: budget.Config{
			StateDir: cfg.StateDir, SoftLimit: cfg.BudgetSoftLimit, HardLimit: cfg.BudgetHardLimit,
			KillGrace: cfg.BudgetKillGrace, PollInterval: cfg.BudgetPollInterval,
			HardLimitGrace: cfg.HardLimitGrace, EventInterval: cfg.BudgetEventInterval, Env: cfg.Env,
			ExtraUsageCheck: cfg.ExtraUsageCheck,
		},
		Settings: &s.settings, Projects: s.projects, Index: idx, Registry: registry, Harness: s.harness,
		Events: events, Guards: s.guards, Chat: s.chat, Log: log, Host: budgetHost{s},
	})
	if err != nil {
		ln.Close()
		if tcp != nil {
			tcp.Close()
		}
		_ = logf.Close()
		lock.release()
		return nil, fmt.Errorf("load budget ledger: %w", err)
	}
	host := queueHost{s, s.harness}
	s.queue.Attach(queue.QueueDeps{Events: events, Guards: s.guards, Index: idx, Projects: s.projects, Registry: registry, Host: host})
	s.oneOffs = queue.NewOneOffs(queue.OneOffsDeps{Queue: s.queue, Index: idx, Projects: s.projects, Registry: registry, Events: events, Log: log, Chat: s.chat, Host: host, TicketStore: cfg.TicketStore})
	s.harness.GiveLog(cfg.Runner)
	if s.cfg.Player == nil {
		s.cfg.Player = sound.NewSystemPlayer(log)
	}
	s.writer = newTicketWriter(s)
	s.recovery = newRecovery(s)
	s.settings.Init(settings.Options{
		ConfigPath: cfg.ConfigPath,
		Startup: settings.Startup{
			TicketStore: cfg.TicketStore,
			TCPListen:   cfg.TCPAddr != "",
			WebAddr:     cfg.WebAddr,
			WebDevURL:   cfg.WebDevURL,
			AgentRunner: cfg.AgentRunner,
		},
		WatchDebounce: cfg.ConfigWatchDebounce,
		Notifier:      cfg.Notifier,
		Log:           log,
		Events:        events,
		Env:           cfg.Env,
		HardStopAt:    s.budget.HardStopAt,
	})
	s.settings.Store(&cfg.Settings)
	s.settings.SetAppliers(s.reloadAppliers())
	s.disks = disk.New(disk.Deps{
		Config: disk.Config{
			FreeSpace: cfg.FreeSpace, GoCacheDir: cfg.GoCacheDir, DiskPollInterval: cfg.DiskPollInterval,
			CleanupRunner: cfg.CleanupRunner, CleanupTimeout: cfg.CleanupTimeout,
			TempSweepDir: cfg.TempSweepDir, TempSweepMinAge: cfg.TempSweepMinAge,
			CacheGateCap: cfg.CacheGateCap, CacheGateScan: cfg.CacheGateScan,
			StateDir: cfg.StateDir, TicketStore: cfg.TicketStore, Env: cfg.Env,
		},
		Settings: &s.settings, Projects: s.projects, Events: events, Chat: s.chat, Guards: s.guards, Log: log,
		HealthSink: func(now time.Time) { s.health.ObserveDisk(now) },
	})
	s.health = health.New(health.Deps{
		Config: health.Config{
			Build: cfg.Build, StateDir: cfg.StateDir, SessionsDir: cfg.SessionsDir,
			SystemHealth: cfg.SystemHealth, HealthPollInterval: cfg.HealthPollInterval,
			ProcessSource: cfg.ProcessSource, ProcessSecondReadDelay: cfg.ProcessSecondReadDelay,
			ProcessShareWindow: cfg.ProcessShareWindow, ProcessCutoff: cfg.ProcessCutoff, ProcessWaitCap: cfg.ProcessWaitCap,
		},
		Settings: &s.settings, Projects: s.projects, Events: events, Disks: s.disks, OneOffs: s.oneOffs,
		Registry: s.registry, Harness: s.harness, Sessions: sessions.SessionMetas{}, Log: log,
	})
	s.limits = limits.New(limits.Deps{
		Config: limits.Config{
			StateDir: cfg.StateDir, TicketStore: cfg.TicketStore,
			UsagePollInterval: cfg.UsagePollInterval, LimitsEventInterval: cfg.LimitsEventInterval,
			TokenPollInterval: cfg.TokenPollInterval,
		},
		Settings: &s.settings, Projects: s.projects, Events: events, Guards: s.guards, Log: log,
		Registry: s.registry, Tokens: s.tokens, TokenHost: tokenHost{s},
		SpendDays: s.budget.DaysCopy,
	})
	s.stats = stats.New(stats.Deps{
		TicketStore: cfg.TicketStore, ConversationsDir: cfg.ConversationsDir,
		Tokens: s.tokens, Burns: s.limits, MapEpics: tokenHost{s},
	})
	s.sessionChat = chat.New(chat.Deps{
		Config: chat.Config{
			TranscriptMissTTL: cfg.TranscriptMissTTL, ChatPollInterval: cfg.ChatPollInterval,
			StallThreshold: cfg.StallThreshold, CommandListAge: cfg.CommandListAge,
			Env: cfg.Env, StateDir: cfg.StateDir,
		},
		Source: chatSource{s}, Log: log,
	})
	// The Manager copies projects, harness and sessionChat, so it comes after them.
	s.sessions = newManager(s)
	s.resumer = newResumer(s)
	if h, ok := cfg.Runner.(*herdrrunner.Runner); ok {
		h.Finished = s.sessions.SessionFinished
	}
	mux := http.NewServeMux()
	for _, r := range routeTable {
		mux.HandleFunc(r.pattern, func(w http.ResponseWriter, req *http.Request) { r.handler(s, w, req) })
	}
	// The rescan steps run in this order after every store scan.
	s.idx.OnRescan(s.scanParentDefects)
	s.idx.OnRescan(s.scanLateFinishes)
	s.idx.OnRescan(s.oneOffs.Sweep)
	s.idx.OnRescan(s.scanDesignCleanups)
	s.idx.SetReady(s.migrationsEnded)
	s.routes = subsystemRoutes(s)
	registerRoutes(mux, s.routes)
	s.http = &http.Server{Handler: mux}
	s.tcpHTTP = &http.Server{Handler: refuseBrowsers(mux, log)}
	s.bindWeb()
	// A down herdr never stops the server; it is reported and retried.
	s.checkHerdr()
	return s, nil
}

// routeTable is the single list of endpoints. cmd's parity test diffs it
// against the CLI command tree, so every route needs a `--json` verb.
var routeTable = []struct {
	pattern string
	handler func(*Server, http.ResponseWriter, *http.Request)
}{
	{"GET /v1/handshake", (*Server).handshake},
	{"GET /v1/web/info", (*Server).webInfo},
	{"POST /v1/web/new-root", (*Server).webNewRoot},
	{"POST /v1/web/reset", (*Server).webReset},
	{"POST /v1/web/reset-authenticator", (*Server).webResetAuthenticator},
	{"GET /v1/web/login-list", (*Server).webLoginListCLI},
	{"POST /v1/web/login-revoke", (*Server).webLoginRevokeCLI},
	{"POST /v1/web/set-password", (*Server).webSetPasswordCLI},
	{"GET /v1/snapshot", (*Server).snapshot},
	{"GET /v1/events", (*Server).streamEvents},
	{"GET /v1/restart/check", (*Server).restartCheckRead},
	{"POST /v1/restart", (*Server).restartWrite},
	{"GET /v1/tickets/explain", (*Server).explain},
	{"GET /v1/tickets/detail", (*Server).detail},
	{"GET /v1/epics", (*Server).epicList},
	{"GET /v1/epics/detail", (*Server).epicDetail},
	{"GET /v1/iterations", (*Server).iterations},
	{"GET /v1/queue", (*Server).queueView},
	{"POST /v1/tickets/merge", func(s *Server, w http.ResponseWriter, r *http.Request) { repairWrite(s.mergeInQueue)(w, r) }},
	{"POST /v1/tickets/start-over", func(s *Server, w http.ResponseWriter, r *http.Request) { repairWrite(s.startOverInQueue)(w, r) }},
	{"POST /v1/tickets/resume", func(s *Server, w http.ResponseWriter, r *http.Request) { repairWrite(s.repairResume)(w, r) }},
	{"POST /v1/tickets/unpark", func(s *Server, w http.ResponseWriter, r *http.Request) { repairWrite(s.repairResume)(w, r) }},
	{"POST /v1/tickets/verify", func(s *Server, w http.ResponseWriter, r *http.Request) { repairWrite(s.repairVerify)(w, r) }},
	{"POST /v1/tickets/unmute", func(s *Server, w http.ResponseWriter, r *http.Request) { queue.Handler(s.ticketUnmute)(w, r) }},
	{"POST /v1/tickets/cancel", func(s *Server, w http.ResponseWriter, r *http.Request) { queue.Handler(s.ticketCancel)(w, r) }},
	{"POST /v1/tickets/recommend", func(s *Server, w http.ResponseWriter, r *http.Request) { queue.HandlerCtx(s.ticketRecommend)(w, r) }},
	{"POST /v1/tickets/answer", func(s *Server, w http.ResponseWriter, r *http.Request) { queue.Handler(s.ticketAnswer)(w, r) }},
	{"POST /v1/tickets/ask", func(s *Server, w http.ResponseWriter, r *http.Request) { queue.Handler(s.ticketAsk)(w, r) }},
	{"POST /v1/tickets/retry", func(s *Server, w http.ResponseWriter, r *http.Request) { s.retryWrite(false)(w, r) }},
	{"POST /v1/epics/merge", func(s *Server, w http.ResponseWriter, r *http.Request) { queue.Handler(s.epicMerge)(w, r) }},
	{"POST /v1/epics/stash-merge", func(s *Server, w http.ResponseWriter, r *http.Request) { queue.Handler(s.epicStashMerge)(w, r) }},
	{"POST /v1/agents/prompt", func(s *Server, w http.ResponseWriter, r *http.Request) { queue.Handler(s.agentPrompt)(w, r) }},
	{"POST /v1/agents/interrupt", func(s *Server, w http.ResponseWriter, r *http.Request) { queue.Handler(s.agentInterrupt)(w, r) }},
	{"POST /v1/agents/answer", func(s *Server, w http.ResponseWriter, r *http.Request) { queue.Handler(s.agentAnswer)(w, r) }},
	// Interactive sessions; /v1/agents/* is the loop agents'.
	{"POST /v1/sessions/actionable", (*Server).sessionActionableWrite},
	{"POST /v1/sessions/run-action", func(s *Server, w http.ResponseWriter, r *http.Request) {
		sessions.SessionWrite(s.runSessionAction)(w, r)
	}},
	{"POST /v1/sessions/prompt", (*Server).sessionPromptWrite},
	{"POST /v1/sessions/keys", (*Server).sessionKeysWrite},
	{"POST /v1/sessions/auto-advance", (*Server).sessionAutoAdvanceWrite},
	{"POST /v1/sessions/auto-advance/cancel", func(s *Server, w http.ResponseWriter, r *http.Request) {
		sessions.SessionWrite(s.cancelAutoAdvanceRoute)(w, r)
	}},
	{"POST /v1/designs/open", (*Server).designOpenWrite},
	{"POST /v1/designs/delete", (*Server).designDeleteWrite},
	{"POST /v1/designs/worktree", (*Server).designWorktreeWrite},
}

// RoutePatterns lists every registered "METHOD /path" pattern.
func RoutePatterns() []string {
	out := make([]string, len(routeTable))
	for i, r := range routeTable {
		out[i] = r.pattern
	}
	for _, r := range subsystemRoutes(nil) {
		if r.Handler != nil {
			out = append(out, r.Pattern)
		}
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
	snap, agents := s.idx.SnapshotAgents()
	snap.HerdrText, snap.HerdrUnavailable = s.herdrBanner()
	for i, t := range snap.Tickets {
		if addr, err := tickets.ParseAddress(t.Address, tickets.AddressContext{}); err == nil {
			snap.Tickets[i].HarnessLabel = s.harness.LabelFor(addr, agents[i], cmp.Or(t.RenderedStatus, t.Status))
		}
		switch t.Status {
		case string(schema.StatusClaimed):
			snap.Tickets[i].ClaimedAt, _ = s.registry.StartedAtOf(t.Address)
		case string(schema.StatusPaused), string(schema.StatusLegacyAnswer), string(schema.StatusLegacyRepair):
			snap.Tickets[i].Recovery = ticketwrite.RecoveryState(s.pauseHold.State(t.Address), t.Address, snap.Tickets)
		}
	}
	s.fillEpicMergeTargets(snap.Tickets)
	snap.Build = s.cfg.Build
	snap.Instance = s.instanceID
	snap.MigrationFailures = s.MigrationFailures()
	snap.Migrating = s.migrator.Migrating()
	snap.Budget = s.budget.Status(time.Now())
	snap.ExtraUsage = s.budget.ExtraUsageOn()
	snap.Disks = s.disks.Listed()
	snap.Health = s.health.Current()
	snap.Pending = s.pendingRows()
	snap.Epics = s.epicSummaries(snap.Tickets)
	snap.RecentlyFinished = recentlyFinished(s.idx.Loaded(), s.settings.Load().RecentlyFinished)
	snap.Mode = s.queue.Mode()
	snap.AFK = s.afk.IsOn()
	snap.ViMode = s.viMode.IsOn()
	snap.SkillsInstallProblem = skills.ReadProblem(skills.StatusPath(s.cfg.StateDir))
	// Checked on every snapshot: a PATH entry can vanish while the server
	// runs, like an fnm per-shell dir once its shell exits.
	_, nodeErr := brief.LookPath(s.cfg.Env, "node")
	snap.NodeMissing = nodeErr != nil
	sessions, err := s.queue.ListSessionsWithQueue()
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
	ch, cancel, err := s.events.Subscribe(since)
	if err != nil {
		http.Error(w, err.Error(), http.StatusGone)
		return
	}
	defer cancel()
	ctx := r.Context()
	if id := webauth.LoginIDFrom(ctx); id != "" {
		var untrack func()
		ctx, untrack = s.webStreams.track(ctx, id)
		defer untrack()
	}
	if client := r.URL.Query().Get("client"); client != "" {
		s.focus.Connect(client)
		defer s.focus.Disconnect(client)
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
		case <-ctx.Done():
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
	h := wire.Handshake{APIVersion: wire.APIVersion, Build: s.cfg.Build, Pid: os.Getpid(), TCPAddr: s.TCPAddr(), WebURL: s.WebURL(), WebError: s.webErr}
	h.HerdrText, h.HerdrUnavailable = s.herdrBanner()
	h.Migrating = s.migrator.Migrating()
	if s.web != nil {
		var err error
		if h.WebSetupPending, _, err = s.webAuth.SetupState(); err != nil {
			s.log.Error("web setup state unavailable", "err", err)
		}
	}
	_ = json.NewEncoder(w).Encode(h)
}

// DefaultMergeStopTimeout is how long a stop waits for a merge in flight.
const DefaultMergeStopTimeout = 30 * time.Second

// Serve blocks until ctx is cancelled, then shuts down and releases the lock.
func (s *Server) Serve(ctx context.Context) error {
	s.log.Info("server started", "pid", os.Getpid(), "build", s.cfg.Build, "socket", SocketPath(s.cfg.StateDir))
	s.budget.SeedNotes(time.Now())
	s.chat.Notice(ralphloop.ServerNotice{Kind: NoticeServerStarted, Emoji: "🚀", Title: "server started", Detail: "build " + s.cfg.Build})
	s.budget.StartExtraUsageCheck(time.Now())
	freshCtx, stopFresh := context.WithCancel(ctx)
	// Armed before the socket serves, so no save after startup goes unseen.
	s.settings.StartConfigWatch(freshCtx)
	// Before the socket serves, so a session created at startup is never
	// taken for one whose holder died while the server was down.
	s.resumer.RestoreSessions()
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
	go s.migrator.MigrateAll(ctx)
	// recoverMerges, reclaimRuns, the log prune and the first scan run in
	// keepFresh once the migrations end.
	stopCommits := func() {}
	if stop, err := s.settings.StartStoreCommits(); err != nil {
		s.log.Error("store commit loop failed to start", "err", err)
	} else {
		stopCommits = stop
	}
	freshDone := make(chan struct{})
	go func() {
		defer close(freshDone)
		s.idx.KeepFresh(freshCtx, index.WatchConfig{
			PollInterval: s.cfg.PollInterval,
			DisableWatch: s.cfg.DisableWatch,
			Start:        s.startAfterMigrations,
		})
	}()
	herdrDone := make(chan struct{})
	go func() { defer close(herdrDone); s.keepHerdrChecked(freshCtx) }()
	// A claim must not see a low disk before the watcher has recorded its projects.
	s.disks.Check()
	claimDone := make(chan struct{})
	go func() { defer close(claimDone); s.keepClaiming(freshCtx) }()
	pruneDone := make(chan struct{})
	go func() { defer close(pruneDone); s.sessions.Logs.KeepPruned(freshCtx) }()
	budgetDone := make(chan struct{})
	go func() { defer close(budgetDone); s.budget.KeepPolled(freshCtx) }()
	tokensDone := make(chan struct{})
	go func() { defer close(tokensDone); s.limits.KeepTokensPolled(freshCtx) }()
	gatesDone := make(chan struct{})
	go func() { defer close(gatesDone); s.keepGatesWatched(freshCtx) }()
	cpuDone := make(chan struct{})
	go func() { defer close(cpuDone); s.health.KeepCPUWatched(freshCtx) }()
	disksDone := make(chan struct{})
	go func() { defer close(disksDone); s.disks.Run(freshCtx) }()
	migFailDone := make(chan struct{})
	go func() { defer close(migFailDone); s.migrator.KeepFailuresWatched(freshCtx) }()
	healthDone := make(chan struct{})
	go func() { defer close(healthDone); s.health.KeepWatched(freshCtx) }()
	usageDone := make(chan struct{})
	go func() { defer close(usageDone); s.limits.Run(freshCtx) }()
	sweepDone := make(chan struct{})
	go func() { defer close(sweepDone); s.sessions.SweepDesignCounts() }()
	s.health.FileStartIncident(time.Now())
	var err error
	select {
	case <-ctx.Done():
		s.events.Close() // open streams would otherwise hold Shutdown
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
	recordShutdown("stop-fresh")
	stopFresh()
	recordShutdown("limits-events-stop")
	s.limits.Stop()
	recordShutdown("budget-events-stop")
	s.budget.StopEvents()
	recordShutdown("session-watches-close")
	s.sessions.Watches.Close()
	for _, w := range []struct {
		name string
		done chan struct{}
	}{
		{"wait-fresh", freshDone},
		{"wait-herdr", herdrDone},
		{"wait-claim", claimDone},
		{"wait-prune", pruneDone},
		{"wait-budget", budgetDone},
		{"wait-tokens", tokensDone},
		{"wait-gates", gatesDone},
		{"wait-cpu", cpuDone},
		{"wait-disks", disksDone},
		{"wait-migration-failures", migFailDone},
		{"wait-health", healthDone},
		{"wait-usage", usageDone},
		{"wait-sweep", sweepDone},
	} {
		recordShutdown(w.name)
		<-w.done
	}
	// A watcher reload still in flight can start a countdown, so stop it after.
	recordShutdown("config-watch-wait")
	s.settings.WaitConfigWatch()
	recordShutdown("stop-hard-grace")
	s.budget.StopHardGrace()
	timeout := s.cfg.MergeStopTimeout
	if timeout <= 0 {
		timeout = DefaultMergeStopTimeout
	}
	recordShutdown("merges-close-and-wait")
	if !s.guards.Merges.CloseAndWait(timeout) {
		s.log.Warn("stopping with a merge still in flight", "timeout", timeout)
	}
	recordShutdown("stop-commits")
	stopCommits()
	recordShutdown("chat-close")
	s.chat.Close()
	_ = os.Remove(SocketPath(s.cfg.StateDir))
	s.log.Info("server stopped")
	_ = s.logf.Close()
	recordShutdown("lock-release")
	s.lock.release()
	if err != nil {
		return fmt.Errorf("serve: %w", err)
	}
	return nil
}

// shutdownRecorder is nil in production; a test sets it to observe the order
// Serve stops its parts in.
var shutdownRecorder atomic.Pointer[func(step string)]

func recordShutdown(step string) {
	if rec := shutdownRecorder.Load(); rec != nil {
		(*rec)(step)
	}
}

// Settings is the settings subsystem, for tests that reach its seams from
// outside the package.
func (s *Server) Settings() *settings.Holder { return &s.settings }
