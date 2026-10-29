package server

import (
	"errors"
	"path/filepath"
	"strings"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/config"
	"github.com/elentok/gx/events"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/recovery"
	"github.com/elentok/gx/tickets"
)

var (
	RecoveryState  = recoveryState
	GuardRailStop  = guardRailStop
	HardGraceTitle = hardGraceTitle
	TicketAgent    = ticketAgent
)

// LiveDiskSpace is the disk-space settings the server currently runs on.
func (s *Server) LiveDiskSpace() config.DiskSpaceConfig {
	return s.settings.Load().DiskSpace
}

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

// LaunchingRuns counts runs whose agent has no session yet, which a budget
// stop skips.
func (s *Server) LaunchingRuns() int {
	return len(s.registry.list()) - len(s.killableRuns())
}

// LockReload holds the reload lock until the returned func runs, so a test can
// keep a reload in flight.
func (s *Server) LockReload() (unlock func()) {
	s.reloadMu.Lock()
	return s.reloadMu.Unlock
}

// HardGraceRunning reports whether a hard-limit countdown is running.
func (s *Server) HardGraceRunning() bool { return !s.hardGrace.deadline().IsZero() }

// PutRun records a launched iteration without launching one, so tests can hold
// a run live as long as they need.
func (s *Server) PutRun(root string, r Run) { s.registry.put(trackedRun{Run: r, Root: root}) }

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
	return s.claimAndLaunch(rootOf(addr), addr, ref.ticket, ref.repo, ralphloop.AgentKind(agent), epics)
}

// PauseAs pauses a ticket as the given kind the way the server's own pauses do,
// including starting recovery.
func (s *Server) PauseAs(address string, kind events.Kind, reason string) error {
	addr, err := tickets.ParseAddress(address, tickets.AddressContext{})
	if err != nil {
		return err
	}
	ref, ok, err := s.findTicket(address)
	if err != nil || !ok {
		return errors.Join(err, errors.New("no ticket "+address))
	}
	return s.pauseTicket(ref.projectDir, addr, ref.ticket.Path, kind, reason, Run{})
}

// SetCostOf swaps how an iteration's cost is read.
func (s *Server) SetCostOf(f func(IterationInfo) (float64, bool)) { s.costOf = f }

// SetMergeInto swaps the merge core a root merge calls.
func (s *Server) SetMergeInto(f func(git.Repo, []git.Worktree, string, string) (git.MergeOutcome, error)) {
	s.mergeInto = f
}

// SetMergedProbe swaps the poll-time ancestry check.
func (s *Server) SetMergedProbe(f func(repo, ancestor, descendant string) (bool, error)) {
	s.mergedProbe = f
}

// ResumeAsWeb is the resume the web mux serves: it needs a state token.
func (s *Server) ResumeAsWeb(req RepairRequest) (RepairResult, error) {
	req.RequireState = true
	req.origin = OriginWeb
	return s.repairResume(req)
}

// StartOverAsWeb is the start-over the web mux serves.
func (s *Server) StartOverAsWeb(req RepairRequest) (RepairResult, error) {
	req.RequireState = true
	req.origin = OriginWeb
	return s.repairStartOver(req)
}

// ResumeAs is the resume a client of the given origin sends over the socket.
func (s *Server) ResumeAs(req RepairRequest, origin string) (RepairResult, error) {
	req.origin = origin
	return s.repairResume(req)
}

// Rescan rescans the store now, as a ping or the watch would.
func (s *Server) Rescan() { s.rescan() }

// PollBudget runs one budget poll now.
func (s *Server) PollBudget() { s.pollBudget(time.Now()) }

// BudgetStatusAt is the budget status as of now.
func (s *Server) BudgetStatusAt(now time.Time) BudgetStatus { return s.budgetStatus(now) }

// HardStopAt is the config status's grace countdown deadline.
func (s *Server) HardStopAt() time.Time { return s.configStatus().HardStopAt }

// RecordProjectSpend adds spend attributed to a project to today's ledger.
func (s *Server) RecordProjectSpend(project, key string, cost float64) {
	s.ledger.record(project, key, cost, time.Now())
}

// RecordSpend adds spend to today's ledger.
func (s *Server) RecordSpend(key string, cost float64) { s.ledger.record("", key, cost, time.Now()) }

// PutRunAt is PutRun with the time the iteration was launched.
func (s *Server) PutRunAt(root string, r Run, at time.Time) {
	s.registry.put(trackedRun{Run: r, Root: root, StartedAt: at})
}

// PauseTicket pauses a ticket the way a failed iteration does: the write, the
// event, then the chat message (or its hold while recovery runs).
func (s *Server) PauseTicket(project, epic, id string, kind events.Kind, reason string) error {
	dir := filepath.Join(s.cfg.TicketStore, project)
	path := filepath.Join(dir, epic, "issues", id+"-first.md")
	return s.pauseTicket(dir, tickets.Address{Project: project, Epic: epic, ID: id}, path, kind, reason, Run{})
}

