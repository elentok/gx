package git

import (
	"fmt"
	"strings"
)

const expectedFetchRefspec = "+refs/heads/*:refs/remotes/origin/*"

// FetchConfigProblem describes a misconfigured origin fetch setup and the
// commands needed to fix it.
type FetchConfigProblem struct {
	Description string
	Commands    []string
}

// CheckFetchConfig checks whether origin is configured to populate
// refs/remotes/origin/* and that those refs exist. Returns nil if everything
// looks fine, or a FetchConfigProblem describing what to fix.
func CheckFetchConfig(repoRoot string) *FetchConfigProblem {
	// No origin remote — nothing to check.
	if runAllowFail(repoRoot, []string{"remote", "get-url", "origin"}) == "" {
		return nil
	}

	refspec := runAllowFail(repoRoot, []string{"config", "remote.origin.fetch"})
	hasRefs := runAllowFail(repoRoot, []string{"for-each-ref", "--format=x", "--count=1", "refs/remotes/origin/"}) != ""

	if refspec == expectedFetchRefspec && hasRefs {
		return nil
	}

	var desc string
	var cmds []string

	if refspec != expectedFetchRefspec {
		desc = fmt.Sprintf(
			"The fetch refspec for origin is %q.\nIt should be %q so that remote tracking refs are populated.",
			refspec, expectedFetchRefspec,
		)
		cmds = append(cmds, fmt.Sprintf("git config remote.origin.fetch '%s'", expectedFetchRefspec))
	} else {
		desc = "No remote tracking refs found for origin (refs/remotes/origin/* is empty)."
	}
	cmds = append(cmds, "git fetch origin")

	return &FetchConfigProblem{Description: desc, Commands: cmds}
}

// FixFetchConfig corrects the origin fetch refspec and runs git fetch.
func FixFetchConfig(repoRoot string) error {
	if _, _, err := run(repoRoot, []string{"config", "remote.origin.fetch", expectedFetchRefspec}); err != nil {
		return err
	}
	_, _, err := run(repoRoot, []string{"fetch", "origin"})
	return err
}

// Fetch runs `git fetch <remote>` in repoRoot.
func Fetch(repoRoot, remote string) error {
	_, _, err := run(repoRoot, []string{"fetch", remote})
	return err
}

// ListRemotes returns the names of all configured remotes.
func ListRemotes(repo Repo) ([]string, error) {
	out, _, err := run(repo.Root, []string{"remote"})
	if err != nil {
		return nil, err
	}
	var remotes []string
	for _, r := range strings.Split(out, "\n") {
		if r != "" {
			remotes = append(remotes, r)
		}
	}
	return remotes, nil
}

// UpdateRemotes fetches updates from all remotes and returns their combined output.
func UpdateRemotes(repo Repo) (string, error) {
	stdout, stderr, err := run(repo.Root, []string{"remote", "update"})
	return joinOutput(stdout, stderr), err
}

// PruneRemote removes remote-tracking references for deleted remote branches.
func PruneRemote(repo Repo, remote string) error {
	_, _, err := run(repo.Root, []string{"remote", "prune", remote})
	return err
}

// Pull fetches and integrates changes from the remote into the worktree and
// returns the combined output.
func Pull(worktreePath string) (string, error) {
	stdout, stderr, err := run(worktreePath, []string{"pull"})
	return joinOutput(stdout, stderr), err
}

// BranchRemote returns the remote configured for branch (e.g. "origin"),
// falling back to "origin" if none is set.
func BranchRemote(repo Repo, branch string) string {
	remote := runAllowFail(repo.Root, []string{"config", "branch." + branch + ".remote"})
	if remote == "" {
		return "origin"
	}
	return remote
}

// Push uploads local branch commits to the remote using an explicit
// "git push <remote> <branch>" invocation.
func Push(worktreePath, remote, branch string) error {
	_, _, err := run(worktreePath, []string{"push", remote, branch})
	return err
}

// PushBranch pushes branch to remote and returns any PR creation URL found in
// the git output (e.g. the GitHub "Create a pull request" link), or "" if none.
// It also returns the combined output for display.
func PushBranch(worktreePath, remote, branch string) (prURL, output string, err error) {
	stdout, stderr, err := run(worktreePath, []string{"push", remote, branch})
	output = joinOutput(stdout, stderr)
	if err != nil {
		return "", output, err
	}
	return ExtractPRURL(output), output, nil
}

// ExtractPRURL scans git push output for a GitHub PR creation URL.
// Git prefixes remote messages with "remote: ", so we strip that first.
func ExtractPRURL(output string) string {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimPrefix(strings.TrimSpace(line), "remote:")
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "https://") && strings.Contains(line, "/pull/new/") {
			return line
		}
	}
	return ""
}

// PushBranchForceWithLease force-pushes branch using --force-with-lease.
func PushBranchForceWithLease(worktreePath, remote, branch string) error {
	_, _, err := run(worktreePath, []string{"push", "--force-with-lease", remote, branch})
	return err
}

// PushBranchForce force-pushes branch using --force and returns the combined output.
func PushBranchForce(worktreePath, remote, branch string) (string, error) {
	stdout, stderr, err := run(worktreePath, []string{"push", "--force", remote, branch})
	return joinOutput(stdout, stderr), err
}

// IsNonFastForwardPushError returns true when err matches a rejected push that
// can be resolved with a force push.
func IsNonFastForwardPushError(err error) bool {
	runErr, ok := err.(*RunError)
	if !ok {
		return false
	}

	s := strings.ToLower(runErr.Stdout + "\n" + runErr.Stderr)
	return strings.Contains(s, "non-fast-forward") ||
		strings.Contains(s, "[rejected]") ||
		strings.Contains(s, "fetch first") ||
		strings.Contains(s, "failed to push some refs")
}

// PruneAllRemotes prunes all configured remotes.
func PruneAllRemotes(repo Repo) error {
	remotes, err := ListRemotes(repo)
	if err != nil {
		return err
	}
	for _, remote := range remotes {
		if err := PruneRemote(repo, remote); err != nil {
			return err
		}
	}
	return nil
}
