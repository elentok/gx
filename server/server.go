// Package server is the orchestrator daemon: HTTP/JSON over a unix socket in
// the state dir, routes under /v1/. Its writes are the server-wide queue in the
// state dir and the claim of a queued root's frontier ticket.
package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/agentrunner/herdrrunner"
	"github.com/elentok/gx/config"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/recovery"
	"github.com/elentok/gx/storecommit"
	"github.com/elentok/gx/subscription"
	"github.com/elentok/gx/tickets/schema"
)

// APIVersion is bumped on any incompatible change to the /v1 wire contract.
const APIVersion = 1

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
	// HerdrUnavailable is set while herdr isn't answering, so a client can say
	// why nothing starts.
	HerdrUnavailable bool `json:"herdr_unavailable,omitempty"`
}

type Config struct {
	StateDir    string
	Build       string
	TicketStore string   // ticket-store root; kept fresh by watch + poll
	TabEnv      []string // KEY=VALUE entries set on each iteration tab
	// AutoMergeEpic merges a finished epic's branch into its target; off leaves
	// the merge to gx-merge.
	AutoMergeEpic bool

	PollInterval time.Duration // zero means the default
	DisableWatch bool          // poll only

	HerdrRetryInterval time.Duration // zero means the default

	// Runner is the configured agent runner; nil means herdr. Herdr health
	// is only probed when it reports health (agentrunner.HealthChecker).
	Runner agentrunner.Runner

	BudgetPollInterval time.Duration // zero means the default

	// BudgetSoftLimit and BudgetHardLimit are config.Budget's daily limits in
	// dollars, reported in the budget status; zero means off.
	BudgetSoftLimit float64
	BudgetHardLimit float64

	BudgetKillGrace time.Duration // wait between ctrl+c and closing a pane; zero means the default

	SubscriberBuffer int // events a stream may lag behind before it is dropped; zero means the default

	// Recovery is the catalog with the user's kill switch and disables applied.
	// The zero value is off.
	Recovery recovery.Catalog
	// RecoverySettings is the loaded config recovery block, defaults applied.
	// An empty FollowUps means DefaultFollowUps; a zero NotifyHold sends a
	// park's chat message without waiting for recovery.
	RecoverySettings config.RecoveryConfig

	// StoreCommitDebounce and StorePushRemote configure the store commit loop,
	// A zero debounce means 60s.
	StoreCommitDebounce time.Duration
	StorePushRemote     string
	// LandStopTimeout bounds how long a stop waits for a land in flight; zero
	// means DefaultLandStopTimeout.
	LandStopTimeout time.Duration

	// MaxAgents caps live agents across all projects (config
	// execution-queue.max-agents); zero means the config default.
	MaxAgents int
	// MaxAgentsPerRoot caps live agents of one root (config
	// execution-queue.max-agents-per-epic). Zero means the default.
	MaxAgentsPerRoot int

	// TCPAddr, when set, adds a loopback-only TCP listener serving the same
	// handler as the socket. No auth: loopback is the only protection.
	TCPAddr string

	// Chat names the chat destinations the server notifies; empty means none.
	Chat ralphloop.ServerChatConfig

	// SuppressExtraUsageWarning is config subscription.suppress-extra-usage-warning.
	SuppressExtraUsageWarning bool
	// ExtraUsageCheck reads the subscription state; nil means subscription.CheckFresh.
	ExtraUsageCheck func() subscription.State
}

// DefaultTCPAddr is where the opt-in TCP listener binds.
const DefaultTCPAddr = "127.0.0.1:7421"

// Server owns the state-dir lock and the listening socket.
type Server struct {
	cfg  Config
	lock *serverLock
	ln   net.Listener
	tcp  net.Listener // nil unless Config.TCPAddr is set
	http *http.Server
	logf *rotatingFile
	log  *slog.Logger
	idx  *index

	queued      *queueStore
	pause       *pauseState
	ledger      *budgetLedger
	budgetNotes budgetNotes
	extraUsage  extraUsageNotes
	costOf      func(IterationInfo) (float64, bool) // swapped in tests
	events      *broker
	herdr       herdrWatch
	parkFold    parkFold
	parkHold    recoveryHold
	defectScan  sync.Mutex
	deadlocks   deadlocks
	gatesRaised map[gateHold]bool // touched only by the gate watchdog
	rewatch     func()            // set by keepFresh when the watch is active

	chat        *ralphloop.ServerChat // nil when no chat destination is configured
	registry    *runRegistry
	savedRuns   []trackedRun // handles the previous server left; consumed by reclaimRuns
	refused     refusals
	lands       landGuard
	verdicts    verdictLog
	nudges      nudgeLimiter
	unavailable unavailableNotes
	kick        chan struct{} // wakes keepClaiming
}

