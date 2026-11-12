// Package devvite owns the Vite child process behind Dev mode: it installs
// the Dev checkout's web dependencies when they are stale, starts Vite on a
// free loopback port, reports running only once Vite answers a request, and
// stops it again. It imports only the standard library.
package devvite

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// State is where the supervisor is in the turn-on order.
type State string

const (
	StateOff        State = "off"
	StateInstalling State = "installing"
	StateStarting   State = "starting"
	StateRunning    State = "running"
	StateDown       State = "down"
)

// ErrPnpmNotFound is what a Launcher returns when pnpm is not installed.
var ErrPnpmNotFound = errors.New("pnpm not found")

// ErrNotDown is returned by Restart when the supervisor is not down.
var ErrNotDown = errors.New("devvite: restart is only valid while down")

// Process is a started Vite.
type Process interface {
	// Wait blocks until the process has exited.
	Wait() error
	// Stop asks the process (and its children) to end; Wait then returns.
	Stop()
}

// Launcher is how processes are spawned and readiness is probed.
type Launcher interface {
	// FreePort returns a loopback port nothing is listening on.
	FreePort() (int, error)
	// Install runs the dependency install in webuiDir, writing output to out.
	Install(ctx context.Context, webuiDir string, out io.Writer) error
	// Start starts Vite in webuiDir on the port, writing output to out.
	Start(ctx context.Context, webuiDir string, port int, out io.Writer) (Process, error)
	// Probe returns nil once Vite answers an HTTP request on the port.
	Probe(ctx context.Context, port int) error
}

// Snapshot is a copy of the supervisor's observable state.
type Snapshot struct {
	State  State
	Reason string
	Root   string
	Port   int
	Logs   []string
}

// Config configures a Supervisor.
type Config struct {
	// LogDir receives the log file. Required.
	LogDir string
	// Env is the child's environment, used by the default launcher.
	Env []string
	// Launcher defaults to the pnpm launcher.
	Launcher Launcher
	// Observer is called on each state or reason change, never for log lines.
	Observer func(Snapshot)
	// TailLines bounds the last-lines buffer (default 200).
	TailLines int
	// ReadyTimeout bounds the wait for Vite to answer (default 60s).
	ReadyTimeout time.Duration
	// ProbeInterval is the gap between failed probes (default 200ms).
	ProbeInterval time.Duration
}

// Supervisor runs Vite for a desired Dev checkout.
type Supervisor struct {
	cfg  Config
	l    Launcher
	logs *logSink

	mu      sync.Mutex
	cond    *sync.Cond
	on      bool
	root    string
	gen     uint64
	closed  bool
	cancel  context.CancelFunc
	snap    Snapshot
	stopped chan struct{}
}

// New starts the supervisor goroutine, off until Set turns it on.
func New(cfg Config) (*Supervisor, error) {
	if cfg.TailLines <= 0 {
		cfg.TailLines = 200
	}
	if cfg.ReadyTimeout <= 0 {
		cfg.ReadyTimeout = 60 * time.Second
	}
	if cfg.ProbeInterval <= 0 {
		cfg.ProbeInterval = 200 * time.Millisecond
	}
	if cfg.Launcher == nil {
		cfg.Launcher = NewPnpmLauncher(cfg.Env)
	}
	sink, err := openLogSink(cfg.LogDir, cfg.TailLines)
	if err != nil {
		return nil, err
	}
	s := &Supervisor{cfg: cfg, l: cfg.Launcher, logs: sink, stopped: make(chan struct{})}
	s.snap.State = StateOff
	s.cond = sync.NewCond(&s.mu)
	go s.loop()
	return s, nil
}

// Set states the desired state and returns at once. A changed root restarts
// the turn-on order; an unchanged desire is a no-op.
func (s *Supervisor) Set(on bool, root string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || (s.on == on && (!on || s.root == root)) {
		return
	}
	s.on, s.root = on, root
	s.bump()
}

// Restart reruns the turn-on order; it is only valid while down.
func (s *Supervisor) Restart() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.snap.State != StateDown || s.closed {
		return ErrNotDown
	}
	s.bump()
	return nil
}

func (s *Supervisor) bump() {
	s.gen++
	if s.cancel != nil {
		s.cancel()
	}
	s.cond.Broadcast()
}

// Snapshot returns the current state, reason, checkout, port and log tail.
func (s *Supervisor) Snapshot() Snapshot {
	s.mu.Lock()
	snap := s.snap
	s.mu.Unlock()
	snap.Logs = s.logs.tail()
	return snap
}

// Close stops the child, waits for it and ends the supervisor goroutine.
func (s *Supervisor) Close() error {
	s.mu.Lock()
	if !s.closed {
		s.closed = true
		s.bump()
	}
	s.mu.Unlock()
	<-s.stopped
	return s.logs.close()
}

