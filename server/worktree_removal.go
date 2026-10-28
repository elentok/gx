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

// removeUnlessKept runs checks 2 to 5 and returns why the worktree stays, or ""
// once it is removed. git refuses a worktree with changes, so no --force.
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

// liveWorkIn is check 2: it returns which live session or run works inside the
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
	if dir == "" || path == "" {
		return false
	}
	rd, errD := filepath.EvalSymlinks(dir)
	rp, errP := filepath.EvalSymlinks(path)
	if errD != nil || errP != nil {
		return false
	}
	rel, err := filepath.Rel(rp, rd)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
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
