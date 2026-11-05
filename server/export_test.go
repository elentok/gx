package server

import (
	"io/fs"
	"net/http"
	"time"
)

// ServeAsset is serveAsset over a fake build, so a test can name hashed assets
// the placeholder embed does not have.
func ServeAsset(assets fs.FS) http.Handler { return serveAsset(assets) }

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
