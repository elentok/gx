package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"sync"
)

const pauseFileName = "queue-mode.json"

// QueueMode is the queue's mode, as reported in QueueResult.Mode.
type QueueMode string

const (
	QueueModeRunning QueueMode = "running"
	QueueModePaused  QueueMode = "paused"
)

// pauseState gates claiming. A pause is persisted so a restart does not start
// work the operator held back. Pausing never touches runs already started, so
// it is also "Finish and pause".
type pauseState struct {
	mu     sync.Mutex
	path   string
	paused bool
}

func openPause(stateDir string) (*pauseState, error) {
	p := &pauseState{path: filepath.Join(stateDir, pauseFileName)}
	data, err := os.ReadFile(p.path)
	if errors.Is(err, os.ErrNotExist) {
		return p, nil
	}
	if err != nil {
		return nil, err
	}
	var f struct {
		Paused bool `json:"paused"`
	}
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, err
	}
	p.paused = f.Paused
	return p, nil
}

// QueuePaused reports whether the pause persisted in stateDir is on. It reads
// the same file the server writes, so it answers for a server that is down or
// an older build that does not report its mode.
func QueuePaused(stateDir string) (bool, error) {
	p, err := openPause(stateDir)
	if err != nil {
		return false, err
	}
	return p.paused, nil
}

// blocked reports whether claiming a new root is held back.
func (p *pauseState) blocked() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.paused
}

// currentMode is mode for callers that do not already hold the lock.
func (p *pauseState) currentMode() QueueMode {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.mode()
}

func (p *pauseState) mode() QueueMode {
	if p.paused {
		return QueueModePaused
	}
	return QueueModeRunning
}

// save writes through a rename so a crash never leaves a torn file.
func (p *pauseState) save() error {
	data, err := json.Marshal(struct {
		Paused bool `json:"paused"`
	}{p.paused})
	if err != nil {
		return err
	}
	tmp := p.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, p.path)
}

// setMode applies a mode change, persists the pause flag, then publishes.
func (s *Server) setMode(paused bool) (QueueResult, error) {
	p := s.pause
	p.mu.Lock()
	defer p.mu.Unlock()
	prev := p.paused
	p.paused = paused
	if err := p.save(); err != nil {
		p.paused = prev
		return QueueResult{}, err
	}
	s.events.publish(EventQueueChanged, "")
	s.kickRunner()
	return QueueResult{Queue: s.queued.list(), Mode: p.mode()}, nil
}

func (s *Server) queuePause() (QueueResult, error)  { return s.setMode(true) }
func (s *Server) queueResume() (QueueResult, error) { return s.setMode(false) }

// queueDrain is the old name of Finish and pause, kept for one release.
func (s *Server) queueDrain() (QueueResult, error) { return s.queuePause() }

// modeWrite adapts a mode change to a POST handler.
func (s *Server) modeWrite(do func() (QueueResult, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		res, err := do()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, res)
	}
}
