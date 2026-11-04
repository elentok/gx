package server

import (
	"errors"
	"io/fs"
	"net/http"
	"path/filepath"
	"time"

	"github.com/elentok/gx/server/queue"
	"github.com/elentok/gx/server/runs"

	"github.com/elentok/gx/events"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/recovery"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
)

var (
	RecoveryState = recoveryState
	GuardRailStop = guardRailStop
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

// ClaimTicket claims and launches one ticket the way a claim pass would, but
// without asking the frontier, and reports whether an iteration launched.
func (s *Server) ClaimTicket(address, agent string) (bool, error) {
	addr, err := tickets.ParseAddress(address, tickets.AddressContext{})
	if err != nil {
		return false, err
	}
	ref, ok, err := s.findTicket(address)
	if err != nil || !ok {
		return false, errors.Join(err, errors.New("no ticket "+address))
	}
	epics, err := tickets.Load(ref.projectDir)
	if err != nil {
		return false, err
	}
	return s.claimAndLaunch(queue.RootOf(addr), addr, ref.ticket, ref.repo, ralphloop.AgentKind(agent), epics)
}

// SetMergeInto swaps the merge core a root merge calls.
func (s *Server) SetMergeInto(f func(git.Repo, []git.Worktree, string, string) (git.MergeOutcome, error)) {
	s.mergeInto = f
}

// SetMergedProbe swaps the poll-time ancestry check.
func (s *Server) SetMergedProbe(f func(repo, ancestor, descendant string) (bool, error)) {
	s.mergedProbe = f
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

// AgentWaiting reports whether the run's agent is not working (idle or done).
func (s *Server) AgentWaiting(project, epic, id string) bool {
	_, idle, err := s.agentState(tickets.Address{Project: project, Epic: epic, ID: id})
	return err == nil && idle
}

// PauseTicket pauses a ticket the way a failed iteration does: the write, the
// event, then the chat message (or its hold while recovery runs).
func (s *Server) PauseTicket(project, epic, id string, kind events.Kind, reason string) error {
	dir := filepath.Join(s.cfg.TicketStore, project)
	path := filepath.Join(dir, epic, "issues", id+"-first.md")
	return s.pauseTicket(dir, tickets.Address{Project: project, Epic: epic, ID: id}, path, kind, reason, runs.Run{})
}

// PruneBackups runs the backup-ref prune as of now.
func (s *Server) PruneBackups(repoRoot string, now time.Time) { s.pruneBackups(repoRoot, now) }

// SetShutdownRecorder makes Serve report each shutdown step to rec; nil clears it.
func SetShutdownRecorder(rec func(step string)) {
	if rec == nil {
		shutdownRecorder.Store(nil)
		return
	}
	shutdownRecorder.Store(&rec)
}

// SettleReleasedRun stands in for the finish of a run whose gate was released:
// the pause release-gate left, if any, then the run is dropped.
func (s *Server) SettleReleasedRun(address string) {
	addr, _ := tickets.ParseAddress(address, tickets.AddressContext{})
	dir := filepath.Join(s.cfg.TicketStore, addr.Project)
	if run, ok := s.registry.RunOf(address); ok {
		s.pauseReleasedGate(dir, addr, filepath.Join(dir, addr.Epic, "issues", addr.ID+"-first.md"), run)
	}
	s.registry.Delete(address)
}

// ClaimNext runs one claim pass now.
func (s *Server) ClaimNext() { s.claimNext() }

// RecoverAsync is recoverAsync for a failure the test raises itself.
func (s *Server) RecoverAsync(f recovery.Failure) { s.recoverAsync(f) }

// RecoveryDo calls one server verb the way a remedy does.
func (s *Server) RecoveryDo(c recovery.Call) (recovery.Result, error) { return recoveryVerbs{s}.Do(c) }

// WatchGates runs one gate watchdog pass as of now.
func (s *Server) WatchGates(now time.Time) { s.watchGates(now) }

// PauseStuckFinishedClaims runs one finished-claim watchdog pass as of now.
func (s *Server) PauseStuckFinishedClaims(now time.Time) { s.pauseStuckFinishedClaims(now) }

// RecoveredCount is recoveredCount for tests.
func RecoveredCount(projectDir, epic string) int { return recoveredCount(projectDir, epic) }

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
	s.completeRootIfDone(queue.RootOf(addr), ralphloop.OneIteration{Epic: addr.Epic, Ticket: t})
	return nil
}

// RemoveMergedWorktree starts the post-merge worktree removal the way stampEpic
// does, on the settle group.
func (s *Server) RemoveMergedWorktree(project, epic string) {
	root := queue.RootRef{Project: project, Epic: epic}
	s.guards.GoSettle(func() { s.removeMergedWorktree(root) })
}

// PutRunIn is Registry.PutRunIn on the configured runner.
func (s *Server) PutRunIn(address, root, cwd string, launching bool) {
	s.registry.PutRunIn(address, root, cwd, s.cfg.Runner.Name(), launching)
}

// LockedTrunks lists the trunks any merge has locked so far, in any repo.
func (s *Server) LockedTrunks() []string {
	return s.guards.Trunks.Locked()
}
