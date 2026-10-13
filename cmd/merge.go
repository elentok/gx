package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/tickets"
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
// merge of it into the repo's trunk — never cwd's current branch — and reports
// the outcome to w. It never rebases, never pushes, and stops without further
// mutation on a non-fast-forward refusal.
func runMerge(cwd, branchArg string, jsonOut bool, w io.Writer) error {
	repo, err := tickets.OpenRepo(cwd)
	if repo == nil {
		return fmt.Errorf("not inside a git repo: %w", err)
	}
	if err != nil {
		return err
	}

	worktrees, err := git.ListWorktrees(*repo)
	if err != nil {
		return err
	}

	branch := resolveMergeBranch(branchArg, worktrees)

	outcome, err := git.MergeBranchInto(*repo, worktrees, branch, repo.Trunk)
	if err != nil {
		return err
	}

	result := MergeResult{Status: "merged", Target: repo.Trunk}
	if outcome.Merged {
		if err := stampMergedEpic(repo.Root, branch); err != nil {
			return fmt.Errorf("merged %s onto %s, but stamping merged_at failed: %w", branch, repo.Trunk, err)
		}
	} else {
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

// stampMergedEpic stamps merged_at on the epic whose feature branch is
// branch (an epic's branch is named after its directory). A branch of no
// epic, or a repo no project registers, stamps nothing. completed_at is left
// alone: it records the epic's outcome, not its land.
func stampMergedEpic(repoRoot, branch string) error {
	if branch == "" || filepath.Base(branch) != branch {
		return nil
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	projectDir, err := tickets.ProjectDir(cfg.TicketStore.Path, repoRoot)
	if errors.Is(err, tickets.ErrNoProject) {
		return nil
	}
	if err != nil {
		return err
	}
	epicPath := filepath.Join(projectDir, branch)
	if _, err := os.Stat(filepath.Join(epicPath, "ticket.md")); err != nil {
		return nil
	}
	return tickets.StampEpicMerged(epicPath, time.Now().UTC())
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
		fmt.Fprintf(w, "merged onto %s\n", result.Target)
	case "needs_rebase":
		fmt.Fprintf(w, "needs rebase: %s onto %s\n", result.Branch, result.Target)
		if result.WorktreePath != "" {
			fmt.Fprintf(w, "worktree: %s\n", result.WorktreePath)
		}
	}
}
