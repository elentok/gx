package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"sync"

	"github.com/elentok/gx/config"
)

const pauseFileName = "queue-mode.json"

// Queue modes, as reported in QueueResult.Mode.
const (
	ModeRunning  = "running"
	ModePaused   = "paused"
	ModeDraining = "draining"
)

// pauseState gates claiming. A pause is persisted so a restart does not start
// work the operator held back; a drain is not, because after a restart there is
// no live run left to wait for.
type pauseState struct {
	mu       sync.Mutex
	path     string
	paused   bool
	draining bool
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

// blocked reports whether claiming a new root is held back.
func (p *pauseState) blocked() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.paused || p.draining
}

func (p *pauseState) mode() string {
	switch {
	case p.paused:
		return ModePaused
	case p.draining:
		return ModeDraining
	}
	return ModeRunning
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
func (s *Server) setMode(paused, draining bool) (QueueResult, error) {
	if s.cfg.Orchestrator != config.OrchestratorServer {
		return refusal(ReasonSchedulerNotSelected, `orchestrator is not "server"`), nil
	}
	p := s.pause
	p.mu.Lock()
	defer p.mu.Unlock()
	prevPaused, prevDraining := p.paused, p.draining
	p.paused, p.draining = paused, draining
	if err := p.save(); err != nil {
		p.paused, p.draining = prevPaused, prevDraining
		return QueueResult{}, err
	}
	s.events.publish(EventQueueChanged, "")
	s.kickRunner()
	return QueueResult{Queue: s.queued.list(), Mode: p.mode()}, nil
}

func (s *Server) queuePause() (QueueResult, error)  { return s.setMode(true, false) }
func (s *Server) queueResume() (QueueResult, error) { return s.setMode(false, false) }
func (s *Server) queueDrain() (QueueResult, error) {
	// Draining a paused queue keeps the pause: it is the stronger, persisted hold.
	return s.setMode(s.pause.isPaused(), true)
}

func (p *pauseState) isPaused() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.paused
}

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
