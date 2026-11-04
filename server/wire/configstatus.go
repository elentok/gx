package wire

import "time"

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
