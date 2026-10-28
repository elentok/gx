package server

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/tickets"
)

// removeMergedWorktree removes a merged epic's feature worktree, never its
// branch. It runs on the settle group, started by stampEpic while the stamping
// path still holds its trunk lock, so it first waits that lock out: the queued
// check then sees the queue after the dequeue. The first failed check keeps the
// worktree and logs why; nothing here pauses the epic or fails the merge.
func (s *Server) removeMergedWorktree(root rootRef) {
	if !isMergeableEpic(root) {
		return
	}
	project := root.Project
	dir, repoDir, err := s.projectOf(project)
	if err != nil {
		return
	}
	base := s.worktreeDir(project)
	if base == "" {
		return
	}
	path := filepath.Join(base, root.Epic)
	if _, err := os.Stat(path); err != nil {
		return
	}
	if sameDir(path, tickets.TrunkCheckout(s.cfg.TicketStore, project)) {
		return
	}
	s.waitOutStamping(root, project, dir, repoDir)
	if reason := s.removeLockedUnlessKept(root, path, repoDir); reason != "" {
		s.log.Info("kept merged epic's feature worktree", "epic", root.String(), "path", path, "reason", reason)
		return
	}
	s.log.Info("removed merged epic's feature worktree", "epic", root.String(), "path", path)
}

// waitOutStamping blocks until the stamping path releases the epic's trunk lock.
func (s *Server) waitOutStamping(root rootRef, project, dir, repoDir string) {
	if epics, err := tickets.LoadResolved(project, dir); err == nil {
		_, _, target, refsErr := s.epicRefs(project, epics, root.Epic, repoDir)
		s.trunkLocks.lock(repoDir, mergeLockKey(root, target, refsErr))()
	}
}

// removeLockedUnlessKept runs removeUnlessKept under the worktree lock and
// returns why the worktree stays, or "" once it is removed.
func (s *Server) removeLockedUnlessKept(root rootRef, path, repoDir string) string {
	repo, err := git.FindRepo(repoDir)
	if err != nil {
		return err.Error()
	}
	var reason string
	_ = ralphloop.WithWorktreeLock(func() error {
		reason = s.removeUnlessKept(root, path, repo)
		return nil
	})
	return reason
}

// removeUnlessKept returns why the worktree stays, or "" once it is removed.
// git refuses a worktree with changes, so no --force.
func (s *Server) removeUnlessKept(root rootRef, path string, repo *git.Repo) string {
	if reason := s.liveWorkIn(root, path); reason != "" {
		return reason
	}
	if s.rootQueued(root) {
		return "the root is queued"
	}
	e, ok := s.freshEpic(root.Project, root.Epic)
	if !ok {
		return "the epic could not be loaded"
	}
	for _, t := range e.Tickets {
		if !e.RenderedStatus(t).Terminal() {
			return fmt.Sprintf("ticket %s is not done", t.Identifier)
		}
	}
	if err := git.RemoveWorktree(*repo, path, false); err != nil {
		return err.Error()
	}
	return ""
}

// liveWorkIn returns which live session or run works inside the
// worktree at path, or "".
func (s *Server) liveWorkIn(root rootRef, path string) string {
	if sessions, err := s.listSessions(); err != nil {
		return "sessions could not be listed: " + err.Error()
	} else {
		for _, in := range sessions {
			if in.Status.State != SessionExited && dirInside(in.Dir, path) {
				return "session " + in.ID + " is live in it"
			}
		}
	}
	for _, t := range s.registry.list() {
		// An older server saved no cwd: any live run of the epic may be inside.
		inside := dirInside(t.Cwd, path) || (t.Cwd == "" && t.Root == root.String())
		if inside && (t.State == runLaunching || s.runLive(t.Run)) {
			return "run " + t.Address + " is live in it"
		}
	}
	return ""
}

// runLive is liveIteration's rule applied to a registry entry, which settleRun
// can leave behind after the agent stopped.
func (s *Server) runLive(run Run) bool {
	runner := s.runnerForRun(run)
	if runner == nil || run.Session.ID == "" {
		return false
	}
	st, err := runner.Status(run.Session)
	return err == nil && (st.State != agentrunner.StateDone || runner.Name() == runnerHerdr)
}

// dirInside reports whether dir is path or below it, through symlinks and by
// whole path elements.
func dirInside(dir, path string) bool {
	rd, rp, ok := resolvePair(dir, path)
	if !ok {
		return false
	}
	rel, err := filepath.Rel(rp, rd)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// sameDir reports whether a and b are one directory, through symlinks (macOS
// temp dirs sit under /private). An empty or unresolvable side is not the same.
func sameDir(a, b string) bool {
	ra, rb, ok := resolvePair(a, b)
	return ok && ra == rb
}

// resolvePair resolves both paths through symlinks; ok is false when either is
// empty or unresolvable.
func resolvePair(a, b string) (ra, rb string, ok bool) {
	if a == "" || b == "" {
		return "", "", false
	}
	ra, errA := filepath.EvalSymlinks(a)
	rb, errB := filepath.EvalSymlinks(b)
	if errA != nil || errB != nil {
		return "", "", false
	}
	return ra, rb, true
}
