package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"sync"

	"github.com/elentok/gx/server/wire"
)

const afkFileName = "afk.json"

// EventAFKChanged announces that AFK mode was switched.
const EventAFKChanged = "afk-changed"

// afkState is AFK mode, persisted so a restart keeps sending to chat.
type afkState struct {
	mu   sync.Mutex
	path string
	on   bool
}

func openAFK(stateDir string) (*afkState, error) {
	a := &afkState{path: filepath.Join(stateDir, afkFileName)}
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

func (a *afkState) isOn() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.on
}

// save writes through a rename so a crash never leaves a torn file.
func (a *afkState) save() error {
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

func (s *Server) setAFK(req wire.AFKRequest) (wire.AFKResult, error) {
	a := s.afk
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
	s.events.publish(EventAFKChanged, "")
	return wire.AFKResult{AFK: a.on}, nil
}

func (s *Server) afkWrite(w http.ResponseWriter, r *http.Request) {
	var req wire.AFKRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	res, err := s.setAFK(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, res)
}
