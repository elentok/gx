package queue

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"sync"

	"github.com/elentok/gx/server/broker"
	"github.com/elentok/gx/server/wire"
)

const afkFileName = "afk.json"

// AFK is AFK mode, persisted so a restart keeps sending to chat.
type AFK struct {
	mu     sync.Mutex
	path   string
	on     bool
	events *broker.Broker
}

// OpenAFK loads the persisted AFK mode from stateDir.
func OpenAFK(stateDir string, events *broker.Broker) (*AFK, error) {
	a := &AFK{path: filepath.Join(stateDir, afkFileName), events: events}
	data, err := os.ReadFile(a.path)
	if errors.Is(err, os.ErrNotExist) {
		return a, nil
	}
	if err != nil {
		return nil, err
	}
	var f struct {
		On bool `json:"on"`
	}
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, err
	}
	a.on = f.On
	return a, nil
}

// IsOn reports whether AFK mode is on.
func (a *AFK) IsOn() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.on
}

// save writes through a rename so a crash never leaves a torn file.
func (a *AFK) save() error {
	data, err := json.Marshal(struct {
		On bool `json:"on"`
	}{a.on})
	if err != nil {
		return err
	}
	tmp := a.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, a.path)
}

// Set turns AFK mode on, off, or (with no value) flips it.
func (a *AFK) Set(req wire.AFKRequest) (wire.AFKResult, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	prev := a.on
	a.on = !a.on
	if req.On != nil {
		a.on = *req.On
	}
	if err := a.save(); err != nil {
		a.on = prev
		return wire.AFKResult{}, err
	}
	a.events.Publish(wire.EventAFKChanged, "")
	return wire.AFKResult{AFK: a.on}, nil
}

func (a *AFK) write(w http.ResponseWriter, r *http.Request) {
	var req wire.AFKRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	res, err := a.Set(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	wire.WriteJSON(w, res)
}
