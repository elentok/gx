package cleanup

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/elentok/gx/cleanupscan"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ralphloop/worktree"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/ticketgit"
)

func (c *Cleanup) removeWrite(w http.ResponseWriter, r *http.Request) {
	c.write(w, r, c.RemoveWorktree)
}

func (c *Cleanup) deleteWrite(w http.ResponseWriter, r *http.Request) {
	c.write(w, r, c.DeleteBranch)
}

func (c *Cleanup) write(w http.ResponseWriter, r *http.Request, do func(project, branch string) (wire.CleanupWriteResult, error)) {
	var req wire.CleanupWriteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	res, err := do(req.Project, req.Branch)
	if err != nil {
		code := http.StatusInternalServerError
		if _, ok := errors.AsType[*tickets.AddressError](err); ok {
			code = http.StatusNotFound
		}
		http.Error(w, err.Error(), code)
		return
	}
	wire.WriteJSON(w, res)
}

// RemoveWorktree removes a merged branch's worktree, then deletes the branch.
func (c *Cleanup) RemoveWorktree(project, branch string) (wire.CleanupWriteResult, error) {
	return c.act(project, branch, true)
}

// DeleteBranch deletes a merged branch that has no worktree.
func (c *Cleanup) DeleteBranch(project, branch string) (wire.CleanupWriteResult, error) {
	return c.act(project, branch, false)
}

// act re-scans under the worktree lock and writes only if the row still passes
// its merged check. Nothing here forces: git's refusal stays the row's reason.
func (c *Cleanup) act(project, branch string, withWorktree bool) (wire.CleanupWriteResult, error) {
	dir, err := tickets.ProjectDirByName(c.d.TicketStore, project)
	if err != nil {
		return wire.CleanupWriteResult{}, err
	}
	root := ticketgit.ProjectRepoRoot(c.d.TicketStore, tickets.ProjectName(dir))
	if root == "" {
		return wire.CleanupWriteResult{}, errors.New("project has no repo")
	}
	var res wire.CleanupWriteResult
	err = worktree.WithWorktreeLock(func() error {
		defer c.Invalidate()
		var err error
		res, err = c.actLocked(root, branch, withWorktree)
		return err
	})
	return res, err
}

func (c *Cleanup) actLocked(root, branch string, withWorktree bool) (wire.CleanupWriteResult, error) {
	scan, err := cleanupscan.ScanIn(c.d.TicketStore, root)
	if err != nil {
		return wire.CleanupWriteResult{}, err
	}
	var row *cleanupscan.WorktreeScan
	for i := range scan.Worktrees {
		if branch != "" && scan.Worktrees[i].Branch == branch {
			row = &scan.Worktrees[i]
		}
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
