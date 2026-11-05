package server

import (
	"time"
)

// SetShutdownRecorder makes Serve report each shutdown step to rec; nil clears it.
func SetShutdownRecorder(rec func(step string)) {
	if rec == nil {
		shutdownRecorder.Store(nil)
		return
	}
	shutdownRecorder.Store(&rec)
}

// ClaimNext runs one claim pass now.
func (s *Server) ClaimNext() { s.claimNext() }

// PauseStuckFinishedClaims runs one finished-claim watchdog pass as of now.
func (s *Server) PauseStuckFinishedClaims(now time.Time) { s.pauseStuckFinishedClaims(now) }