// New prepares the state dir, takes the server lock and binds the socket.
// It does not serve until Serve is called.
func New(cfg Config) (*Server, error) {
	if cfg.Runner == nil {
		cfg.Runner = herdrrunner.New()
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
	log := slog.New(slog.NewJSONHandler(logf, &slog.HandlerOptions{Level: slog.LevelInfo}))
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

		queued: queued,
		pause:  pause,
		ledger: ledger,
		costOf: iterationCost,
		events: events,
		kick:   make(chan struct{}, 1),
	}
	mux := http.NewServeMux()
	for _, r := range routeTable {
		mux.HandleFunc(r.pattern, func(w http.ResponseWriter, req *http.Request) { r.handler(s, w, req) })
	}
	s.http = &http.Server{Handler: mux}
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
	{"GET /v1/snapshot", (*Server).snapshot},
	{"GET /v1/events", (*Server).streamEvents},
	{"GET /v1/projects", (*Server).projects},
	{"POST /v1/projects/add", (*Server).projectAdd},
	{"POST /v1/projects/remove", func(s *Server, w http.ResponseWriter, r *http.Request) { s.projectWrite(s.removeProject)(w, r) }},
	{"POST /v1/projects/set-path", func(s *Server, w http.ResponseWriter, r *http.Request) { s.projectWrite(s.setProjectPath)(w, r) }},
	{"GET /v1/locks", (*Server).locks},
	{"GET /v1/budget", (*Server).budget},
	{"POST /v1/budget/override", (*Server).budgetOverrideWrite},
	{"POST /v1/budget/increase", (*Server).budgetIncreaseWrite},
	{"GET /v1/tickets/history", (*Server).history},
	{"GET /v1/tickets/explain", (*Server).explain},
	{"GET /v1/iterations", (*Server).iterations},
	{"POST /v1/tickets/changed", (*Server).ticketChanged},
	{"GET /v1/queue", (*Server).queue},
	{"GET /v1/queue/items", (*Server).queueItems},
	{"POST /v1/queue/add", func(s *Server, w http.ResponseWriter, r *http.Request) { s.queueWrite(s.queueAdd)(w, r) }},
	{"POST /v1/oneoff", (*Server).oneOffHandler},
	{"POST /v1/queue/remove", func(s *Server, w http.ResponseWriter, r *http.Request) { s.queueWrite(s.queueRemove)(w, r) }},
	{"POST /v1/queue/move", func(s *Server, w http.ResponseWriter, r *http.Request) { s.queueWrite(s.queueMove)(w, r) }},
	{"POST /v1/queue/replace", func(s *Server, w http.ResponseWriter, r *http.Request) { s.queueWrite(s.queueReplace)(w, r) }},
	{"POST /v1/queue/pause", func(s *Server, w http.ResponseWriter, r *http.Request) { s.modeWrite(s.queuePause)(w, r) }},
	{"POST /v1/queue/resume", func(s *Server, w http.ResponseWriter, r *http.Request) { s.modeWrite(s.queueResume)(w, r) }},
	{"POST /v1/queue/drain", func(s *Server, w http.ResponseWriter, r *http.Request) { s.modeWrite(s.queueDrain)(w, r) }},
	{"POST /v1/tickets/land", func(s *Server, w http.ResponseWriter, r *http.Request) { repairWrite(s.repairLand)(w, r) }},
	{"POST /v1/tickets/reset", func(s *Server, w http.ResponseWriter, r *http.Request) { repairWrite(s.repairReset)(w, r) }},
	{"POST /v1/tickets/unpark", func(s *Server, w http.ResponseWriter, r *http.Request) { repairWrite(s.repairUnpark)(w, r) }},
	{"POST /v1/tickets/verify", func(s *Server, w http.ResponseWriter, r *http.Request) { repairWrite(s.repairVerify)(w, r) }},
	{"POST /v1/tickets/park", func(s *Server, w http.ResponseWriter, r *http.Request) { s.queueWrite(s.ticketPark)(w, r) }},
	{"POST /v1/tickets/cancel", func(s *Server, w http.ResponseWriter, r *http.Request) { s.queueWrite(s.ticketCancel)(w, r) }},
	{"POST /v1/tickets/nudge", func(s *Server, w http.ResponseWriter, r *http.Request) { s.queueWrite(s.ticketNudge)(w, r) }},
	{"POST /v1/tickets/approve", func(s *Server, w http.ResponseWriter, r *http.Request) { s.queueWrite(s.ticketApprove)(w, r) }},
	{"POST /v1/tickets/relaunch", func(s *Server, w http.ResponseWriter, r *http.Request) { s.queueWrite(s.ticketRelaunch)(w, r) }},
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
		case string(schema.StatusNeedsAnswer), string(schema.StatusNeedsRepair):
			snap.Tickets[i].Recovery = recoveryState(s.parkHold.state(t.Address), t.Address, snap.Tickets)
		}
	}
	snap.Budget = s.budgetStatus(time.Now())
	snap.ExtraUsage = s.extraUsageOn()
	snap.Pending = s.pendingRows()
	_ = json.NewEncoder(w).Encode(snap)
}

