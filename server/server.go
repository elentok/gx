// Package server is the orchestrator daemon: HTTP/JSON over a unix socket in
// the state dir, routes under /v1/. In this stage it is read-only — it never
// claims or writes tickets, so it can run beside the in-process loop.
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
}

type Config struct {
	StateDir    string
	Build       string
	TicketStore string // ticket-store root; kept fresh by watch + poll

	PollInterval time.Duration // zero means the default
	DisableWatch bool          // poll only

	SubscriberBuffer int // events a stream may lag behind before it is dropped; zero means the default
}

// Server owns the state-dir lock and the listening socket.
type Server struct {
	cfg  Config
	lock *serverLock
	ln   net.Listener
	http *http.Server
	logf *rotatingFile
	log  *slog.Logger
	idx  *index

	events *broker
	rewatch func() // set by keepFresh when the watch is active
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
	logf, err := openRotatingFile(LogPath(cfg.StateDir), logMaxBytes, logMaxFiles)
	if err != nil {
		ln.Close()
		lock.release()
		return nil, err
	}
	s := &Server{
		cfg:  cfg,
		lock: lock,
		ln:   ln,
		logf: logf,
		log:  slog.New(slog.NewJSONHandler(logf, &slog.HandlerOptions{Level: slog.LevelInfo})),
		idx:  idx,

		events: events,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/handshake", s.handshake)
	mux.HandleFunc("GET /v1/snapshot", s.snapshot)
	mux.HandleFunc("GET /v1/events", s.streamEvents)
	s.http = &http.Server{Handler: mux}
	return s, nil
}

func (s *Server) snapshot(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(s.idx.snapshot())
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
	_ = json.NewEncoder(w).Encode(Handshake{APIVersion: APIVersion, Build: s.cfg.Build, Pid: os.Getpid()})
}

// Serve blocks until ctx is cancelled, then shuts down and releases the lock.
func (s *Server) Serve(ctx context.Context) error {
	s.log.Info("server started", "pid", os.Getpid(), "build", s.cfg.Build, "socket", SocketPath(s.cfg.StateDir))
	errc := make(chan error, 1)
	go func() { errc <- s.http.Serve(s.ln) }()
	freshCtx, stopFresh := context.WithCancel(ctx)
	freshDone := make(chan struct{})
	go func() { defer close(freshDone); s.keepFresh(freshCtx) }()
	defer func() { stopFresh(); <-freshDone }()
	var err error
	select {
	case <-ctx.Done():
		s.events.close() // open streams would otherwise hold Shutdown
		err = s.http.Shutdown(context.Background())
		<-errc
	case err = <-errc:
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
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
