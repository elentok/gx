// Package server is the orchestrator daemon: HTTP/JSON over a unix socket in
// the state dir, routes under /v1/. In this stage it is read-only — it never
// claims or writes tickets, so it can run beside the in-process loop.
package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
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
}

type Config struct {
	StateDir string
	Build    string
}

// Server owns the state-dir lock and the listening socket.
type Server struct {
	cfg  Config
	lock *serverLock
	ln   net.Listener
	http *http.Server
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
	s := &Server{cfg: cfg, lock: lock, ln: ln}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/handshake", s.handshake)
	s.http = &http.Server{Handler: mux}
	return s, nil
}

func (s *Server) handshake(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(Handshake{APIVersion: APIVersion, Build: s.cfg.Build})
}

// Serve blocks until ctx is cancelled, then shuts down and releases the lock.
func (s *Server) Serve(ctx context.Context) error {
	errc := make(chan error, 1)
	go func() { errc <- s.http.Serve(s.ln) }()
	var err error
	select {
	case <-ctx.Done():
		err = s.http.Shutdown(context.Background())
		<-errc
	case err = <-errc:
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
	}
	_ = os.Remove(SocketPath(s.cfg.StateDir))
	s.lock.release()
	if err != nil {
		return fmt.Errorf("serve: %w", err)
	}
	return nil
}
