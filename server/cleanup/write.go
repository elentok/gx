package cleanup

import (
	"encoding/json"
	"net/http"

	"github.com/elentok/gx/cleanupscan"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ralphloop/worktree"
	"github.com/elentok/gx/server/wire"
)

func (c *Cleanup) removeWrite(w http.ResponseWriter, r *http.Request) {
	c.write(w, r, byBranch(c.RemoveWorktree))
}

func (c *Cleanup) deleteWrite(w http.ResponseWriter, r *http.Request) {
	c.write(w, r, byBranch(c.DeleteBranch))
}

func (c *Cleanup) removeUnmergedWrite(w http.ResponseWriter, r *http.Request) {
	c.write(w, r, byBranch(c.RemoveUnmergedWorktree))
}

func (c *Cleanup) archiveWrite(w http.ResponseWriter, r *http.Request) {
	c.write(w, r, func(req wire.CleanupWriteRequest) (wire.CleanupWriteResult, error) {
		return c.ArchiveEpic(req.Project, req.Epic)
	})
}

func byBranch(do func(project, branch string) (wire.CleanupWriteResult, error)) func(wire.CleanupWriteRequest) (wire.CleanupWriteResult, error) {
	return func(req wire.CleanupWriteRequest) (wire.CleanupWriteResult, error) {
		return do(req.Project, req.Branch)
	}
}

func (c *Cleanup) write(w http.ResponseWriter, r *http.Request, do func(wire.CleanupWriteRequest) (wire.CleanupWriteResult, error)) {
	decodeAndRun(w, r, do)
}

// decodeAndRun reads the write request body, runs do and answers with its result.
func decodeAndRun[T any](w http.ResponseWriter, r *http.Request, do func(wire.CleanupWriteRequest) (T, error)) {
	var req wire.CleanupWriteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	res, err := do(req)
	if err != nil {
		writeErr(w, err)
		return
	}
	wire.WriteJSON(w, res)
}

// RemoveWorktree removes a merged branch's worktree, then deletes the branch.
func (c *Cleanup) RemoveWorktree(project, branch string) (wire.CleanupWriteResult, error) {
	return c.act(project, branch, func(root, branch string) (wire.CleanupWriteResult, error) {
		return c.actLocked(root, branch, true)
	})
}

// DeleteBranch deletes a merged branch that has no worktree.
func (c *Cleanup) DeleteBranch(project, branch string) (wire.CleanupWriteResult, error) {
	return c.act(project, branch, func(root, branch string) (wire.CleanupWriteResult, error) {
		return c.actLocked(root, branch, false)
	})
}

// RemoveUnmergedWorktree removes the worktree of an unmerged branch that is
// clean and idle, and keeps the branch.
func (c *Cleanup) RemoveUnmergedWorktree(project, branch string) (wire.CleanupWriteResult, error) {
	return c.act(project, branch, c.removeUnmergedLocked)
}

// act runs do under the worktree lock. do re-scans and writes only if the row
// still passes its check. Nothing here forces: git's refusal stays the reason.
func (c *Cleanup) act(project, branch string, do func(root, branch string) (wire.CleanupWriteResult, error)) (wire.CleanupWriteResult, error) {
	_, root, err := c.repoFor(project)
	if err != nil {
		return wire.CleanupWriteResult{}, err
	}
	var res wire.CleanupWriteResult
	err = worktree.WithWorktreeLock(func() error {
		defer c.Invalidate()
		var err error
		res, err = do(root, branch)
		return err
	})
	return res, err
}

func (c *Cleanup) scanRow(root, branch string) (*cleanupscan.WorktreeScan, error) {
	return cleanupscan.ScanWorktree(c.d.TicketStore, root, branch)
}

// removeUnmergedLocked refuses a dirty worktree and one a live session works
// in. A recent commit does not block it.
func (c *Cleanup) removeUnmergedLocked(root, branch string) (wire.CleanupWriteResult, error) {
	row, err := c.scanRow(root, branch)
	if err != nil {
		return wire.CleanupWriteResult{}, err
	}
	switch {
	case row == nil:
		return refuse(wire.ReasonCleanupUnknown, "no such branch any more"), nil
	case row.Path == "":
		return refuse(wire.ReasonCleanupNoWorktree, "the branch has no worktree"), nil
	case row.Dirty:
		return refuse(wire.ReasonCleanupDirty, "the worktree has uncommitted changes"), nil
	}
	live, err := c.liveSessionIn(row.Path)
	if err != nil {
		return wire.CleanupWriteResult{}, err
	}
	if live != "" {
		return refuse(wire.ReasonCleanupLive, "session "+live+" is live in the worktree"), nil
	}
	repo, err := git.FindRepo(root)
	if err != nil {
		return wire.CleanupWriteResult{}, err
	}
	if err := git.RemoveWorktree(*repo, row.Path, false); err != nil {
		return refuse(wire.ReasonCleanupGitRefused, err.Error()), nil
	}
	return wire.CleanupWriteResult{WorktreeRemoved: true}, nil
}

func (c *Cleanup) liveSessionIn(path string) (string, error) {
	if c.d.Sessions == nil {
		return "", nil
	}
	sessions, err := c.d.Sessions()
	if err != nil {
		return "", err
	}
	for _, s := range sessions {
		if s.Status.State != wire.SessionExited && worktree.DirInside(s.Dir, path) {
			return s.ID, nil
		}
	}
	return "", nil
}

func (c *Cleanup) actLocked(root, branch string, withWorktree bool) (wire.CleanupWriteResult, error) {
	row, err := c.scanRow(root, branch)
	if err != nil {
		return wire.CleanupWriteResult{}, err
	}
	switch {
	case row == nil:
		return refuse(wire.ReasonCleanupUnknown, "no such branch any more"), nil
	case withWorktree && row.Path == "":
		return refuse(wire.ReasonCleanupNoWorktree, "the branch has no worktree"), nil
	case !withWorktree && row.Path != "":
		return refuse(wire.ReasonCleanupHasWorktree, "the branch still has a worktree"), nil
	case row.DeleteMode == cleanupscan.DeleteNone:
		return refuse(wire.ReasonCleanupNotMerged, "the branch is no longer proven merged"), nil
	}
	repo, err := git.FindRepo(root)
	if err != nil {
		return wire.CleanupWriteResult{}, err
	}
	var res wire.CleanupWriteResult
	if withWorktree {
		if err := git.RemoveWorktree(*repo, row.Path, false); err != nil {
			return refuse(wire.ReasonCleanupGitRefused, err.Error()), nil
		}
		res.WorktreeRemoved = true
	}
	if err := git.DeleteLocalBranch(*repo, branch, row.DeleteMode == cleanupscan.DeleteForce); err != nil {
		res.Refused, res.Reason, res.Message = true, wire.ReasonCleanupGitRefused, err.Error()
	}
	return res, nil
}

func refuse(reason, msg string) wire.CleanupWriteResult {
	return wire.CleanupWriteResult{Refused: true, Reason: reason, Message: msg}
}
