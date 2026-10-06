// Package server is the orchestrator daemon: HTTP/JSON over a unix socket in
// the state dir, routes under /v1/. Its writes are the server-wide queue in the
// state dir and, when the orchestrator switch says "server", the claim of a
// queued root's frontier ticket.
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
	"time"
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
}

type Config struct {
	StateDir    string
	Build       string
	TicketStore string // ticket-store root; kept fresh by watch + poll

	PollInterval time.Duration // zero means the default
	DisableWatch bool          // poll only

	HerdrRetryInterval time.Duration // zero means the default

	SubscriberBuffer int // events a stream may lag behind before it is dropped; zero means the default

	// Orchestrator is config.Orchestrator. Queue writes are refused unless it
	// is "server": the in-process loop owns claiming otherwise.
	Orchestrator string

	// TCPAddr, when set, adds a loopback-only TCP listener serving the same
	// handler as the socket. No auth: loopback is the only protection.
	TCPAddr string
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

	queued  *queueStore
	events  *broker
	herdr   herdrWatch
	rewatch func() // set by keepFresh when the watch is active

	registry runRegistry
	kick     chan struct{} // wakes keepClaiming
}

// New prepares the state dir, takes the server lock and binds the socket.
// It does not serve until Serve is called.
func New(cfg Config) (*Server, error) {
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
	s := &Server{
		cfg:  cfg,
		lock: lock,
		ln:   ln,
		tcp:  tcp,
		logf: logf,
		log:  slog.New(slog.NewJSONHandler(logf, &slog.HandlerOptions{Level: slog.LevelInfo})),
		idx:  idx,

		queued: queued,
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
	{"GET /v1/locks", (*Server).locks},
	{"GET /v1/tickets/history", (*Server).history},
	{"GET /v1/tickets/explain", (*Server).explain},
	{"GET /v1/iterations", (*Server).iterations},
	{"GET /v1/queue", (*Server).queue},
	{"GET /v1/queue/items", (*Server).queueItems},
	{"POST /v1/queue/add", func(s *Server, w http.ResponseWriter, r *http.Request) { s.queueWrite(s.queueAdd)(w, r) }},
	{"POST /v1/queue/remove", func(s *Server, w http.ResponseWriter, r *http.Request) { s.queueWrite(s.queueRemove)(w, r) }},
	{"POST /v1/queue/move", func(s *Server, w http.ResponseWriter, r *http.Request) { s.queueWrite(s.queueMove)(w, r) }},
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
	_ = json.NewEncoder(w).Encode(Handshake{APIVersion: APIVersion, Build: s.cfg.Build, Pid: os.Getpid(), TCPAddr: s.TCPAddr()})
}

// Serve blocks until ctx is cancelled, then shuts down and releases the lock.
func (s *Server) Serve(ctx context.Context) error {
	s.log.Info("server started", "pid", os.Getpid(), "build", s.cfg.Build, "socket", SocketPath(s.cfg.StateDir))
	errc := make(chan error, 2)
	lns := []net.Listener{s.ln}
	if s.tcp != nil {
		lns = append(lns, s.tcp)
	}
	for _, ln := range lns {
		go func() { errc <- s.http.Serve(ln) }()
	}
	freshCtx, stopFresh := context.WithCancel(ctx)
	freshDone := make(chan struct{})
	go func() { defer close(freshDone); s.keepFresh(freshCtx) }()
	herdrDone := make(chan struct{})
	go func() { defer close(herdrDone); s.keepHerdrChecked(freshCtx) }()
	claimDone := make(chan struct{})
	go func() { defer close(claimDone); s.keepClaiming(freshCtx) }()
	defer func() { stopFresh(); <-freshDone; <-herdrDone; <-claimDone }()
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
	_ = os.Remove(SocketPath(s.cfg.StateDir))
	s.log.Info("server stopped")
	_ = s.logf.Close()
	s.lock.release()
	if err != nil {
		return fmt.Errorf("serve: %w", err)
	}
	return nil
}
