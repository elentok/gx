package server

import (
	"net/http"
	"slices"
	"sync"

	"github.com/elentok/gx/server/wire"
)

// reloadHistory is the last reload result and the startup-only keys seen
// changed. A pending key clears only on restart, so it is never removed.
type reloadHistory struct {
	mu      sync.Mutex
	last    *wire.ReloadRecord
	pending []string
}

func (h *reloadHistory) setLast(rec wire.ReloadRecord) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.last = &rec
}

func (h *reloadHistory) addPending(keys []string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, key := range keys {
		if !slices.Contains(h.pending, key) {
			h.pending = append(h.pending, key)
		}
	}
}

func (s *Server) configStatus() wire.ConfigStatus {
	s.reloads.mu.Lock()
	st := wire.ConfigStatus{LastReload: s.reloads.last, PendingRestart: slices.Clone(s.reloads.pending)}
	s.reloads.mu.Unlock()
	if s.ledger.held() {
		st.HardStopAt = s.hardGrace.deadline()
	}
	return st
}

func (s *Server) configStatusRead(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, s.configStatus())
}
