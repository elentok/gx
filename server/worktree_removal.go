package server

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/tickets"
)

// removeMergedWorktree removes a merged epic's feature worktree, never its
// branch. It runs on the settle group, started by stampEpic while the stamping
// path still holds its trunk lock, so it first waits that lock out: check 3 then
// sees the queue after the dequeue. The first failed check keeps the worktree
// and logs why; nothing here pauses the epic or fails the merge.
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
	if epics, err := tickets.LoadResolved(project, dir); err == nil {
		_, _, target, refsErr := s.epicRefs(project, epics, root.Epic, repoDir)
		s.trunkLocks.lock(repoDir, mergeLockKey(root, target, refsErr))()
	}
	// Check 2, a live session or pane in the worktree, goes here.
	repo, err := git.FindRepo(repoDir)
	if err != nil {
		s.keepWorktree(root, path, err.Error())
		return
	}
	var reason string
	_ = ralphloop.WithWorktreeLock(func() error {
		reason = s.removeUnlessKept(root, path, repo)
		return nil
	})
	if reason != "" {
		s.keepWorktree(root, path, reason)
		return
	}
	s.log.Info("removed merged epic's feature worktree", "epic", root.String(), "path", path)
}

// removeUnlessKept runs checks 3 to 5 and returns why the worktree stays, or ""
// once it is removed. git refuses a worktree with changes, so no --force.
func (s *Server) removeUnlessKept(root rootRef, path string, repo *git.Repo) string {
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

func (s *Server) keepWorktree(root rootRef, path, reason string) {
	s.log.Info("kept merged epic's feature worktree", "epic", root.String(), "path", path, "reason", reason)
}

// sameDir reports whether a and b are one directory, through symlinks (macOS
// temp dirs sit under /private). An empty or unresolvable side is not the same.
func sameDir(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	ra, errA := filepath.EvalSymlinks(a)
	rb, errB := filepath.EvalSymlinks(b)
	return errA == nil && errB == nil && ra == rb
}
