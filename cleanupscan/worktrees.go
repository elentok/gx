package cleanupscan

import (
	"sort"
	"time"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/ticketgit"
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
	Kind string `json:"kind"` // "iteration", "feature", "other", "detached"
	// Active is true when there's a recent commit or uncommitted changes,
	// suppressing Recommendation regardless of what it would otherwise be.
	Active bool `json:"active"`
	// Dirty is true when the linked worktree has uncommitted changes. Unlike
	// Active it ignores commit age, so a caller can gate removing the worktree.
	Dirty bool `json:"dirty,omitempty"`
	// Head is the checked-out commit of a detached worktree (Branch is empty).
	Head string `json:"head,omitempty"`

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

	// DeleteMode is the `git branch` flag that deletes the branch ("-d" or
	// "-D"), empty when it is not proven merged.
	DeleteMode string `json:"delete_mode,omitempty"`

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
	detached := []git.Worktree{}
	for _, wt := range worktrees {
		if wt.Branch == "" && wt.IsDetached && !wt.IsBare {
			detached = append(detached, wt)
		}
	}

	names := []string{}
	for _, b := range branches {
		if isScannableBranch(repo, b.Name, b.IsRemote) {
			names = append(names, b.Name)
		}
	}
	result, err := classifyBranches(repo, names, pathByBranch(worktrees), epicIndex(epics))
	if err != nil {
		return nil, err
	}
	for _, wt := range detached {
		scan, err := classifyDetached(repo, wt)
		if err != nil {
			return nil, err
		}
		result = append(result, scan)
	}
	return result, nil
}

// isScannableBranch is the filter both scans share: a local branch that is
// not the trunk.
func isScannableBranch(repo git.Repo, name string, isRemote bool) bool {
	return !isRemote && name != repo.Trunk
}

// pathByBranch maps each checked-out branch to its worktree path.
func pathByBranch(worktrees []git.Worktree) map[string]string {
	m := map[string]string{}
	for _, wt := range worktrees {
		if wt.Branch != "" {
			m[wt.Branch] = wt.Path
		}
	}
	return m
}

// epicIndex maps epic names to their epics.
func epicIndex(epics []tickets.Epic) map[string]tickets.Epic {
	m := map[string]tickets.Epic{}
	for _, e := range epics {
		m[e.Name] = e
	}
	return m
}

// classifyDetached reports a detached-HEAD worktree. It has no branch to
// delete or merge, so it never gets a recommendation.
func classifyDetached(repo git.Repo, wt git.Worktree) (WorktreeScan, error) {
	scan := WorktreeScan{Path: wt.Path, Kind: "detached", Head: wt.Head}
	dirty, err := worktreeDirty(wt.Path)
	if err != nil {
		return WorktreeScan{}, err
	}
	scan.Dirty = dirty
	recent, err := recentCommit(repo, wt.Head)
	if err != nil {
		return WorktreeScan{}, err
	}
	scan.Active = dirty || recent
	return scan, nil
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
	scan.Dirty, err = worktreeDirty(path)
	if err != nil {
		return WorktreeScan{}, err
	}

	if epicName, ticketID, ok := ticketgit.ParseIterationBranch(branch); ok {
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

		check, err := CheckMerged(repo, branch, epicName)
		if err != nil {
			return WorktreeScan{}, err
		}
		scan.Merged = check.Merged
		scan.DeleteMode = check.DeleteMode

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

	check, err := CheckMerged(repo, branch, "")
	if err != nil {
		return WorktreeScan{}, err
	}
	scan.MergedToTrunk = check.Merged
	scan.DeleteMode = check.DeleteMode
	if !scan.Active && check.Merged {
		scan.Recommendation = "delete"
	}

	return scan, nil
}

// isActiveWork reports whether branch has a commit within worktreeActiveWindow,
// or (when it has a linked worktree at path) uncommitted changes sitting in it.
func isActiveWork(repo git.Repo, branch, path string) (bool, error) {
	recent, err := recentCommit(repo, branch)
	if err != nil || recent {
		return recent, err
	}
	return worktreeDirty(path)
}

// recentCommit reports whether ref's tip is within worktreeActiveWindow.
func recentCommit(repo git.Repo, ref string) (bool, error) {
	head, err := git.HeadCommit(repo.Root, ref)
	if err != nil {
		return false, err
	}
	return !head.Date.IsZero() && time.Since(head.Date) < worktreeActiveWindow, nil
}

// worktreeDirty reports uncommitted changes at path; an empty path (no
// worktree) is never dirty.
func worktreeDirty(path string) (bool, error) {
	if path == "" {
		return false, nil
	}
	staged, unstaged, untracked, err := git.WorktreeStatusSummary(path)
	if err != nil {
		return false, err
	}
	return staged+unstaged+untracked > 0, nil
}
