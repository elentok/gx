package server

import (
	"log/slog"
	"path/filepath"
	"time"

	"github.com/elentok/gx/server/budget"
	harnesschoice "github.com/elentok/gx/server/harness"
	"github.com/elentok/gx/server/limits"
	"github.com/elentok/gx/server/queue"
	srvrecovery "github.com/elentok/gx/server/recovery"
	"github.com/elentok/gx/server/sched"
	"github.com/elentok/gx/server/sessions/chat"
	sessprompt "github.com/elentok/gx/server/sessions/prompt"
	"github.com/elentok/gx/server/triage"
	"github.com/elentok/gx/server/webauth"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/server/workspaces"
	"github.com/elentok/gx/tickets"
)

// queueHost is queue.Host: the harness methods the queue reads, and the rest
// from the root.
type queueHost struct {
	sessionHost
	*harnesschoice.Harness
}

// CheckExtraUsage forwards to the budget, which owns the daily notice latch.
func (h queueHost) CheckExtraUsage(now time.Time) bool { return h.budget.CheckExtraUsage(now) }

// ContextTokens reads the agent's context count through the session chat's cache.
func (h queueHost) ContextTokens(harness, dir, conversationID, state string) int {
	if h.sessionChat == nil {
		return 0
	}
	_, tokens := h.sessionChat.ReportedModelAndContext(chat.Meta{
		Harness: harness, Dir: dir, ConversationID: conversationID, Status: wire.SessionStatus{State: state},
	})
	return tokens
}

// StampEpic forwards to the merger, which owns the epic stamps.
func (h queueHost) StampEpic(root queue.RootRef, epicPath string, keys ...string) {
	h.merger.StampEpic(root, epicPath, keys...)
}

// ClearHold takes the hold off the epic; the store maps an `-impl` epic to its Design.
func (h queueHost) ClearHold(root queue.RootRef) { h.clearHold(root.Project + ":" + root.Epic) }

// clearHold unholds key when it is held, so an unheld key writes nothing.
func (s *Server) clearHold(key string) {
	if !s.workspaces.IsHeld(key) {
		return
	}
	if err := s.workspaces.Unhold(key); err != nil {
		slog.Warn("clear hold", "key", key, "err", err)
	}
}

// dropSessionMember forgets a chat's workspace when its session exits or is removed.
func (s *Server) dropSessionMember(id string) {
	if err := s.workspaces.Remove(id); err != nil {
		slog.Warn("drop session workspace member", "session", id, "err", err)
	}
}

// pruneEpicMembers drops the workspace members and holds of epics and Designs
// that no longer exist. A finished epic still exists, so it keeps its
// workspace. Session keys are left to dropSessionMember, and an epic of a
// project the index does not know yet is kept.
func (s *Server) pruneEpicMembers() {
	err := s.workspaces.Prune(func(key string) bool {
		project, _, ok := tickets.ParseEpicAddress(key)
		if !ok {
			return true
		}
		if _, _, known := s.idx.ProjectEpics(project); !known {
			return true
		}
		_, exists := workspacesDirectory{s}.Epic(key)
		return exists
	})
	if err != nil {
		slog.Warn("prune workspace members", "err", err)
	}
}

// Workspaces is the workspace and hold store. Tests outside the package read
// their seams through here.
func (s *Server) Workspaces() *workspaces.Store { return s.workspaces }

// Prompt is the prompt code. Tests outside the package read the auto-queue
// seams through here.
func (s *Server) Prompt() *sessprompt.Prompt { return s.prompt }

// TriageOf gives tests the Triage seams (StopFinished). A function, not a
// method, to keep Server under its method limit.
func TriageOf(s *Server) *triage.Triage { return s.triage }

// Harness is the server's harness choice. Tests outside the package read it
// through here.
func (s *Server) Harness() *harnesschoice.Harness { return s.harness }

// Limits is the usage limits and token poll. Tests outside the package read
// their seams through here.
func (s *Server) Limits() *limits.Limits { return s.limits }

// Budget is the daily spend budget. Tests outside the package read its seams
// through here.
func (s *Server) Budget() *budget.Budget { return s.budget }

// Recovery is the failure-recovery subsystem. Tests outside the package read
// its seams through here.
func (s *Server) Recovery() *srvrecovery.Recovery { return s.recovery }

// SettleReleasedRun stands in for the finish of a run whose gate was released:
// the pause release-gate left, if any, then the run is dropped. For tests.
func (s *Server) SettleReleasedRun(address string) {
	addr, _ := tickets.ParseAddress(address, tickets.AddressContext{})
	dir := filepath.Join(s.cfg.TicketStore, addr.Project)
	if run, ok := s.registry.RunOf(address); ok {
		s.recovery.PauseReleasedGate(dir, addr, filepath.Join(dir, addr.Epic, "issues", addr.ID+"-first.md"), run)
	}
	s.registry.Delete(address)
}

// Scheduler is the claim loop's watchdogs. Tests outside the package read their
// seams through here.
func (s *Server) Scheduler() *sched.Scheduler { return s.scheduler }

// WebAuth is the web login gate, nil unless the web listener is bound. Tests
// outside the package read its seams through here.
func (s *Server) WebAuth() *webauth.Gate { return s.web.Gate() }
