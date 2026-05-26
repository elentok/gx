package git

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

// BranchPRURL returns the GitHub PR URL for the current branch in worktreeRoot.
// Uses `gh pr view` which returns an error if no open PR exists.
func BranchPRURL(worktreeRoot string) (string, error) {
	return runGH(worktreeRoot, []string{"pr", "view", "--json", "url", "-q", ".url"})
}

// CommitPRURL returns the GitHub PR URL for the given commit hash, or "" if no
// PR is found. An empty result is not an error — callers surface it as a warning.
func CommitPRURL(worktreeRoot, hash string) (string, error) {
	return runGH(worktreeRoot, []string{
		"pr", "list",
		"--search", hash,
		"--state", "all",
		"--json", "url",
		"-q", ".[0].url",
	})
}

// IsCommitMergedToMain reports whether hash is an ancestor of the repo's main
// branch. Returns false (not an error) when git exits with code 1.
func IsCommitMergedToMain(worktreeRoot, hash string) (bool, error) {
	repo, err := FindRepo(worktreeRoot)
	if err != nil {
		return false, fmt.Errorf("IsCommitMergedToMain: %w", err)
	}
	_, _, err = run(repo.Root, []string{"merge-base", "--is-ancestor", hash, repo.MainBranch})
	if err != nil {
		if runErr, ok := err.(*RunError); ok && runErr.Code == 1 {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// runGH executes a gh command in dir and returns trimmed stdout.
func runGH(dir string, args []string) (string, error) {
	cmd := exec.Command("gh", args...)
	cmd.Dir = dir
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return "", &RunError{
				Args:   args,
				Dir:    dir,
				Stdout: strings.TrimSpace(outBuf.String()),
				Stderr: strings.TrimSpace(errBuf.String()),
				Code:   exitErr.ExitCode(),
			}
		}
		return "", fmt.Errorf("gh %s: %w", strings.Join(args, " "), err)
	}
	return strings.TrimRight(outBuf.String(), "\r\n"), nil
}