// PruneBackups runs the backup-ref prune as of now.
func (s *Server) PruneBackups(repoRoot string, now time.Time) { s.pruneBackups(repoRoot, now) }

// PutRunFrom is PutRun with the base the iteration branched from.
func (s *Server) PutRunFrom(root string, r Run, base string) {
	s.registry.put(trackedRun{Run: r, Root: root, Base: base})
}

// GateReleased reports whether the run's background-task gate was released.
func (s *Server) GateReleased(address string) bool { return s.registry.gateReleased(address)() }

// DropRun forgets a run the way its finish does.
func (s *Server) DropRun(address string) { s.registry.delete(address) }

// SettleReleasedRun stands in for the finish of a run whose gate was released:
// the pause release-gate left, if any, then the run is dropped.
func (s *Server) SettleReleasedRun(address string) {
	addr, _ := tickets.ParseAddress(address, tickets.AddressContext{})
	dir := filepath.Join(s.cfg.TicketStore, addr.Project)
	if run, ok := s.registry.runOf(address); ok {
		s.pauseReleasedGate(dir, addr, filepath.Join(dir, addr.Epic, "issues", addr.ID+"-first.md"), run)
	}
	s.registry.delete(address)
}

// ClaimNext runs one claim pass now.
func (s *Server) ClaimNext() { s.claimNext() }

// RecoverAsync is recoverAsync for a failure the test raises itself.
func (s *Server) RecoverAsync(f recovery.Failure) { s.recoverAsync(f) }

// RecoveryDo calls one server verb the way a remedy does.
func (s *Server) RecoveryDo(c recovery.Call) (recovery.Result, error) { return recoveryVerbs{s}.Do(c) }

// RunOf is the tracked run of address, watched ones included.
func (s *Server) RunOf(address string) (Run, bool) { return s.registry.runOf(address) }

// WatchGates runs one gate watchdog pass as of now.
func (s *Server) WatchGates(now time.Time) { s.watchGates(now) }

// PauseStuckFinishedClaims runs one finished-claim watchdog pass as of now.
func (s *Server) PauseStuckFinishedClaims(now time.Time) { s.pauseStuckFinishedClaims(now) }

// WorktreeDir is where the project's iteration worktrees live.
func (s *Server) WorktreeDir(project string) string { return s.worktreeDir(project) }

// RecoveredCount is recoveredCount for tests.
func RecoveredCount(projectDir, epic string) int { return recoveredCount(projectDir, epic) }

// HoldTrunkLock takes the project's trunk lock the way a merge does, until
// release runs.
func (s *Server) HoldTrunkLock(project, trunk string) (release func(), err error) {
	_, repo, err := s.projectOf(project)
	if err != nil {
		return nil, err
	}
	return s.trunkLocks.lock(repo, trunk), nil
}

// CompleteRootIfDone runs the all-done check of address's root as if its
// iteration had just settled.
func (s *Server) CompleteRootIfDone(address string) error {
	addr, err := tickets.ParseAddress(address, tickets.AddressContext{})
	if err != nil {
		return err
	}
	_, t, ok := s.freshTicket(addr)
	if !ok {
		return errors.New("no ticket " + address)
	}
	s.completeRootIfDone(rootOf(addr), ralphloop.OneIteration{Epic: addr.Epic, Ticket: t})
	return nil
}

// RemoveMergedWorktree starts the post-merge worktree removal the way stampEpic
// does, on the settle group.
func (s *Server) RemoveMergedWorktree(project, epic string) {
	root := rootRef{Project: project, Epic: epic}
	s.goSettle(func() { s.removeMergedWorktree(root) })
}

// PutRunIn registers a run of address in root, started in cwd, whose herdr
// session does not exist (a stopped agent whose entry settleRun has not yet
// deleted). launching marks it as one whose agent may not have started.
func (s *Server) PutRunIn(address, root, cwd string, launching bool) {
	t := trackedRun{Run: Run{Address: address, Runner: runnerHerdr, Session: agentrunner.Session{Label: "gone", ID: "gone"}}, Root: root, Cwd: cwd}
	if launching {
		t.State = runLaunching
	}
	s.registry.put(t)
}

// LockedTrunks lists the trunks any merge has locked so far, in any repo.
func (s *Server) LockedTrunks() []string {
	s.trunkLocks.mu.Lock()
	defer s.trunkLocks.mu.Unlock()
	var trunks []string
	for key := range s.trunkLocks.locks {
		_, trunk, _ := strings.Cut(key, "\x00")
		trunks = append(trunks, trunk)
	}
	return trunks
}
