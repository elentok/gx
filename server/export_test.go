package server

import (
	"errors"
	"io/fs"
	"net/http"
	"time"

	"github.com/elentok/gx/server/queue"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
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

// SetMergeInto swaps the merge core a root merge calls.
func (s *Server) SetMergeInto(f func(git.Repo, []git.Worktree, string, string) (git.MergeOutcome, error)) {
	s.merger.mergeInto = f
}

// SetMergedProbe swaps the poll-time ancestry check.
func (s *Server) SetMergedProbe(f func(repo, ancestor, descendant string) (bool, error)) {
	s.rec.mergedProbe = f
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

// PruneBackups runs the backup-ref prune as of now.
func (s *Server) PruneBackups(repoRoot string, now time.Time) { s.landing.pruneBackups(repoRoot, now) }

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

// HoldTrunkLock takes the project's trunk lock the way a merge does, until
// release runs.
func (s *Server) HoldTrunkLock(project, trunk string) (release func(), err error) {
	_, repo, err := s.projects.Of(project)
	if err != nil {
		return nil, err
	}
	return s.guards.Trunks.Lock(repo, trunk), nil
}

// CompleteRootIfDone runs the all-done check of address's root as if its
// iteration had just settled.
func (s *Server) CompleteRootIfDone(address string) error {
	addr, err := tickets.ParseAddress(address, tickets.AddressContext{})
	if err != nil {
		return err
	}
	_, t, ok := s.FreshTicket(addr)
	if !ok {
		return errors.New("no ticket " + address)
	}
	s.merger.completeRootIfDone(queue.RootOf(addr), ralphloop.OneIteration{Epic: addr.Epic, Ticket: t})
	return nil
}

// RemoveMergedWorktree starts the post-merge worktree removal the way stampEpic
// does, on the settle group.
func (s *Server) RemoveMergedWorktree(project, epic string) {
	root := queue.RootRef{Project: project, Epic: epic}
	s.guards.GoSettle(func() { s.rec.removeMergedWorktree(root) })
}

// PutRunIn is Registry.PutRunIn on the configured runner.
func (s *Server) PutRunIn(address, root, cwd string, launching bool) {
	s.registry.PutRunIn(address, root, cwd, s.cfg.Runner.Name(), launching)
}

// LockedTrunks lists the trunks any merge has locked so far, in any repo.
func (s *Server) LockedTrunks() []string {
	return s.guards.Trunks.Locked()
}
