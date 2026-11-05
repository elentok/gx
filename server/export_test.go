package server

import (
	"io/fs"
	"net/http"
	"time"

	"github.com/elentok/gx/server/wire"
)

// ServeAsset is serveAsset over a fake build, so a test can name hashed assets
// the placeholder embed does not have.
func ServeAsset(assets fs.FS) http.Handler { return serveAsset(assets) }

// LiveDesignSessionID is the ID of epicAddr's newest live design session
// leaving out excludeID, or "".
func (s *Server) LiveDesignSessionID(epicAddr, excludeID string) string {
	live, err := s.liveDesignSessionExcept(epicAddr, excludeID)
	if err != nil || live == nil {
		return ""
	}
	return live.ID
}

// RebaseDesign is rebaseDesign: the skip note, or "" when it rebased.
func (s *Server) RebaseDesign(project, epic, worktree, excludeID string) string {
	return s.rebaseDesign(project, epic, worktree, excludeID)
}

// ResumeAsWeb is the resume the web mux serves: it needs a state token.
func (s *Server) ResumeAsWeb(req wire.RepairRequest) (wire.RepairResult, error) {
	return s.repairResume(req, repairCtx{RequireState: true, origin: wire.OriginWeb})
}

// StartOverAsWeb is the start-over the web mux serves.
func (s *Server) StartOverAsWeb(req wire.RepairRequest) (wire.RepairResult, error) {
	return s.repairStartOver(req, repairCtx{RequireState: true, origin: wire.OriginWeb})
}

// ResumeAs is the resume a client of the given origin sends over the socket.
func (s *Server) ResumeAs(req wire.RepairRequest, origin string) (wire.RepairResult, error) {
	return s.repairResume(req, repairCtx{origin: origin})
}

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
