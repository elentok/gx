package ralphloop

import (
	"fmt"
	"path/filepath"
)

// PrepareFeature runs a ticket in the epic's feature worktree itself, on the
// epic branch, with no iteration branch. A missing worktree is recreated from
// the branch. The handle is kept, so ending the ticket only stops its agent.
func PrepareFeature(d Deps, o OneIteration) (IterationWorktree, error) {
	w, err := ResumeFeature(d, o)
	if err != nil {
		return IterationWorktree{}, err
	}
	worktreeLock.Lock()
	defer worktreeLock.Unlock()
	if err := d.AddWorktree(o.RepoDir, w.Path, o.Epic, o.RootBase); err != nil {
		return IterationWorktree{}, fmt.Errorf("creating feature worktree for branch %q: %w", o.Epic, err)
	}
	return w, nil
}

// ResumeFeature rebuilds the handle PrepareFeature made, without touching git.
func ResumeFeature(d Deps, o OneIteration) (IterationWorktree, error) {
	wtDir, err := d.WorktreeDir(o.RepoDir)
	if err != nil {
		return IterationWorktree{}, fmt.Errorf("resolving worktree directory for %q: %w", o.RepoDir, err)
	}
	path := filepath.Join(wtDir, o.Epic)
	return IterationWorktree{
		Path: path, Label: iterLabel(o.Epic, o.Ticket.Identifier),
		featureWorktree: path, worktreeDir: wtDir, keep: true,
	}, nil
}
