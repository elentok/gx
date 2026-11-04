package server

import (
	"github.com/elentok/gx/check"
	"github.com/elentok/gx/server/projects"
)

// checkWaitingLabel is the status label of a claimed ticket whose `gx check
// --full` waits for the check lock, or "". It is a label only: no pause kind,
// no status change.
//
// Mechanism: the waiting file the check writes names the worktree it checks
// (check.Waiting.Root); the ticket's tracked run knows its worktree (Cwd), so
// the two meet on that path. check.ReadWaiting drops a waiter whose process is
// gone, so a killed waiter never leaves a label, and a run with no tracked
// ticket matches nothing.
func (s *Server) checkWaitingLabel(address string) string {
	t, ok := s.registry.Tracked(address)
	if !ok || t.Cwd == "" {
		return ""
	}
	cwd := projects.CanonicalDir(t.Cwd)
	for _, w := range check.ReadWaiting(s.cfg.StateDir) {
		if w.Root == "" || projects.CanonicalDir(w.Root) != cwd {
			continue
		}
		if w.HeldBy == "" {
			return "waiting for the check lock"
		}
		return "waiting for the check lock, held by " + w.HeldBy
	}
	return ""
}