// streamEvents is SSE: every event with seq > ?since, then live events until
// the client leaves or is dropped for lagging. 410 means re-snapshot.
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
	flusher, _ := w.(http.Flusher)
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	if flusher != nil {
		flusher.Flush()
	}
	for {
		select {
		case <-r.Context().Done():
			return
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
	_ = json.NewEncoder(w).Encode(Handshake{APIVersion: APIVersion, Build: s.cfg.Build, Pid: os.Getpid(), TCPAddr: s.TCPAddr(), HerdrUnavailable: s.herdr.isUnavailable()})
}

// DefaultLandStopTimeout is how long a stop waits for a land in flight.
const DefaultLandStopTimeout = 30 * time.Second

// Serve blocks until ctx is cancelled, then shuts down and releases the lock.
func (s *Server) Serve(ctx context.Context) error {
	s.log.Info("server started", "pid", os.Getpid(), "build", s.cfg.Build, "socket", SocketPath(s.cfg.StateDir))
	s.seedBudgetNotes(time.Now())
	s.chat.Notice(ralphloop.ServerNotice{Kind: NoticeServerStarted, Emoji: "🚀", Title: "server started", Detail: "build " + s.cfg.Build})
	s.checkExtraUsage(time.Now())
	errc := make(chan error, 2)
	lns := []net.Listener{s.ln}
	if s.tcp != nil {
		lns = append(lns, s.tcp)
	}
	for _, ln := range lns {
		go func() { errc <- s.http.Serve(ln) }()
	}
	s.recoverLands()
	s.reclaimRuns()
	s.scanParentDefects()
	stopCommits := func() {}
	if stop, err := s.startStoreCommits(); err != nil {
		s.log.Error("store commit loop failed to start", "err", err)
	} else {
		stopCommits = stop
	}
	freshCtx, stopFresh := context.WithCancel(ctx)
	freshDone := make(chan struct{})
	go func() { defer close(freshDone); s.keepFresh(freshCtx) }()
	herdrDone := make(chan struct{})
	go func() { defer close(herdrDone); s.keepHerdrChecked(freshCtx) }()
	claimDone := make(chan struct{})
	go func() { defer close(claimDone); s.keepClaiming(freshCtx) }()
	budgetDone := make(chan struct{})
	go func() { defer close(budgetDone); s.keepBudgetPolled(freshCtx) }()
	gatesDone := make(chan struct{})
	go func() { defer close(gatesDone); s.keepGatesWatched(freshCtx) }()
	var err error
	select {
	case <-ctx.Done():
		s.events.close() // open streams would otherwise hold Shutdown
		err = s.http.Shutdown(context.Background())
		for range lns {
			<-errc
		}
	case err = <-errc:
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		_ = s.http.Shutdown(context.Background())
	}
	// Stop claiming, let a land in flight finish, then flush the store commit
	// loop, and only then release the lock. Agents in their panes are not touched.
	stopFresh()
	<-freshDone
	<-herdrDone
	<-claimDone
	<-budgetDone
	<-gatesDone
	timeout := s.cfg.LandStopTimeout
	if timeout <= 0 {
		timeout = DefaultLandStopTimeout
	}
	if !s.lands.closeAndWait(timeout) {
		s.log.Warn("stopping with a land still in flight", "timeout", timeout)
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

// startStoreCommits makes the server the store's committer and commits edits
// made while it was down.
func (s *Server) startStoreCommits() (stop func(), err error) {
	if s.cfg.TicketStore == "" {
		return func() {}, nil
	}
	debounce := s.cfg.StoreCommitDebounce
	if debounce <= 0 {
		debounce = time.Duration(config.DefaultCommitDebounceSeconds) * time.Second
	}
	loop, err := storecommit.StartWith(s.cfg.TicketStore, storecommit.Options{
		Debounce:   debounce,
		PushRemote: s.cfg.StorePushRemote,
		OnError:    func(err error) { s.log.Warn("store push failed", "err", err) },
	})
	if err != nil {
		return nil, err
	}
	if err := loop.Flush(); err != nil {
		s.log.Warn("committing down-time store edits failed", "err", err)
	}
	return loop.Stop, nil
}
