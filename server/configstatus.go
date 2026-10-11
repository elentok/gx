package server

import (
	"net/http"
	"slices"
	"sync"
	"time"
)

// ConfigStatus is what `gx server config-status` and the Config block of
// `gx server status` show.
type ConfigStatus struct {
	// LastReload is nil until the first reload since startup.
	LastReload *ReloadRecord `json:"last_reload,omitempty"`
	// PendingRestart lists the startup-only keys a reload saw changed.
	PendingRestart []string `json:"pending_restart,omitempty"`
	// HardStopAt is when a reload-lowered hard limit stops agents; zero when
	// no grace countdown runs.
	HardStopAt time.Time `json:"hard_stop_at,omitzero"`
}

// Empty reports that there is nothing worth a Config block.
func (st ConfigStatus) Empty() bool {
	return st.LastReload == nil && len(st.PendingRestart) == 0 && st.HardStopAt.IsZero()
}

// ReloadRecord is one reload's result as kept for status.
type ReloadRecord struct {
	At      time.Time `json:"at"`
	Trigger string    `json:"trigger"`
	// Outcome is the reload's event level: success, warning or error.
	Outcome string `json:"outcome"`
	Message string `json:"message"`
}

// reloadHistory is the last reload result and the startup-only keys seen
// changed. A pending key clears only on restart, so it is never removed.
type reloadHistory struct {
	mu      sync.Mutex
	last    *ReloadRecord
	pending []string
}

func (h *reloadHistory) setLast(rec ReloadRecord) {
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

func (s *Server) configStatus() ConfigStatus {
	s.reloads.mu.Lock()
	st := ConfigStatus{LastReload: s.reloads.last, PendingRestart: slices.Clone(s.reloads.pending)}
	s.reloads.mu.Unlock()
	if s.ledger.held() {
		st.HardStopAt = s.hardGrace.deadline()
	}
	return st
}

func (s *Server) configStatusRead(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, s.configStatus())
}
