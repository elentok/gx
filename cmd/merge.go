package cmd

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/elentok/gx/git"
)

// MergeResult is the `gx merge <branch> --json` payload.
type MergeResult struct {
	Status       string `json:"status"` // "merged" or "needs_rebase"
	Branch       string `json:"branch,omitempty"`
	Base         string `json:"base,omitempty"`
	WorktreePath string `json:"worktree_path"`
}

// runMerge is the deterministic core of `gx merge <branch>` (see ADR 0015):
// it resolves branchArg to a real branch name, attempts a fast-forward-only
// merge of it into the repo's main branch inside main's own worktree — never
// cwd's current branch — and reports the outcome to w. It never rebases,
// never pushes, and stops without further mutation on a non-fast-forward
// refusal.
func runMerge(cwd, branchArg string, jsonOut bool, w io.Writer) error {
	info, err := git.IdentifyDir(cwd)
	if err != nil {
		return fmt.Errorf("not inside a git repo: %w", err)
	}
	repo := info.Repo

	worktrees, err := git.ListWorktrees(repo)
	if err != nil {
		return err
	}

	branch := resolveMergeBranch(branchArg, worktrees)

	ok, err := git.MergeFastForward(mainWorktreeDir(repo, worktrees), branch)
	if err != nil {
		return err
	}

	var result MergeResult
	if ok {
		result = MergeResult{Status: "merged"}
	} else {
		result = MergeResult{
			Status:       "needs_rebase",
			Branch:       branch,
			Base:         repo.MainBranch,
			WorktreePath: worktreePathForBranch(branch, worktrees),
		}
	}

	if jsonOut {
		return printMergeJSON(w, result)
	}
	printMergeText(w, result)
	return nil
}

// resolveMergeBranch follows the existing worktree listing when arg matches a
// worktree's directory basename (e.g. a nested "ralph-loop/..." branch
// checked out under a shorter worktree dir name), otherwise treats arg as a
// literal branch name.
func resolveMergeBranch(arg string, worktrees []git.Worktree) string {
	for _, wt := range worktrees {
		if wt.Name == arg {
			return wt.Branch
		}
	}
	return arg
}

// mainWorktreeDir returns the directory to run the merge in: the linked
// worktree checked out to repo.MainBranch, if one exists. Without one (main
// was never checked out as its own worktree) it falls back to repo.Root; for
// a non-bare repo that's the only working tree there is, and for a bare repo
// it has no working tree at all, so the merge below fails with git's own
// "must be run in a work tree" error rather than silently doing nothing.
func mainWorktreeDir(repo git.Repo, worktrees []git.Worktree) string {
	if path := worktreePathForBranch(repo.MainBranch, worktrees); path != "" {
		return path
	}
	return repo.Root
}

func worktreePathForBranch(branch string, worktrees []git.Worktree) string {
	for _, wt := range worktrees {
		if wt.Branch == branch {
			return wt.Path
		}
	}
	return ""
}

func printMergeJSON(w io.Writer, result MergeResult) error {
	b, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "%s\n", b)
	return err
}

func printMergeText(w io.Writer, result MergeResult) {
	switch result.Status {
	case "merged":
		fmt.Fprintln(w, "merged")
	case "needs_rebase":
		fmt.Fprintf(w, "needs rebase: %s onto %s\n", result.Branch, result.Base)
		if result.WorktreePath != "" {
			fmt.Fprintf(w, "worktree: %s\n", result.WorktreePath)
		}
	}
}
