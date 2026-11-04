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

const viModeFileName = "vimode.json"

// viModeState is the web UI's vi mode, persisted so a restart keeps it.
type viModeState struct {
	mu   sync.Mutex
	path string
	on   bool
}

func openViMode(stateDir string) (*viModeState, error) {
	v := &viModeState{path: filepath.Join(stateDir, viModeFileName)}
	data, err := os.ReadFile(v.path)
	if errors.Is(err, os.ErrNotExist) {
		return v, nil
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
	v.on = f.On
	return v, nil
}

func (v *viModeState) isOn() bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.on
}

// save writes through a rename so a crash never leaves a torn file.
func (v *viModeState) save() error {
	data, err := json.Marshal(struct {
		On bool `json:"on"`
	}{v.on})
	if err != nil {
		return err
	}
	tmp := v.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, v.path)
}

func (s *Server) setViMode(req wire.ViModeRequest) (wire.ViModeResult, error) {
	v := s.viMode
	v.mu.Lock()
	defer v.mu.Unlock()
	prev := v.on
	v.on = !v.on
	if req.On != nil {
		v.on = *req.On
	}
	if err := v.save(); err != nil {
		v.on = prev
		return wire.ViModeResult{}, err
	}
	s.events.publish(wire.EventViModeChanged, "")
	return wire.ViModeResult{ViMode: v.on}, nil
}

func (s *Server) viModeWrite(w http.ResponseWriter, r *http.Request) {
	var req wire.ViModeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	res, err := s.setViMode(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, res)
}
