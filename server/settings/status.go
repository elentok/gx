package settings

import (
	"slices"
	"sync"

	"github.com/elentok/gx/server/wire"
)

// history is the last reload result and the startup-only keys seen changed. A
// pending key clears only on restart, so it is never removed.
type history struct {
	mu      sync.Mutex
	last    *wire.ReloadRecord
	pending []string
}

func (h *history) setLast(rec wire.ReloadRecord) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.last = &rec
}

func (h *history) addPending(keys []string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, key := range keys {
		if !slices.Contains(h.pending, key) {
			h.pending = append(h.pending, key)
		}
	}
}

// Status is the config status: the last reload, the keys that need a restart
// and the grace countdown of a reload-lowered hard limit.
func (h *Holder) Status() wire.ConfigStatus {
	h.history.mu.Lock()
	st := wire.ConfigStatus{LastReload: h.history.last, PendingRestart: slices.Clone(h.history.pending)}
	h.history.mu.Unlock()
	if h.opts.HardStopAt != nil {
		st.HardStopAt = h.opts.HardStopAt()
	}
	return st
}
