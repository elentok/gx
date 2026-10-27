package cmd

import (
	"sort"
	"time"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/tickets"
)

// worktreeActiveWindow is how recent a branch's tip commit (or a linked
// worktree's uncommitted changes) must be to count as active-work. Chosen
// generously (a full working day plus overnight) since the cost of a false
// "active" is a missed recommendation, while the cost of a false negative is
// proposing to delete/merge work someone is mid-way through.
const worktreeActiveWindow = 48 * time.Hour

// WorktreeScan is one non-main branch's classification, as reported by
// `gx cleanup scan --json`.
type WorktreeScan struct {
	Branch string `json:"branch"`
	// Path is the linked worktree's directory, empty when the branch has no
	// worktree checked out.
	Path string `json:"path,omitempty"`
	Kind string `json:"kind"` // "iteration", "feature", "other"
	// Active is true when there's a recent commit or uncommitted changes,
	// suppressing Recommendation regardless of what it would otherwise be.
	Active bool `json:"active"`

	// Epic is set for "iteration" (the epic the iteration belongs to) and
	// "feature" (the epic itself) kinds.
	Epic string `json:"epic,omitempty"`

	// TicketID/TicketDone/Merged are set only for Kind == "iteration".
	TicketID   string `json:"ticket_id,omitempty"`
	TicketDone bool   `json:"ticket_done,omitempty"`
	// Merged is true when the branch's commits are already cherry-picked
	// into the epic's feature branch, or already reachable from main.
	Merged bool `json:"merged,omitempty"`

	// MergedToTrunk is set for Kind == "feature" and "other".
	MergedToTrunk bool `json:"merged_to_main,omitempty"`

	// Recommendation is "delete", "merge", or "" (no action proposed).
	// Always "" when Active is true.
	Recommendation string `json:"recommendation,omitempty"`
}

// scanWorktrees classifies every local branch other than repo.Trunk
// into an iteration/feature/other WorktreeScan, using epics (as loaded from
// .scratch) to resolve iteration branches to their ticket and feature
// branches to their epic.
func scanWorktrees(repo git.Repo, epics []tickets.Epic) ([]WorktreeScan, error) {
	branches, err := git.ListBranches(repo)
	if err != nil {
		return nil, err
	}
	worktrees, err := git.ListWorktrees(repo)
	if err != nil {
		return nil, err
	}
	pathByBranch := map[string]string{}
	for _, wt := range worktrees {
		if wt.Branch != "" {
			pathByBranch[wt.Branch] = wt.Path
		}
	}

	epicByName := map[string]tickets.Epic{}
	for _, e := range epics {
		epicByName[e.Name] = e
	}

	names := []string{}
	for _, b := range branches {
		if b.IsRemote || b.Name == repo.Trunk {
			continue
		}
		names = append(names, b.Name)
	}
	return classifyBranches(repo, names, pathByBranch, epicByName)
}

// classifyBranches classifies each named branch, sorted by name. A branch that
// no longer exists (deleted by a concurrent cleanup or loop since it was
// listed) is left out; any other failure fails the scan.
func classifyBranches(repo git.Repo, names []string, pathByBranch map[string]string, epicByName map[string]tickets.Epic) ([]WorktreeScan, error) {
	result := []WorktreeScan{}
	for _, name := range names {
		scan, err := classifyBranch(repo, name, pathByBranch[name], epicByName)
		if err != nil {
			if !git.IsLocalBranch(repo.Root, name) {
				continue
			}
			return nil, err
		}
		result = append(result, scan)
	}

	sort.Slice(result, func(i, j int) bool { return result[i].Branch < result[j].Branch })
	return result, nil
}

func classifyBranch(repo git.Repo, branch, path string, epicByName map[string]tickets.Epic) (WorktreeScan, error) {
	scan := WorktreeScan{Branch: branch, Path: path}

	active, err := isActiveWork(repo, branch, path)
	if err != nil {
		return WorktreeScan{}, err
	}
	scan.Active = active

	if epicName, ticketID, ok := tickets.ParseIterationBranch(branch); ok {
		scan.Kind = "iteration"
		scan.Epic = epicName
		scan.TicketID = ticketID
		if epic, ok := epicByName[epicName]; ok {
			for _, t := range epic.Tickets {
				if t.Identifier == ticketID {
					scan.TicketDone = t.IsDone()
					break
				}
			}
		}

		merged, err := iterationMerged(repo, branch, epicName)
		if err != nil {
			return WorktreeScan{}, err
		}
		scan.Merged = merged

		if !scan.Active && scan.TicketDone && scan.Merged {
			scan.Recommendation = "delete"
		}
		return scan, nil
	}

	if _, ok := epicByName[branch]; ok {
		scan.Kind = "feature"
		scan.Epic = branch
	} else {
		scan.Kind = "other"
	}

	merged, err := git.IsCommitMergedToTrunk(repo.Root, repo.Trunk, branch)
	if err != nil {
		return WorktreeScan{}, err
	}
	scan.MergedToTrunk = merged
	if !scan.Active && merged {
		scan.Recommendation = "delete"
	}

	return scan, nil
}

// iterationMerged reports whether branch's commits are already cherry-picked
// into the epic's feature branch (epicName), or already reachable from trunk
// — either way there's nothing left to merge. Checked as an ancestor first
// (the common case where the branch was never diverged/rebased away), then
// via patch-id equivalence (git cherry), since CherryPickRange produces
// fresh commit hashes on the feature branch that are never literally
// reachable from the source iteration branch.
func iterationMerged(repo git.Repo, branch, epicName string) (bool, error) {
	if git.IsLocalBranch(repo.Root, epicName) {
		anc, err := git.IsAncestor(repo.Root, branch, epicName)
		if err != nil {
			return false, err
		}
		if anc {
			return true, nil
		}

		if base, err := git.MergeBase(repo.Root, branch, epicName); err == nil {
			applied, err := git.PatchesApplied(repo.Root, epicName, base, branch)
			if err != nil {
				return false, err
			}
			if applied {
				return true, nil
			}
		}
	}

	return git.IsCommitMergedToTrunk(repo.Root, repo.Trunk, branch)
}

// isActiveWork reports whether branch has a commit within worktreeActiveWindow,
// or (when it has a linked worktree at path) uncommitted changes sitting in it.
func isActiveWork(repo git.Repo, branch, path string) (bool, error) {
	head, err := git.HeadCommit(repo.Root, branch)
	if err != nil {
		return false, err
	}
	if !head.Date.IsZero() && time.Since(head.Date) < worktreeActiveWindow {
		return true, nil
	}

	if path == "" {
		return false, nil
	}
	staged, unstaged, untracked, err := git.WorktreeStatusSummary(path)
	if err != nil {
		return false, err
	}
	return staged+unstaged+untracked > 0, nil
}
