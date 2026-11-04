package queue

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"sync"

	"github.com/elentok/gx/server/wire"
)

const pauseFileName = "queue-mode.json"

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

// Blocked reports whether claiming a new root is held back.
func (q *Queue) Blocked() bool {
	q.pause.mu.Lock()
	defer q.pause.mu.Unlock()
	return q.pause.paused
}

// Mode is the queue mode for callers that do not already hold the pause lock.
func (q *Queue) Mode() wire.QueueMode {
	q.pause.mu.Lock()
	defer q.pause.mu.Unlock()
	return q.pause.mode()
}

func (p *pauseState) mode() wire.QueueMode {
	if p.paused {
		return wire.QueueModePaused
	}
	return wire.QueueModeRunning
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
func (q *Queue) setMode(paused bool) (wire.QueueResult, error) {
	p := q.pause
	p.mu.Lock()
	defer p.mu.Unlock()
	prev := p.paused
	p.paused = paused
	if err := p.save(); err != nil {
		p.paused = prev
		return wire.QueueResult{}, err
	}
	q.events.Publish(wire.EventQueueChanged, "")
	q.guards.Kick()
	return wire.QueueResult{Queue: q.List(), Mode: p.mode()}, nil
}

// Pause holds back new claims. Runs already started are untouched.
func (q *Queue) Pause() (wire.QueueResult, error) { return q.setMode(true) }

// Resume lifts the pause.
func (q *Queue) Resume() (wire.QueueResult, error) { return q.setMode(false) }

// Flush is the old name of Finish and pause, kept for one release.
func (q *Queue) Flush() (wire.QueueResult, error) { return q.Pause() }

// modeWrite adapts a mode change to a POST handler.
func modeWrite(do func() (wire.QueueResult, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		res, err := do()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		wire.WriteJSON(w, res)
	}
}
