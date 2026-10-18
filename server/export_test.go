package server

import (
	"errors"
	"path/filepath"
	"strings"
	"time"

	"github.com/elentok/gx/events"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/recovery"
	"github.com/elentok/gx/tickets"
)

var (
	RecoveryState  = recoveryState
	HardGraceTitle = hardGraceTitle
)

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

// ParkAs parks a ticket as the given kind the way the server's own parks do,
// including starting recovery.
func (s *Server) ParkAs(address string, kind events.Kind, reason string) error {
	addr, err := tickets.ParseAddress(address, tickets.AddressContext{})
	if err != nil {
		return err
	}
	ref, ok, err := s.findTicket(address)
	if err != nil || !ok {
		return errors.Join(err, errors.New("no ticket "+address))
	}
	return s.parkTicket(ref.projectDir, addr, ref.ticket.Path, kind, reason)
}

// SetCostOf swaps how an iteration's cost is read.
func (s *Server) SetCostOf(f func(IterationInfo) (float64, bool)) { s.costOf = f }

// SetMergeInto swaps the merge core a root land calls.
func (s *Server) SetMergeInto(f func(git.Repo, []git.Worktree, string, string) (git.MergeOutcome, error)) {
	s.mergeInto = f
}

// SetLandedProbe swaps the poll-time ancestry check.
func (s *Server) SetLandedProbe(f func(repo, ancestor, descendant string) (bool, error)) {
	s.landedProbe = f
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

// ParkTicket parks a ticket the way a failed iteration does: the write, the
// event, then the chat message (or its hold while recovery runs).
func (s *Server) ParkTicket(project, epic, id string, kind events.Kind, reason string) error {
	dir := filepath.Join(s.cfg.TicketStore, project)
	path := filepath.Join(dir, epic, "issues", id+"-first.md")
	return s.parkTicket(dir, tickets.Address{Project: project, Epic: epic, ID: id}, path, kind, reason)
}

// PutRunFrom is PutRun with the base the iteration branched from.
func (s *Server) PutRunFrom(root string, r Run, base string) {
	s.registry.put(trackedRun{Run: r, Root: root, Base: base})
}

// GateReleased reports whether the run's background-task gate was released.
func (s *Server) GateReleased(address string) bool { return s.registry.gateReleased(address)() }

// DropRun forgets a run the way its finish does.
func (s *Server) DropRun(address string) { s.registry.delete(address) }

// ClaimNext runs one claim pass now.
func (s *Server) ClaimNext() { s.claimNext() }

// RecoverAsync is recoverAsync for a failure the test raises itself.
func (s *Server) RecoverAsync(f recovery.Failure) { s.recoverAsync(f) }

// WatchGates runs one gate watchdog pass as of now.
func (s *Server) WatchGates(now time.Time) { s.watchGates(now) }

// WorktreeDir is where the project's iteration worktrees live.
func (s *Server) WorktreeDir(project string) string { return s.worktreeDir(project) }

// RecoveredCount is recoveredCount for tests.
func RecoveredCount(projectDir, epic string) int { return recoveredCount(projectDir, epic) }

// LockedTrunks lists the trunks any land has locked so far, in any repo.
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
