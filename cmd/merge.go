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
	Target       string `json:"target,omitempty"`
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

	outcome, err := git.MergeBranchInto(repo, worktrees, branch, repo.MainBranch)
	if err != nil {
		return err
	}

	result := MergeResult{Status: "merged"}
	if !outcome.Merged {
		result = MergeResult{
			Status:       "needs_rebase",
			Branch:       outcome.Branch,
			Target:       outcome.Target,
			WorktreePath: outcome.WorktreePath,
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
		fmt.Fprintf(w, "needs rebase: %s onto %s\n", result.Branch, result.Target)
		if result.WorktreePath != "" {
			fmt.Fprintf(w, "worktree: %s\n", result.WorktreePath)
		}
	}
}