func (s *Supervisor) loop() {
	defer close(s.stopped)
	var seen uint64
	for {
		s.mu.Lock()
		for s.gen == seen && !s.closed {
			s.cond.Wait()
		}
		if s.closed {
			s.mu.Unlock()
			s.set(StateOff, "", 0)
			return
		}
		seen = s.gen
		on, root := s.on, s.root
		ctx, cancel := context.WithCancel(context.Background())
		s.cancel = cancel
		s.snap.Root = root
		s.mu.Unlock()
		if !on {
			s.set(StateOff, "", 0)
		} else {
			s.turnOn(ctx, root)
		}
		cancel()
	}
}

// set records a state change and tells the observer if state or reason moved.
func (s *Supervisor) set(st State, reason string, port int) {
	s.mu.Lock()
	changed := s.snap.State != st || s.snap.Reason != reason
	s.snap.State, s.snap.Reason, s.snap.Port = st, reason, port
	snap := s.snap
	s.mu.Unlock()
	if changed && s.cfg.Observer != nil {
		snap.Logs = s.logs.tail()
		s.cfg.Observer(snap)
	}
}

func (s *Supervisor) down(format string, args ...any) {
	s.set(StateDown, fmt.Sprintf(format, args...), 0)
}

// turnOn runs install, start, readiness and the watch; it returns when ctx is
// cancelled (after stopping the child) or when it has ended down.
func (s *Supervisor) turnOn(ctx context.Context, root string) {
	webui := filepath.Join(root, "webui")
	if needsInstall(webui) {
		s.set(StateInstalling, "Installing web dependencies", 0)
		err := s.l.Install(ctx, webui, s.logs)
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			s.down("%s", failReason("Installing web dependencies failed", err))
			return
		}
		if _, err := os.Stat(filepath.Join(webui, "node_modules")); err != nil {
			s.down("Installing web dependencies finished but left no node_modules folder")
			return
		}
	}
	port, err := s.l.FreePort()
	if err != nil {
		s.down("Could not find a free port for Vite: %v", err)
		return
	}
	s.set(StateStarting, "Starting Vite", port)
	proc, err := s.l.Start(ctx, webui, port, s.logs)
	if ctx.Err() != nil {
		if err == nil {
			proc.Stop()
			_ = proc.Wait()
		}
		return
	}
	if err != nil {
		s.down("%s", failReason("Starting Vite failed", err))
		return
	}
	exited := make(chan error, 1)
	go func() { exited <- proc.Wait() }()
	stop := func() {
		proc.Stop()
		<-exited
	}
	switch out, exitErr := s.waitReady(ctx, port, exited); out {
	case readyCancelled:
		stop()
		return
	case readyExited:
		s.down("Vite exited before it was ready%s", errSuffix(exitErr))
		return
	case readyTimedOut:
		stop()
		s.down("Vite did not answer within %s", s.cfg.ReadyTimeout)
		return
	}
	s.set(StateRunning, "Vite is running", port)
	select {
	case err := <-exited:
		s.down("Vite exited%s", errSuffix(err))
	case <-ctx.Done():
		stop()
	}
}

type readyOutcome int

const (
	readyOK readyOutcome = iota
	readyCancelled
	readyExited
	readyTimedOut
)

// waitReady polls the probe until it answers, the process exits, ctx ends or
// the bounded wait runs out.
func (s *Supervisor) waitReady(ctx context.Context, port int, exited <-chan error) (readyOutcome, error) {
	deadline := time.NewTimer(s.cfg.ReadyTimeout)
	defer deadline.Stop()
	tick := time.NewTicker(s.cfg.ProbeInterval)
	defer tick.Stop()
	for {
		if s.l.Probe(ctx, port) == nil {
			return readyOK, nil
		}
		select {
		case err := <-exited:
			return readyExited, err
		case <-ctx.Done():
			return readyCancelled, nil
		case <-deadline.C:
			return readyTimedOut, nil
		case <-tick.C:
		}
	}
}

func errSuffix(err error) string {
	if err == nil {
		return ""
	}
	return ": " + err.Error()
}

func failReason(what string, err error) string {
	if errors.Is(err, ErrPnpmNotFound) {
		return what + ": pnpm was not found, install pnpm and try again"
	}
	return what + ": " + err.Error()
}

// needsInstall reports a missing node_modules, or one older than the lock file.
func needsInstall(webui string) bool {
	nm, err := os.Stat(filepath.Join(webui, "node_modules"))
	if err != nil {
		return true
	}
	lock, err := os.Stat(filepath.Join(webui, "pnpm-lock.yaml"))
	if err != nil {
		return false
	}
	return lock.ModTime().After(nm.ModTime())
}
